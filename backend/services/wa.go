package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"kirimwa/backend/database"
	"kirimwa/backend/models"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "modernc.org/sqlite"
)

func init() {
	// Minta history sedalam mungkin dari HP (berlaku penuh saat pair/QR baru;
	// OnDemandReady juga membantu catch-up on-demand sesi yang sudah ada).
	// Catatan: batas nyata ditentukan HP/WhatsApp, bukan angka hard-coded 50 di UI.
	if store.DeviceProps != nil {
		store.DeviceProps.RequireFullSync = proto.Bool(true)
		if store.DeviceProps.HistorySyncConfig == nil {
			return
		}
		cfg := store.DeviceProps.HistorySyncConfig
		cfg.FullSyncDaysLimit = proto.Uint32(180)
		cfg.RecentSyncDaysLimit = proto.Uint32(30)
		cfg.InitialSyncMaxMessagesPerChat = proto.Uint32(5000)
		cfg.StorageQuotaMb = proto.Uint32(10240)
		cfg.OnDemandReady = proto.Bool(true)
		cfg.CompleteOnDemandReady = proto.Bool(true)
		cfg.InlineInitialPayloadInE2EeMsg = proto.Bool(true)
		cfg.SupportRecentSyncChunkMessageCountTuning = proto.Bool(true)
		cfg.SupportGroupHistory = proto.Bool(true)
	}
}

// Ukuran halaman & batas pengaman untuk HISTORY_SYNC_ON_DEMAND.
// whatsmeow merekomendasikan 50/halaman; HP biasanya melayani lebih besar.
// Kita paginate sampai HP tidak memberi pesan baru (bukan hard-stop di 50 total).
const (
	historyOnDemandPageSize    = 300
	historyOnDemandMaxCount    = 500
	historyDeepMaxPages        = 40 // pengaman ~12k pesan teoritis per klik sinkron
	historyDeepFullDays        = 90
	historyOnDemandTimeout     = 12 * time.Second
	fullHistoryOnDemandTimeout = 25 * time.Second
	interactiveWASendTimeout   = 20 * time.Second
	mediaWASendTimeout         = 90 * time.Second
	// History grup dibatasi agar initial sync dari akun dengan banyak grup tidak
	// memenuhi antrean database dan membuat Inbox terasa macet.
	groupHistoryPerChatLimit  = 100
	groupHistoryPerChunkLimit = 1000
)

var errHistoryAnchorUnavailable = errors.New("belum ada pesan acuan untuk catch-up")
var ErrWASendTimeout = errors.New("pengiriman WhatsApp melewati batas waktu")
var errHistoryDeviceNoResponse = errors.New("perangkat utama WhatsApp tidak merespons sinkronisasi")

func historySyncFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errHistoryDeviceNoResponse) {
		return "Perangkat tertaut tetap online, tetapi HP utama belum mengirim paket riwayat. Buka chat tersebut di HP dan biarkan WhatsApp aktif, lalu coba lagi."
	}
	// errors.Join menuliskan setiap error pada baris baru. Pertahankan detail
	// yang unik saja agar satu kegagalan fallback tidak terlihat berulang.
	seen := make(map[string]bool)
	parts := make([]string, 0, 3)
	for _, part := range strings.Split(err.Error(), "\n") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "Sinkronisasi belum lengkap. Coba lagi setelah WhatsApp di HP online."
	}
	if len(parts) == 1 && strings.HasPrefix(strings.ToLower(parts[0]), "sinkronisasi belum lengkap") {
		return parts[0]
	}
	return "Sinkronisasi belum lengkap: " + strings.Join(parts, "; ")
}

// IncomingMessage = isi pesan masuk (teks dan/atau media).
type IncomingMessage struct {
	Text      string // teks chat biasa atau caption media
	ActionID  string // ID internal dari tombol/list interaktif (tidak bergantung pada label)
	MediaType string // "", image, document, audio, video, sticker, location
	Mimetype  string
	FileName  string
	Data      []byte
	// MediaMetadata menyimpan envelope pesan WA untuk lazy-download/retry.
	// Pesan media tetap diteruskan ke Inbox walau unduhan pertama gagal.
	MediaMetadata []byte
	WAMsgID       string // ID pesan asli dari WhatsApp (untuk reply-to)
	WAMsgIDs      []string
	Timestamp     time.Time
	ChatJID       types.JID // alamat chat asli; penting untuk read receipt pada akun LID
	SenderJID     types.JID // pengirim asli yang dipakai server WhatsApp
	ReplyTo       string    // ID pesan yg di-reply (dari ContextInfo)
	ReplyText     string    // preview singkat pesan yang di-quote (teks/caption/label media)
	PushName      string    // nama profil pengirim (dari WA), untuk disimpan ke Contact
	// MessagePrelogged = true bila baris pesan customer sudah ditulis ke ChatHistory
	// (debounce per-pesan). logRow hanya boleh menulis balasan, bukan menggandakan pesan.
	MessagePrelogged bool
}

// MessageHandler dipanggil tiap pesan masuk, membawa ID agent (CS) penerima.
type MessageHandler func(agentID uint, sender types.JID, in IncomingMessage)

// OutgoingMessageHandler menerima pesan manual yang dikirim dari HP/WhatsApp Web
// lain pada akun yang sama. Pesan yang dikirim service sendiri tidak masuk jalur ini.
type OutgoingMessageHandler func(agentID uint, recipient types.JID, in IncomingMessage)

// HistoricalMessage adalah representasi ringan pesan dari HistorySync. Pesan lama
// sengaja tidak masuk pipeline OnWAMessage agar tidak memicu AI, webhook, atau read receipt.
type HistoricalMessage struct {
	Sender        string
	Text          string
	FromMe        bool
	MediaType     string
	FileName      string
	Mimetype      string
	MediaMetadata []byte
	WAMsgID       string
	ReplyTo       string
	ReplyText     string // preview quote dari ContextInfo (teks/caption/label media)
	PushName      string
	Timestamp     time.Time
}

type HistorySyncHandler func(agentID uint, messages []HistoricalMessage) (imported, skipped int, err error)
type HistoryChatState struct {
	Sender       string
	UnreadCount  int
	MarkedUnread bool
	Timestamp    time.Time
}
type HistoryChatStateHandler func(agentID uint, states []HistoryChatState)
type WhatsAppReadStateHandler func(agentID uint, sender string, read bool, timestamp time.Time)

type HistorySyncStatus struct {
	State    string `json:"state"`
	Mode     string `json:"mode,omitempty"`
	Sender   string `json:"sender,omitempty"`
	Progress int    `json:"progress"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
	Error    string `json:"error,omitempty"`
	// StillStale = true bila last_msg_at WA masih lebih baru dari pesan lokal
	// setelah sync — artinya preview Inbox bisa tetap basi.
	StillStale bool `json:"still_stale,omitempty"`
	// Message = teks jujur untuk UI (bukan "sudah diperiksa" palsu).
	Message    string     `json:"message,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// DeviceLinkedHandler dipanggil saat agent berhasil login via QR.
type DeviceLinkedHandler func(agentID uint, jid, number string)

// GroupMessageMeta = ringkasan pesan grup untuk moderasi (tanpa unduh media).
type GroupMessageMeta struct {
	GroupJID   string
	SenderJID  string // JID asli pengirim (untuk revoke/kick)
	SenderPN   string // nomor telepon pengirim (untuk cocokkan allowlist/admin & tampil)
	SenderName string
	Text       string // isi/caption (tanpa unduh media)
	MessageID  string
	Timestamp  time.Time
	FromMe     bool // true bila dikirim dari HP/WhatsApp Web milik akun tertaut
}

// GroupMessageHandler dipanggil tiap pesan grup masuk/keluar (jalur terpisah dari CS/AI).
type GroupMessageHandler func(agentID uint, m GroupMessageMeta)

// ReceiptMeta membawa perubahan status pengiriman pesan KELUAR kita (delivered/read/played).
type ReceiptMeta struct {
	Recipient  string   // nomor lawan chat yang menerima/membaca pesan kita
	Status     string   // "delivered" | "read" | "played"
	MessageIDs []string // ID pesan yang statusnya berubah
	Timestamp  int64    // unix detik
}

// ReceiptHandler dipanggil saat ada receipt (status pengiriman) untuk pesan keluar kita.
type ReceiptHandler func(agentID uint, m ReceiptMeta)

// ChatPresenceHandler meneruskan status mengetik pelanggan ke Inbox.
// Presence bersifat sementara dan tidak disimpan sebagai riwayat percakapan.
type ChatPresenceHandler func(agentID uint, sender string, active bool)

// MessageRevokeHandler dipanggil saat WhatsApp mengirim protokol "hapus untuk
// semua orang". messageID adalah stanza ID pesan asli yang harus ditandai revoked.
type MessageRevokeHandler func(agentID uint, messageID string)

type waInstance struct {
	mu               sync.Mutex
	labelSyncMu      sync.Mutex
	messageOrderMu   sync.Mutex
	messageOrder     map[int64]uint16
	agentID          uint
	client           *whatsmeow.Client
	qrCode           string
	qrExpiry         time.Time // kapan kode QR saat ini akan diputar whatsmeow (untuk countdown akurat)
	status           string    // "disconnected", "qr", "connecting", "connected", "expired", "pairing", "pair_error"
	contactsSynced   bool      // true setelah backfill nama kontak dari buku alamat (sekali per proses)
	readStatesSynced bool      // true setelah status read/unread app-state direkonsiliasi
	readPollSeq      uint64    // generasi polling incremental status baca
	historySyncMu    sync.Mutex
	historyRequestMu sync.Mutex // serialisasi job sync; terpisah dari status UI
	historyProbeMu   sync.Mutex
	historyWaitersMu sync.Mutex
	historyWaiters   map[string][]chan struct{}
	historyStatus    HistorySyncStatus
	historySeq       uint64
	reconcileSeq     uint64 // debounce rekonsiliasi Connected/OfflineSyncCompleted
	reconcileRunning bool
	reconcilePending bool
	groupNamesMu     sync.RWMutex
	groupNames       map[string]string
	groupAliasMu     sync.Mutex

	// Jalur login via kode pairing (alternatif QR): user memasukkan kode 8 huruf di WA.
	pairing   bool   // true bila sesi ini sedang dalam alur kode pairing (bukan QR)
	pairPhone string // nomor tujuan pairing (format internasional tanpa 0 di depan)
	pairCode  string // kode 8 huruf dari WhatsApp untuk ditampilkan ke user
	pairErr   string // pesan error bila permintaan kode pairing gagal
}

var (
	instances           = make(map[uint]*waInstance)
	globalMu            sync.Mutex
	legacyDBPath        = "./wa-assistant.db"
	onMessage           MessageHandler
	onOwnMessage        OutgoingMessageHandler
	onLinked            DeviceLinkedHandler
	onGroupMessage      GroupMessageHandler
	onReceipt           ReceiptHandler
	onChatPresence      ChatPresenceHandler
	onMessageRevoke     MessageRevokeHandler
	onHistorySync       HistorySyncHandler
	onHistoryChatState  HistoryChatStateHandler
	onWhatsAppReadState WhatsAppReadStateHandler
	waLogger            waLog.Logger = waLog.Noop // logger whatsmeow (default senyap; diaktifkan via WA_LOG_LEVEL)
)

func InitWA(dbPath string) {
	if dbPath != "" {
		legacyDBPath = dbPath
	}
	// Aktifkan log whatsmeow (disconnect/stream-error/reconnect) untuk diagnosa koneksi.
	// WA_LOG_LEVEL: WARN (default), INFO untuk lebih detail, atau NONE/OFF untuk senyap.
	if lvl := strings.ToUpper(strings.TrimSpace(os.Getenv("WA_LOG_LEVEL"))); lvl == "" || (lvl != "NONE" && lvl != "OFF") {
		if lvl == "" {
			lvl = "WARN"
		}
		waLogger = waLog.Stdout("WA", lvl, false)
	}
}

// SetHandlers mendaftarkan callback global (dipanggil sekali dari main).
func SetHandlers(msg MessageHandler, linked DeviceLinkedHandler) {
	onMessage = msg
	onLinked = linked
}

func SetOutgoingMessageHandler(handler OutgoingMessageHandler) {
	onOwnMessage = handler
}

func SetHistorySyncHandler(handler HistorySyncHandler)             { onHistorySync = handler }
func SetHistoryChatStateHandler(handler HistoryChatStateHandler)   { onHistoryChatState = handler }
func SetWhatsAppReadStateHandler(handler WhatsAppReadStateHandler) { onWhatsAppReadState = handler }

// SetGroupMessageHandler mendaftarkan callback moderasi pesan grup (dipanggil sekali dari main).
func SetGroupMessageHandler(h GroupMessageHandler) {
	onGroupMessage = h
}

// SetReceiptHandler mendaftarkan callback status pengiriman (receipt) pesan keluar.
func SetReceiptHandler(h ReceiptHandler) {
	onReceipt = h
}

func SetChatPresenceHandler(h ChatPresenceHandler) {
	onChatPresence = h
}

func SetMessageRevokeHandler(h MessageRevokeHandler) {
	onMessageRevoke = h
}

// Handler event label WhatsApp (Business).
type LabelEditHandler func(agentID uint, labelID, name string, color int, deleted bool)
type LabelAssocHandler func(agentID uint, sender, labelID string, labeled bool)

var (
	onLabelEdit  LabelEditHandler
	onLabelAssoc LabelAssocHandler
)

func SetLabelHandlers(edit LabelEditHandler, assoc LabelAssocHandler) {
	onLabelEdit = edit
	onLabelAssoc = assoc
}

// WALabelSnapshot adalah satu label WhatsApp dari hasil sinkronisasi penuh.
type WALabelSnapshot struct {
	LabelID string
	Name    string
	Color   int
}

// WALabelContactSnapshot adalah relasi label ke nomor telepon asli (PN), bukan LID.
type WALabelContactSnapshot struct {
	LabelID string
	Number  string
}

// ConnectedHandler dipanggil setelah koneksi awal/reconnect selesai untuk
// merekonsiliasi unread dan pesan yang tertinggal.
type ConnectedHandler func(agentID uint)

var onConnected ConnectedHandler

func SetConnectedHandler(h ConnectedHandler) { onConnected = h }

// WA mengembalikan instance WhatsApp untuk satu agent, membuatnya jika belum ada.
func WA(agentID uint) *waInstance {
	globalMu.Lock()
	defer globalMu.Unlock()
	if w, ok := instances[agentID]; ok {
		return w
	}
	w := &waInstance{
		agentID: agentID, status: "disconnected",
		historyWaiters: make(map[string][]chan struct{}),
		groupNames:     make(map[string]string),
		messageOrder:   make(map[int64]uint16),
	}
	instances[agentID] = w
	return w
}

// orderedLiveMessageTime mempertahankan urutan kedatangan untuk beberapa pesan
// dalam chat yang sama pada detik WA yang sama. WhatsApp hanya memberi presisi
// detik, sedangkan callback diserahkan ke goroutine; offset milidetik internal ini
// tidak mengubah tanggal/jam yang terlihat, tetapi mencegah urutan bubble terbalik.
func (w *waInstance) orderedLiveMessageTime(chat string, timestamp time.Time) time.Time {
	chat = strings.TrimSpace(chat)
	if timestamp.IsZero() || timestamp.Year() < 2020 {
		timestamp = time.Now()
	}
	base := timestamp.Truncate(time.Second)
	if chat == "" {
		return base
	}

	w.messageOrderMu.Lock()
	defer w.messageOrderMu.Unlock()
	if w.messageOrder == nil {
		w.messageOrder = make(map[int64]uint16)
	}
	second := base.Unix()
	offset := w.messageOrder[second]
	if offset < 999 {
		w.messageOrder[second] = offset + 1
	}
	// Simpan jendela kecil saja. Counter global per agent (bukan per chat)
	// mempertahankan juga urutan daftar percakapan pada detik yang sama.
	if len(w.messageOrder) > 256 {
		cutoff := second - 120
		for seenSecond := range w.messageOrder {
			if seenSecond < cutoff {
				delete(w.messageOrder, seenSecond)
			}
		}
	}
	return base.Add(time.Duration(offset) * time.Millisecond)
}

// RemoveWA memutus sesi WA agent, mengeluarkannya dari memori (map instances), dan menghapus
// file sesinya. Dipanggil saat agent dihapus agar tidak bocor memori/koneksi/file descriptor.
func RemoveWA(agentID uint) {
	globalMu.Lock()
	w, ok := instances[agentID]
	delete(instances, agentID)
	globalMu.Unlock()
	if ok {
		_ = w.Logout() // putus client + lepas koneksi/goroutine
	}
	// Hapus file sesi SQLite per-agent. Agent 1 memakai file lama bersama — jangan dihapus.
	if agentID != 1 {
		base := fmt.Sprintf("data/wa-session-agent-%d.db", agentID)
		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(base + suffix)
		}
	}
}

// sessionDSN: tiap agent punya file sesi SQLite sendiri (di-key per-agent, bukan per-JID
// yang mengandung ':'/'@'). Agent 1 memakai file lama agar sesi yang sudah login tidak hilang.
func sessionDSN(agentID uint) string {
	path := legacyDBPath
	if agentID != 1 {
		os.MkdirAll("data", 0o755)
		path = fmt.Sprintf("data/wa-session-agent-%d.db", agentID)
	}
	return "file:" + path + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
}

// FirstDeviceJID membaca device pada file sesi agent 1 (untuk migrasi single-number lama).
func FirstDeviceJID() string {
	container, err := sqlstore.New(context.Background(), "sqlite", sessionDSN(1), waLog.Noop)
	if err != nil {
		return ""
	}
	defer container.Close()
	devices, err := container.GetAllDevices(context.Background())
	if err != nil || len(devices) == 0 || devices[0].ID == nil {
		return ""
	}
	return devices[0].ID.String()
}

// Connect menyambungkan agent lewat QR. Param deviceJID tidak dipakai untuk path.
func (w *waInstance) Connect(_ string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client != nil {
		if w.client.Store.ID != nil {
			if !w.client.IsConnected() {
				_ = w.client.Connect()
			}
			return w.status, nil
		}
		w.client.Disconnect()
		w.client = nil
	}

	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite", sessionDSN(w.agentID), waLog.Noop)
	if err != nil {
		return "", fmt.Errorf("gagal buat store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return "", fmt.Errorf("gagal ambil device: %w", err)
	}

	w.client = whatsmeow.NewClient(device, waLogger)
	// Label WhatsApp disimpan di app-state. Tanpa opsi ini, label yang sudah ada
	// sebelum perangkat ditautkan tidak diterbitkan sebagai event saat full sync.
	w.client.EmitAppStateEventsOnFullSync = true
	w.client.AddEventHandler(w.handleEvent)

	if w.client.Store.ID == nil {
		qrChan, _ := w.client.GetQRChannel(ctx)
		if err := w.client.Connect(); err != nil {
			return "", fmt.Errorf("gagal connect: %w", err)
		}
		Go("watchQR", func() { w.watchQR(qrChan) })
		w.status = "qr"
		return "qr", nil
	}

	if err := w.client.Connect(); err != nil {
		return "", fmt.Errorf("gagal connect existing: %w", err)
	}
	w.status = "connected"
	return "connected", nil
}

// ConnectPairing menyambungkan agent lewat kode pairing alih-alih QR.
func (w *waInstance) ConnectPairing(_, phone string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client != nil {
		if w.client.Store.ID != nil {
			if !w.client.IsConnected() {
				_ = w.client.Connect()
			}
			return w.status, nil
		}
		w.client.Disconnect()
		w.client = nil
	}

	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite", sessionDSN(w.agentID), waLog.Noop)
	if err != nil {
		return "", fmt.Errorf("gagal buat store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return "", fmt.Errorf("gagal ambil device: %w", err)
	}

	w.client = whatsmeow.NewClient(device, waLogger)
	w.client.EmitAppStateEventsOnFullSync = true
	w.client.AddEventHandler(w.handleEvent)

	if w.client.Store.ID != nil {
		if err := w.client.Connect(); err != nil {
			return "", fmt.Errorf("gagal connect existing: %w", err)
		}
		w.status = "connected"
		return "connected", nil
	}

	qrChan, _ := w.client.GetQRChannel(ctx)
	if err := w.client.Connect(); err != nil {
		return "", fmt.Errorf("gagal connect: %w", err)
	}
	w.pairing = true
	w.pairPhone = phone
	w.pairCode = ""
	w.pairErr = ""
	w.status = "connecting"
	Go("watchQR", func() { w.watchQR(qrChan) })
	return "connecting", nil
}

func (w *waInstance) watchQR(qrChan <-chan whatsmeow.QRChannelItem) {
	for evt := range qrChan {
		if evt.Event == "code" {
			w.mu.Lock()
			// Mode pairing: kanal QR hanya penanda "koneksi siap". Minta kode 8 huruf sekali.
			if w.pairing {
				needCode := w.pairCode == "" && w.pairErr == ""
				phone := w.pairPhone
				w.mu.Unlock()
				if needCode {
					w.requestPairCode(phone)
				}
				continue
			}
			w.qrCode = evt.Code
			w.qrExpiry = time.Now().Add(evt.Timeout)
			w.status = "qr"
			w.mu.Unlock()
			continue
		}
		w.mu.Lock()
		w.qrCode = ""
		w.pairCode = ""
		w.pairing = false
		var jid *types.JID
		if w.client != nil {
			jid = w.client.Store.ID
		}
		if jid != nil {
			w.status = "connected"
		} else {
			w.status = "expired"
		}
		w.mu.Unlock()
		if jid != nil && onLinked != nil {
			onLinked(w.agentID, jid.String(), jid.User)
		}
		return
	}
}

// requestPairCode meminta kode pairing 8 huruf ke WhatsApp.
func (w *waInstance) requestPairCode(phone string) {
	code, err := w.client.PairPhone(context.Background(), phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.pairing {
		return
	}
	if err != nil {
		w.pairErr = pairErrMessage(err)
		w.status = "pair_error"
		log.Printf("WA agent %d: gagal minta kode pairing: %v", w.agentID, err)
		return
	}
	w.pairCode = code
	w.status = "pairing"
	log.Printf("WA agent %d: kode pairing tersedia", w.agentID)
}

// GetPairCode = kode pairing 8 huruf saat ini.
func (w *waInstance) GetPairCode() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pairCode
}

// GetPairError = pesan error alur pairing terakhir.
func (w *waInstance) GetPairError() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pairErr
}

func pairErrMessage(err error) string {
	msg := err.Error()
	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "bad request") || strings.Contains(msg, "400"):
		return "Nomor tidak valid atau belum terdaftar di WhatsApp. Pastikan nomor aktif dengan kode negara yang benar."
	case strings.Contains(msg, "too recent"):
		return "Terlalu sering mencoba. Tunggu beberapa menit sebelum mencoba lagi."
	default:
		return "Gagal membuat kode pairing: " + err.Error()
	}
}

func (w *waInstance) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		// Tersambung / berhasil reconnect.
		w.mu.Lock()
		w.status = "connected"
		w.qrCode = ""
		client := w.client
		w.contactsSynced = true
		syncReadStates := !w.readStatesSynced
		w.readStatesSynced = true
		w.readPollSeq++
		readPollSeq := w.readPollSeq
		w.mu.Unlock()
		log.Printf("WA agent %d: connected", w.agentID)
		// WhatsApp hanya mengirim event composing/paused bila perangkat tertaut
		// menyatakan dirinya online. Jalankan sekali per reconnect; kegagalan
		// presence tidak boleh menggagalkan koneksi utama.
		if client != nil {
			Go("presenceAvailable", func() {
				if err := client.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
					log.Printf("WA agent %d gagal mengaktifkan presence: %v", w.agentID, err)
				}
			})
		}
		// Jalankan rekonsiliasi juga saat reconnect, bukan hanya koneksi pertama.
		// OfflineSyncCompleted di bawah akan men-debounce panggilan ini sampai seluruh
		// event yang tertinggal selesai diterima.
		w.scheduleReconciliation("connected", 2*time.Second)
		Go("syncGroupNames", func() { _, _ = w.GetGroups() })
		if syncReadStates {
			Go("syncReadStates", func() {
				// Tunggu sinkronisasi awal whatsmeow selesai agar regular_low tidak
				// berebut dengan patch startup. Retry menangani koneksi yang baru stabil.
				time.Sleep(3 * time.Second)
				var err error
				for attempt := 1; attempt <= 3; attempt++ {
					if err = w.syncReadStates(); err == nil {
						log.Printf("WA agent %d: status baca antar perangkat tersinkron", w.agentID)
						return
					}
					log.Printf("WA agent %d gagal sinkron status baca (percobaan %d/3): %v", w.agentID, attempt, err)
					time.Sleep(time.Duration(attempt*3) * time.Second)
				}
				w.mu.Lock()
				w.readStatesSynced = false
				w.mu.Unlock()
			})
		}
		Go("readStateReconciler", func() { w.reconcileReadStates(readPollSeq) })

	case *events.Disconnected:
		// Putus sementara (jaringan) — whatsmeow akan auto-reconnect sendiri.
		log.Printf("WA agent %d: disconnected (mencoba reconnect otomatis)", w.agentID)

	case *events.OfflineSyncPreview:
		log.Printf("WA agent %d: menerima %d event tertinggal (%d pesan)",
			w.agentID, v.Total, v.Messages)

	case *events.OfflineSyncCompleted:
		// Semua event yang tertinggal sudah diterima. Rekonsiliasi unread + stale
		// sekali lagi agar pesan yang dikirim saat socket putus masuk ke Inbox.
		log.Printf("WA agent %d: offline sync selesai (%d event), rekonsiliasi Inbox", w.agentID, v.Count)
		w.scheduleReconciliation("offline-sync-completed", 250*time.Millisecond)

	case *events.LoggedOut:
		// Sesi dicabut/di-logout dari HP atau di-banned — TIDAK bisa auto-recover, perlu scan ulang.
		// Hapus sesi basi & buang client supaya Connect berikutnya bisa membuat QR baru.
		w.mu.Lock()
		if w.client != nil {
			if w.client.Store != nil && w.client.Store.ID != nil {
				_ = w.client.Store.Delete(context.Background())
			}
			w.client.Disconnect()
			w.client = nil
		}
		w.status = "disconnected"
		w.qrCode = ""
		w.reconcileSeq++ // batalkan rekonsiliasi tertunda dari Connected
		w.mu.Unlock()
		log.Printf("WA agent %d: LOGGED OUT (reason=%v) — sesi dibersihkan, perlu scan QR ulang", w.agentID, v.Reason)

	case *events.LabelEdit:
		if onLabelEdit != nil && v.Action != nil {
			onLabelEdit(w.agentID, v.LabelID, v.Action.GetName(), int(v.Action.GetColor()), v.Action.GetDeleted())
		}

	case *events.LabelAssociationChat:
		if onLabelAssoc != nil && v.Action != nil {
			number, err := phoneNumberForJID(context.Background(), w.client, v.JID)
			if err != nil {
				log.Printf("WA agent %d: label %s gagal mengubah JID %s ke nomor: %v", w.agentID, v.LabelID, v.JID, err)
				return
			}
			if number != "" {
				onLabelAssoc(w.agentID, number, v.LabelID, v.Action.GetLabeled())
			}
		}

	case *events.HistorySync:
		// History lama diproses di jalur terpisah. Menjalankannya lewat events.Message
		// biasa akan berbahaya karena dapat memicu AI, webhook, dan read receipt.
		// Respons ON_DEMAND yang valid dapat berisi nol percakapan ketika tidak ada
		// pesan tambahan. Tetap proses agar waiter manual tidak salah timeout.
		if v.Data != nil && onHistorySync != nil {
			Go("historySync", func() { w.processHistorySync(v) })
		}

	case *events.MarkChatAsRead:
		if onWhatsAppReadState != nil && v.Action != nil {
			number := v.JID.User
			if v.JID.Server == types.GroupServer {
				number = v.JID.String()
			} else if v.JID.Server != types.DefaultUserServer {
				if resolved, err := phoneNumberForJID(context.Background(), w.client, v.JID); err == nil {
					number = resolved
				}
			}
			if number != "" {
				read := v.Action.GetRead()
				stateAt := w.orderedLiveMessageTime(number, v.Timestamp)
				Go("onWhatsAppReadState", func() { onWhatsAppReadState(w.agentID, number, read, stateAt) })
			}
		}

	case *events.Receipt:
		// read-self/played-self berarti pemilik akun membuka pesan masuk dari HP
		// atau perangkat tertaut lain (terutama saat read receipts dimatikan).
		// Ini adalah sinyal otoritatif untuk menghapus badge Inbox.
		if isOwnerReadReceipt(v) {
			if onWhatsAppReadState != nil {
				chat := v.Chat
				if chat.Server == types.HiddenUserServer && !v.RecipientAlt.IsEmpty() {
					chat = v.RecipientAlt
				}
				number := chat.User
				if chat.Server == types.GroupServer {
					number = chat.String()
				} else if chat.Server != types.DefaultUserServer {
					if resolved, err := phoneNumberForJID(context.Background(), w.client, chat); err == nil {
						number = resolved
					}
				}
				if number != "" {
					stateAt := w.orderedLiveMessageTime(number, v.Timestamp)
					Go("onWhatsAppReadSelf", func() { onWhatsAppReadState(w.agentID, number, true, stateAt) })
				}
			}
			return
		}
		// Status pengiriman pesan KELUAR kita (delivered/read/played) dari lawan chat.
		// Hanya DM; marker milik kita sendiri sudah ditangani di atas.
		if onReceipt == nil || v.IsGroup || v.IsFromMe {
			return
		}
		var status string
		switch v.Type {
		case types.ReceiptTypeDelivered:
			status = "delivered"
		case types.ReceiptTypeRead:
			status = "read"
		case types.ReceiptTypePlayed:
			status = "played"
		default:
			return
		}
		ids := make([]string, 0, len(v.MessageIDs))
		for _, id := range v.MessageIDs {
			ids = append(ids, string(id))
		}
		meta := ReceiptMeta{Recipient: v.Sender.User, Status: status, MessageIDs: ids, Timestamp: v.Timestamp.Unix()}
		Go("onReceipt", func() { onReceipt(w.agentID, meta) })

	case *events.ChatPresence:
		// Jangan pernah menampilkan aktivitas perangkat kita sendiri sebagai
		// indikator pelanggan. Resolve LID di goroutine agar event loop WA ringan.
		if onChatPresence == nil || v.IsFromMe {
			return
		}
		source := v.MessageSource
		active := v.State == types.ChatPresenceComposing
		w.mu.Lock()
		client := w.client
		w.mu.Unlock()
		Go("onChatPresence", func() {
			sender := inboxPresenceSender(client, source)
			if sender != "" {
				onChatPresence(w.agentID, sender, active)
			}
		})

	case *events.Message:
		// status@broadcast (Status/Story) dan @newsletter adalah saluran sistem
		// WhatsApp, bukan pelanggan. whatsmeow menandai pesan broadcast sebagai
		// IsGroup=true (lihat parseMessageSource di library), sehingga tanpa filter
		// ini pesannya masuk cabang grup dan tercipta thread Inbox palsu
		// ("+status@broadcast"). History Sync sudah memfilter di processHistorySync;
		// jalur live ini harus konsisten. Letakkan paling awal agar revoke/echo
		// broadcast pun tidak diproses.
		if v.Info.Chat.Server == types.BroadcastServer || v.Info.Chat.Server == types.NewsletterServer {
			return
		}
		// REVOKE adalah pesan protokol, bukan bubble baru. Tangani sebelum cabang
		// IsFromMe agar penghapusan dari HP/WhatsApp Web kita maupun pelanggan
		// sama-sama menyinkron ke Inbox.
		if messageID := revokedMessageID(v); messageID != "" {
			if onMessageRevoke != nil {
				Go("onMessageRevoke", func() { onMessageRevoke(w.agentID, messageID) })
			}
			return
		}
		// Respon PLACEHOLDER_MESSAGE_RESEND: simpan/update saja, jangan jalankan AI.
		if v.UnavailableRequestID != "" && !v.Info.IsGroup {
			w.storeResentMessage(v)
			return
		}
		// Semua pesan grup (masuk maupun kiriman kita dari HP/WA Web) masuk jalur
		// khusus grup. Letakkan sebelum IsFromMe agar balasan grup dari perangkat lain
		// tidak hilang. Jalur ini tetap tidak pernah memicu AI/CRM/webhook personal.
		if v.Info.IsGroup {
			if onGroupMessage != nil {
				sender := v.Info.Sender
				if sender.Server == types.HiddenUserServer && !v.Info.SenderAlt.IsEmpty() {
					sender = v.Info.SenderAlt
				}
				senderPN := ""
				if sender.Server == types.DefaultUserServer {
					senderPN = sender.User
				}
				meta := GroupMessageMeta{
					GroupJID:   v.Info.Chat.String(),
					SenderJID:  v.Info.Sender.String(),
					SenderPN:   senderPN,
					SenderName: v.Info.PushName,
					Text:       groupMessageText(v),
					MessageID:  string(v.Info.ID),
					Timestamp:  v.Info.Timestamp,
					FromMe:     v.Info.IsFromMe,
				}
				Go("onGroupMessage", func() { onGroupMessage(w.agentID, meta) })
			}
			return
		}
		// Pesan dari akun sendiri (HP / WA Web lain / echo kiriman service).
		// Dulu: hanya DeviceSentMeta != nil → banyak balasan dari HP hilang.
		// Sekarang: terima semua IsFromMe non-grup; dedupe by wa_msg_id di OnWAOwnMessage
		// mencegah double-catat echo dari kiriman Inbox/API.
		if v.Info.IsFromMe {
			if onOwnMessage != nil {
				in, ok := w.extractIncoming(v)
				if ok {
					in.WAMsgID = string(v.Info.ID)
					recipient := v.Info.Chat
					if recipient.Server == types.HiddenUserServer && !v.Info.RecipientAlt.IsEmpty() {
						recipient = v.Info.RecipientAlt
					}
					in.Timestamp = w.orderedLiveMessageTime(recipient.User, v.Info.Timestamp)
					Go("onOwnMessage", func() { onOwnMessage(w.agentID, recipient, in) })
				}
			}
			return
		}
		// Read receipt adalah pilihan per nomor. Default manual agar pesan tidak langsung
		// centang biru hanya karena service menerima event di background.
		var agent models.Agent
		if database.DB.Select("auto_read").First(&agent, w.agentID).Error == nil && agent.AutoRead {
			if err := w.client.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, time.Now(), v.Info.Chat, v.Info.Sender); err != nil {
				log.Printf("WA agent %d gagal menandai pesan dibaca: %v", w.agentID, err)
			}
		}

		in, ok := w.extractIncoming(v)
		if !ok || onMessage == nil {
			return
		}
		in.WAMsgID = v.Info.ID
		in.WAMsgIDs = []string{string(v.Info.ID)}
		in.ChatJID = v.Info.Chat
		in.SenderJID = v.Info.Sender
		in.PushName = v.Info.PushName
		// Kontak modern bisa beralamat LID (privasi). Pakai nomor telepon asli (SenderAlt)
		// agar yang tersimpan & ditampilkan adalah nomor WA betulan, bukan angka LID.
		contact := v.Info.Sender
		if contact.Server == types.HiddenUserServer && !v.Info.SenderAlt.IsEmpty() {
			contact = v.Info.SenderAlt
		}
		in.Timestamp = w.orderedLiveMessageTime(contact.User, v.Info.Timestamp)
		Go("onMessage", func() { onMessage(w.agentID, contact, in) })
	}
}

// scheduleReconciliation men-debounce Connected dan OfflineSyncCompleted.
// WhatsApp biasanya mengirim kedua event berdekatan saat reconnect; hanya event
// terakhir yang perlu menjalankan rekonsiliasi Inbox.
func (w *waInstance) scheduleReconciliation(reason string, delay time.Duration) {
	if onConnected == nil {
		return
	}
	w.mu.Lock()
	w.reconcileSeq++
	seq := w.reconcileSeq
	w.mu.Unlock()
	Go("reconcile-"+reason, func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.mu.Lock()
		current := w.reconcileSeq == seq
		client := w.client
		connected := client != nil && client.IsConnected() && client.IsLoggedIn()
		if !current || !connected {
			w.mu.Unlock()
			return
		}
		if w.reconcileRunning {
			// OfflineSyncCompleted biasanya datang saat rekonsiliasi Connected
			// sudah aktif. Event history-nya tetap diproses oleh handler sendiri,
			// jadi jangan mengulang seluruh bootstrap/catch-up untuk koneksi yang
			// sama. Connected baru tetap meminta satu rerun terkoales.
			if reason != "offline-sync-completed" {
				w.reconcilePending = true
			}
			w.mu.Unlock()
			return
		}
		w.reconcileRunning = true
		w.mu.Unlock()

		runReason := reason
		for {
			startedAt := time.Now()
			log.Printf("WA agent %d: rekonsiliasi Inbox mulai (%s)", w.agentID, runReason)
			Safe("onConnected", func() { onConnected(w.agentID) })
			log.Printf("WA agent %d: rekonsiliasi Inbox selesai (%s, durasi=%s)",
				w.agentID, runReason, time.Since(startedAt).Round(time.Millisecond))
			w.mu.Lock()
			client = w.client
			connected = client != nil && client.IsConnected() && client.IsLoggedIn()
			if connected && w.reconcilePending {
				w.reconcilePending = false
				runReason = "reconnect-pending"
				w.mu.Unlock()
				continue
			}
			w.reconcilePending = false
			w.reconcileRunning = false
			w.mu.Unlock()
			return
		}
	})
}

func isOwnerReadReceipt(receipt *events.Receipt) bool {
	if receipt == nil {
		return false
	}
	if receipt.Type == types.ReceiptTypeReadSelf || receipt.Type == types.ReceiptTypePlayedSelf {
		return true
	}
	return receipt.IsFromMe && (receipt.Type == types.ReceiptTypeRead || receipt.Type == types.ReceiptTypePlayed)
}

// storeResentMessage menyimpan pesan yang diminta ulang via BuildUnavailableMessageRequest.
// Update stub quote bila wa_msg_id sudah ada; jangan memicu pipeline AI.
func (w *waInstance) storeResentMessage(v *events.Message) {
	in, ok := w.extractIncoming(v)
	if !ok {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" && in.MediaType != "" {
		text = historicalMediaLabel(in.MediaType, in.FileName)
	}
	if text == "" {
		return
	}
	num := ""
	chat := v.Info.Chat
	if chat.Server == types.HiddenUserServer && !v.Info.RecipientAlt.IsEmpty() && v.Info.IsFromMe {
		chat = v.Info.RecipientAlt
	}
	if v.Info.IsFromMe {
		num = chat.User
	} else {
		contact := v.Info.Sender
		if contact.Server == types.HiddenUserServer && !v.Info.SenderAlt.IsEmpty() {
			contact = v.Info.SenderAlt
		}
		num = contact.User
	}
	num = NormalizePhone(num)
	if num == "" {
		return
	}
	msgID := string(v.Info.ID)
	ts := v.Info.Timestamp
	// Jangan pakai time.Now() untuk created_at — itu menggeser pesan kemarin ke "Hari ini".
	// Bila timestamp WA kosong, pertahankan created_at lama (stub) atau relatif
	// terhadap tetangga terdekat. Setelah itu kirim lewat handler history kanonik:
	// jalur tersebut menangani unique ID, media metadata, tip monotonik, dan SSE.
	if ts.IsZero() || ts.Year() < 2020 {
		var existing models.ChatHistory
		if database.DB.Select("created_at").
			Where("agent_id = ? AND wa_msg_id = ?", w.agentID, msgID).
			First(&existing).Error == nil && !existing.CreatedAt.IsZero() {
			ts = existing.CreatedAt
		} else {
			var neighbor models.ChatHistory
			if database.DB.Select("created_at").
				Where("agent_id = ? AND sender = ?", w.agentID, num).
				Order("created_at DESC, id DESC").First(&neighbor).Error == nil {
				ts = neighbor.CreatedAt.Add(-time.Second)
			} else {
				ts = time.Now().Add(-time.Minute)
			}
		}
	}
	if onHistorySync == nil {
		log.Printf("WA agent %d: handler history belum siap; resend %s ditunda", w.agentID, msgID)
		return
	}
	imported, skipped, err := onHistorySync(w.agentID, []HistoricalMessage{{
		Sender: num, Text: text, FromMe: v.Info.IsFromMe,
		MediaType: in.MediaType, FileName: in.FileName, Mimetype: in.Mimetype,
		MediaMetadata: in.MediaMetadata,
		WAMsgID:       msgID, ReplyTo: in.ReplyTo, ReplyText: in.ReplyText,
		PushName: v.Info.PushName, Timestamp: ts,
	}})
	if err != nil {
		log.Printf("WA agent %d: gagal simpan resend %s: %v", w.agentID, msgID, err)
		return
	}
	log.Printf("WA agent %d: pesan resend diproses id=%s chat=%s imported=%d updated=%d",
		w.agentID, msgID, num, imported, skipped)
}

func newestHistoryMessages(messages []*waHistorySync.HistorySyncMsg, limit int) []*waHistorySync.HistorySyncMsg {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	selected := append([]*waHistorySync.HistorySyncMsg(nil), messages...)
	sort.SliceStable(selected, func(i, j int) bool {
		left := selected[i].GetMessage().GetMessageTimestamp()
		right := selected[j].GetMessage().GetMessageTimestamp()
		if left == right {
			return selected[i].GetMsgOrderID() > selected[j].GetMsgOrderID()
		}
		return left > right
	})
	return selected[:limit]
}

func (w *waInstance) processHistorySync(evt *events.HistorySync) {
	w.historySyncMu.Lock()
	defer w.historySyncMu.Unlock()
	if evt == nil || evt.Data == nil || onHistorySync == nil {
		return
	}

	now := time.Now()
	w.mu.Lock()
	if w.historyStatus.State != "syncing" {
		w.historyStatus = HistorySyncStatus{State: "syncing", Mode: strings.ToLower(evt.Data.GetSyncType().String()), StartedAt: &now}
	}
	w.historySeq++
	seq := w.historySeq
	if p := int(evt.Data.GetProgress()); p > w.historyStatus.Progress {
		w.historyStatus.Progress = p
	}
	w.mu.Unlock()

	batch := make([]HistoricalMessage, 0, 250)
	states := make([]HistoryChatState, 0, len(evt.Data.GetConversations()))
	revokedMessageIDs := make(map[string]struct{})
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		imported, skipped, err := onHistorySync(w.agentID, batch)
		w.mu.Lock()
		w.historyStatus.Imported += imported
		w.historyStatus.Skipped += skipped
		if err != nil {
			finished := time.Now()
			w.historyStatus.State = "failed"
			w.historyStatus.Error = err.Error()
			w.historyStatus.FinishedAt = &finished
		}
		w.mu.Unlock()
		batch = batch[:0]
		return err == nil
	}

	msgInChunk := 0
	groupMsgInChunk := 0
	for _, conv := range evt.Data.GetConversations() {
		chatJID, err := types.ParseJID(conv.GetID())
		if err != nil || chatJID.Server == types.BroadcastServer || chatJID.Server == types.NewsletterServer {
			continue
		}
		isGroup := chatJID.Server == types.GroupServer
		sender := ""
		if isGroup {
			sender = chatJID.String()
		} else {
			if pn := conv.GetPnJID(); pn != "" {
				if pnJID, parseErr := types.ParseJID(pn); parseErr == nil {
					sender = pnJID.User
				}
			}
			if sender == "" && chatJID.Server == types.DefaultUserServer {
				sender = chatJID.User
			}
			if sender == "" {
				if resolved, resolveErr := phoneNumberForJID(context.Background(), w.client, chatJID); resolveErr == nil {
					sender = resolved
				}
			}
		}
		sender = NormalizeInboxSender(sender)
		if sender == "" {
			continue
		}
		unread := int(conv.GetUnreadCount())
		lastMsgAt := time.Time{}
		if rawTimestamp := int64(conv.GetLastMsgTimestamp()); rawTimestamp > 0 {
			lastMsgAt = time.Unix(rawTimestamp, 0)
		}
		states = append(states, HistoryChatState{
			Sender: sender, UnreadCount: unread, MarkedUnread: conv.GetMarkedAsUnread(),
			Timestamp: lastMsgAt,
		})
		historyMessages := conv.GetMessages()
		if isGroup {
			remaining := groupHistoryPerChunkLimit - groupMsgInChunk
			if remaining <= 0 {
				continue
			}
			limit := groupHistoryPerChatLimit
			if remaining < limit {
				limit = remaining
			}
			historyMessages = newestHistoryMessages(historyMessages, limit)
		}
		for _, historyMsg := range historyMessages {
			webMsg := historyMsg.GetMessage()
			if webMsg == nil {
				continue
			}
			if messageID := historyRevokedMessageID(webMsg); messageID != "" {
				revokedMessageIDs[messageID] = struct{}{}
				continue
			}
			// Notifikasi protokol (ganti nama, perubahan grup, revoke, dll.)
			// ditandai WhatsApp lewat MessageStubType. Jangan menebak dari isi teks:
			// pesan customer yang pendek/nama-saja tetap merupakan chat yang valid.
			if isProtocolSystemNotification(webMsg) {
				continue
			}
			msgEvt, parseErr := w.client.ParseWebMessage(chatJID, webMsg)
			if parseErr != nil || msgEvt == nil || msgEvt.Info.ID == "" {
				continue
			}
			text, mediaType, fileName, mimetype, replyTo, replyText := historicalMessageContent(msgEvt.Message)
			if strings.TrimSpace(text) == "" && mediaType == "" {
				continue
			}
			// Bersihkan format ekspor WhatsApp ([timestamp] sender: message).
			text = cleanHistoryExportFormat(text)
			if mediaType != "" {
				label := historicalMediaLabel(mediaType, fileName)
				if strings.TrimSpace(text) == "" {
					text = label
				}
			}
			pushName := strings.TrimSpace(msgEvt.Info.PushName)
			if pushName == "" {
				pushName = strings.TrimSpace(webMsg.GetPushName())
			}
			if isGroup && !msgEvt.Info.IsFromMe {
				participant := msgEvt.Info.Sender
				if participant.Server == types.HiddenUserServer && !msgEvt.Info.SenderAlt.IsEmpty() {
					participant = msgEvt.Info.SenderAlt
				}
				participantPN := ""
				if participant.Server == types.DefaultUserServer {
					participantPN = participant.User
				}
				text = FormatGroupInboxText(text, pushName, participantPN)
			}
			batch = append(batch, HistoricalMessage{
				Sender: sender, Text: text, FromMe: msgEvt.Info.IsFromMe,
				MediaType: mediaType, FileName: fileName, Mimetype: mimetype,
				WAMsgID: string(msgEvt.Info.ID), ReplyTo: replyTo, ReplyText: replyText,
				PushName: pushName, Timestamp: msgEvt.Info.Timestamp,
			})
			msgInChunk++
			if isGroup {
				groupMsgInChunk++
			}
			if mediaType != "" {
				batch[len(batch)-1].MediaMetadata, _ = proto.Marshal(msgEvt.Message)
			}
			if len(batch) >= 250 && !flush() {
				return
			}
		}
	}
	if !flush() {
		return
	}
	// Terapkan tombstone setelah batch pesan selesai tersimpan. Urutan ini
	// mencegah revoke mendahului INSERT pesan asli pada HistorySync yang sama.
	if onMessageRevoke != nil {
		for messageID := range revokedMessageIDs {
			onMessageRevoke(w.agentID, messageID)
		}
	}
	if msgInChunk > 0 {
		log.Printf("WA agent %d: HistorySync %s memproses %d pesan dari %d percakapan",
			w.agentID, strings.ToLower(evt.Data.GetSyncType().String()), msgInChunk, len(evt.Data.GetConversations()))
	}
	// Terapkan posisi unread setelah seluruh pesan pada chunk tersimpan. Dengan
	// urutan ini handler dapat menentukan batas last_read_at yang persis.
	if onHistoryChatState != nil && len(states) > 0 {
		onHistoryChatState(w.agentID, states)
	}
	for _, state := range states {
		w.notifyHistoryChatState(state.Sender)
	}
	// HISTORY_SYNC_ON_DEMAND diserialkan oleh historyRequestMu. Karena respons
	// kosong atau hasil dengan identitas alias tidak selalu memiliki state sender
	// yang sama, satu event ON_DEMAND/FULL tetap merupakan acknowledgement valid
	// untuk request aktif setelah seluruh batch selesai ditulis ke database.
	syncType := evt.Data.GetSyncType()
	if syncType == waHistorySync.HistorySync_ON_DEMAND || syncType == waHistorySync.HistorySync_FULL {
		w.notifyAllHistoryWaiters()
	}

	w.mu.Lock()
	completeNow := w.historyStatus.Progress >= 100
	w.mu.Unlock()
	if completeNow {
		w.finishHistorySync(seq)
		return
	}
	// Sebagian perangkat tidak mengisi progress=100. Anggap selesai setelah tidak
	// ada chunk lanjutan selama beberapa detik, tanpa menahan event loop.
	Go("historySyncSettle", func() {
		time.Sleep(3 * time.Second)
		w.finishHistorySync(seq)
	})
}

// DownloadHistoricalMedia mengunduh lampiran HistorySync saat pertama kali dibuka.
func (w *waInstance) DownloadHistoricalMedia(ctx context.Context, metadata []byte) ([]byte, error) {
	if len(metadata) == 0 {
		return nil, errors.New("metadata media tidak tersedia")
	}
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected()
	w.mu.Unlock()
	if !connected {
		return nil, errors.New("WhatsApp belum terhubung")
	}
	var msg waProto.Message
	if err := proto.Unmarshal(metadata, &msg); err != nil {
		return nil, fmt.Errorf("metadata media rusak: %w", err)
	}
	switch {
	case msg.GetImageMessage() != nil:
		return client.Download(ctx, msg.GetImageMessage())
	case msg.GetDocumentMessage() != nil:
		return client.Download(ctx, msg.GetDocumentMessage())
	case msg.GetVideoMessage() != nil:
		return client.Download(ctx, msg.GetVideoMessage())
	case msg.GetAudioMessage() != nil:
		return client.Download(ctx, msg.GetAudioMessage())
	case msg.GetStickerMessage() != nil:
		return client.Download(ctx, msg.GetStickerMessage())
	default:
		return nil, errors.New("jenis media tidak didukung")
	}
}

// ProfilePictureURL mengambil thumbnail foto profil WhatsApp jika diizinkan privasi pengguna.
func (w *waInstance) ProfilePictureURL(ctx context.Context, sender string) (string, error) {
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	w.mu.Unlock()
	if !connected {
		return "", errors.New("WhatsApp belum terhubung")
	}
	jid, err := recipientJID(sender)
	if err != nil {
		return "", errors.New("nomor kontak kosong")
	}
	info, err := client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
	if err != nil {
		return "", err
	}
	if info == nil || strings.TrimSpace(info.URL) == "" {
		return "", errors.New("foto profil tidak tersedia")
	}
	return info.URL, nil
}

func (w *waInstance) finishHistorySync(seq uint64) {
	w.mu.Lock()
	if w.historySeq != seq || w.historyStatus.State != "syncing" {
		w.mu.Unlock()
		return
	}
	now := time.Now()
	sender := w.historyStatus.Sender
	w.historyStatus.State = "completed"
	w.historyStatus.Progress = 100
	w.historyStatus.FinishedAt = &now
	agentID := w.agentID
	imported := w.historyStatus.Imported
	w.mu.Unlock()
	// Evaluasi di luar lock (query DB).
	stale := false
	if sender != "" {
		stale = ChatPreviewStale(agentID, sender)
	}
	w.mu.Lock()
	if w.historySeq == seq {
		w.historyStatus.StillStale = stale
		w.historyStatus.Imported = imported
		w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
	}
	w.mu.Unlock()
}

func historicalMessageContent(m *waProto.Message) (text, mediaType, fileName, mimetype, replyTo, replyText string) {
	if m == nil {
		return
	}
	// Buka wrapper FutureProof (ephemeral/view-once/dll) bila ParseWebMessage
	// tidak sempat unwrap (path manual/unit test).
	m = unwrapHistoryMessage(m)
	if t := m.GetConversation(); t != "" {
		return t, "", "", "", "", ""
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		ci := ext.GetContextInfo()
		return ext.GetText(), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if img := m.GetImageMessage(); img != nil {
		ci := img.GetContextInfo()
		return img.GetCaption(), "image", "", img.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		ci := doc.GetContextInfo()
		return doc.GetCaption(), "document", doc.GetFileName(), doc.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if vid := m.GetVideoMessage(); vid != nil {
		ci := vid.GetContextInfo()
		return vid.GetCaption(), "video", "", vid.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if aud := m.GetAudioMessage(); aud != nil {
		ci := aud.GetContextInfo()
		return "", "audio", "", aud.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if sticker := m.GetStickerMessage(); sticker != nil {
		ci := sticker.GetContextInfo()
		return "", "sticker", "", sticker.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if loc := m.GetLocationMessage(); loc != nil {
		label := strings.TrimSpace(loc.GetName())
		if label == "" {
			label = "Lokasi"
		}
		ci := loc.GetContextInfo()
		return fmt.Sprintf("📍 %s\nhttps://maps.google.com/?q=%f,%f", label, loc.GetDegreesLatitude(), loc.GetDegreesLongitude()), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if live := m.GetLiveLocationMessage(); live != nil {
		ci := live.GetContextInfo()
		return fmt.Sprintf("📍 Lokasi live\nhttps://maps.google.com/?q=%f,%f", live.GetDegreesLatitude(), live.GetDegreesLongitude()), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if text, _, reply, ok := interactiveReplyText(m); ok {
		return text, "", "", "", reply, ""
	}
	return
}

// unwrapHistoryMessage membuka wrapper FutureProof yang umum di history/export.
func unwrapHistoryMessage(m *waProto.Message) *waProto.Message {
	if m == nil {
		return m
	}
	for i := 0; i < 4; i++ {
		switch {
		case m.GetDeviceSentMessage().GetMessage() != nil:
			m = m.GetDeviceSentMessage().GetMessage()
		case m.GetEphemeralMessage().GetMessage() != nil:
			m = m.GetEphemeralMessage().GetMessage()
		case m.GetViewOnceMessage().GetMessage() != nil:
			m = m.GetViewOnceMessage().GetMessage()
		case m.GetViewOnceMessageV2().GetMessage() != nil:
			m = m.GetViewOnceMessageV2().GetMessage()
		case m.GetViewOnceMessageV2Extension().GetMessage() != nil:
			m = m.GetViewOnceMessageV2Extension().GetMessage()
		case m.GetDocumentWithCaptionMessage().GetMessage() != nil:
			m = m.GetDocumentWithCaptionMessage().GetMessage()
		case m.GetEditedMessage().GetMessage() != nil:
			m = m.GetEditedMessage().GetMessage()
		default:
			return m
		}
	}
	return m
}

func historicalMediaLabel(mediaType, fileName string) string {
	switch mediaType {
	case "image":
		return "📷 Foto dari riwayat WhatsApp"
	case "sticker":
		return "🌟 Stiker dari riwayat WhatsApp"
	case "video":
		return "🎥 Video dari riwayat WhatsApp"
	case "audio":
		return "🎵 Audio dari riwayat WhatsApp"
	case "document":
		if fileName != "" {
			return "📄 " + fileName
		}
		return "📄 Dokumen dari riwayat WhatsApp"
	default:
		return "Pesan dari riwayat WhatsApp"
	}
}

// historyExportPattern cocokkan baris ekspor WhatsApp: [HH.MM, DD/MM/YYYY] Nama/No: pesan
var historyExportPattern = regexp.MustCompile(`^\[[\d.,/:]+\]\s*[^:]+:\s*`)

// cleanHistoryExportFormat menghapus prefix ekspor WhatsApp bila terdeteksi.
// Hanya diterapkan bila teks dimulai dengan pola "[timestamp] sender: ".
func cleanHistoryExportFormat(text string) string {
	return historyExportPattern.ReplaceAllString(text, "")
}

func inboxPresenceSender(client *whatsmeow.Client, source types.MessageSource) string {
	if source.IsGroup || source.Chat.Server == types.GroupServer {
		return strings.TrimSpace(source.Chat.String())
	}
	jid := source.Sender
	if jid.IsEmpty() {
		jid = source.Chat
	}
	if jid.Server == types.HiddenUserServer && !source.SenderAlt.IsEmpty() {
		jid = source.SenderAlt
	}
	number := ""
	if resolved, err := phoneNumberForJID(context.Background(), client, jid); err == nil {
		number = resolved
	}
	return NormalizePhone(number)
}

func protocolRevokeTargetID(message *waProto.Message) string {
	message = unwrapHistoryMessage(message)
	if message == nil {
		return ""
	}
	protocolMessage := message.GetProtocolMessage()
	if protocolMessage == nil || protocolMessage.GetType() != waProto.ProtocolMessage_REVOKE {
		return ""
	}
	return strings.TrimSpace(protocolMessage.GetKey().GetID())
}

func revokedMessageID(event *events.Message) string {
	if event == nil {
		return ""
	}
	if messageID := protocolRevokeTargetID(event.Message); messageID != "" {
		return messageID
	}
	if messageID := protocolRevokeTargetID(event.RawMessage); messageID != "" {
		return messageID
	}
	if event.Info.Edit == types.EditAttributeSenderRevoke ||
		event.Info.Edit == types.EditAttributeAdminRevoke {
		return strings.TrimSpace(string(event.Info.MsgMetaInfo.TargetID))
	}
	return ""
}

func historyRevokedMessageID(message *waWeb.WebMessageInfo) string {
	if message == nil {
		return ""
	}
	switch message.GetMessageStubType() {
	case waWeb.WebMessageInfo_REVOKE, waWeb.WebMessageInfo_ADMIN_REVOKE:
		return strings.TrimSpace(message.GetKey().GetID())
	default:
		return ""
	}
}

// isProtocolSystemNotification hanya memakai metadata protokol WhatsApp.
// UNKNOWN (0) adalah pesan biasa; semua stub non-zero adalah notifikasi/status
// protokol dan tidak boleh disamarkan sebagai bubble chat customer.
func isProtocolSystemNotification(msg *waWeb.WebMessageInfo) bool {
	return msg != nil && msg.GetMessageStubType() != waWeb.WebMessageInfo_UNKNOWN
}

func (w *waInstance) GetQR() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.qrCode
}

// GetQRTTL = sisa detik sebelum kode QR saat ini diputar whatsmeow (0 bila bukan status qr).
func (w *waInstance) GetQRTTL() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != "qr" || w.qrExpiry.IsZero() {
		return 0
	}
	if s := int(time.Until(w.qrExpiry).Seconds()); s > 0 {
		return s
	}
	return 0
}

func (w *waInstance) GetStatus() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Kalau cache bilang "connected" tapi socket sebenarnya turun, laporkan "connecting"
	// supaya dashboard tidak menipu "Online" padahal tidak bisa kirim.
	if w.status == "connected" && (w.client == nil || !w.client.IsConnected()) {
		return "connecting"
	}
	return w.status
}

// IsConnected melaporkan apakah socket WA benar-benar hidup & ter-login (bukan sekadar
// status cache). Dipakai broadcast: w.status bisa basi "connected" walau koneksi sudah turun.
func (w *waInstance) IsConnected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.client != nil && w.client.IsConnected() && w.client.IsLoggedIn()
}

func (w *waInstance) HistorySyncStatus() HistorySyncStatus {
	w.mu.Lock()
	status := w.historyStatus
	agentID := w.agentID
	w.mu.Unlock()
	// Status completed hanya ditahan sebentar untuk banner FE, lalu idle.
	if status.State == "completed" && status.FinishedAt != nil && time.Since(*status.FinishedAt) > 20*time.Second {
		status = HistorySyncStatus{State: "idle"}
	}
	if status.State == "" {
		status.State = "idle"
	}
	// Lengkapi status akhir dengan deteksi celah preview vs WhatsApp.
	if (status.State == "completed" || status.State == "failed") && status.Sender != "" {
		status.StillStale = ChatPreviewStale(agentID, status.Sender)
		status.Message = historySyncUserMessage(status)
	}
	return status
}

// ChatPreviewStale true bila timestamp pesan resmi WhatsApp lebih baru dari
// pesan lokal. Fungsi ini read-only: bukti celah tidak boleh "disembuhkan"
// dengan menurunkan last_msg_at ke data lokal.
func ChatPreviewStale(agentID uint, sender string) bool {
	sender = NormalizeInboxSender(sender)
	if agentID == 0 || sender == "" {
		return false
	}
	localMax, hasLocal := latestLocalMessageTime(agentID, sender)
	var state models.InboxReadState
	if database.DB.Select("last_msg_at").
		Where("agent_id = ? AND sender = ?", agentID, sender).
		First(&state).Error != nil {
		return false
	}
	if state.LastMsgAt == nil || state.LastMsgAt.IsZero() {
		return false
	}
	if !hasLocal {
		return true
	}
	return state.LastMsgAt.After(localMax)
}

func latestLocalMessageTime(agentID uint, sender string) (time.Time, bool) {
	var maxCreatedAt sql.NullTime
	err := database.DB.Raw(
		`SELECT MAX(created_at) FROM chat_histories WHERE agent_id = ? AND sender = ?`,
		agentID, sender,
	).Row().Scan(&maxCreatedAt)
	if err != nil || !maxCreatedAt.Valid || maxCreatedAt.Time.IsZero() {
		return time.Time{}, false
	}
	return maxCreatedAt.Time, true
}

// ChatWATipTime mengembalikan waktu pesan terakhir resmi (last_msg_at).
// Tidak memakai whats_app_state_at agar catch-up tidak dikejar tip baca palsu.
func ChatWATipTime(agentID uint, sender string) time.Time {
	var state models.InboxReadState
	if database.DB.Select("last_msg_at").
		Where("agent_id = ? AND sender = ?", agentID, sender).
		First(&state).Error != nil {
		return time.Time{}
	}
	if state.LastMsgAt == nil || state.LastMsgAt.IsZero() {
		return time.Time{}
	}
	return *state.LastMsgAt
}

func historySyncUserMessage(s HistorySyncStatus) string {
	if s.State == "failed" {
		if strings.TrimSpace(s.Error) != "" {
			return s.Error
		}
		return "Sinkronisasi gagal. Pastikan HP online dan WhatsApp terbuka."
	}
	if s.Imported > 0 && s.StillStale {
		return fmt.Sprintf("%d pesan ditambahkan. Masih ada yang lebih baru di WhatsApp — buka chat di HP lalu sinkron lagi.", s.Imported)
	}
	if s.Imported > 0 {
		return fmt.Sprintf("%d pesan ditambahkan dari WhatsApp.", s.Imported)
	}
	if s.StillStale {
		// Pesan lebih singkat — frontend juga auto-sembunyikan bila preview tidak stale.
		return "Sebagian pesan mungkin belum masuk. Buka chat di HP (online) lalu sinkron lagi."
	}
	return "Sinkronisasi selesai. WhatsApp tidak mengirim pesan tambahan."
}

// RequestHistorySync meminta satu halaman pesan yang lebih lama dari primary device.
// Anchor wajib berasal dari pesan nyata yang sudah tersimpan agar WhatsApp tahu posisi awal.
func (w *waInstance) RequestHistorySync(anchor types.MessageInfo, count int, sender string) error {
	w.historyRequestMu.Lock()
	defer w.historyRequestMu.Unlock()
	return w.sendHistoryOnDemand(anchor, count, sender, "on_demand", false)
}

// RequestUnavailableMessage meminta HP mengirim ulang salinan pesan yang hilang
// (whatsmeow: BuildUnavailableMessageRequest / PLACEHOLDER_MESSAGE_RESEND).
// Respons datang sebagai events.Message biasa (dengan UnavailableRequestID).
// docs: https://pkg.go.dev/go.mau.fi/whatsmeow#Client.BuildUnavailableMessageRequest
//
// fromMe=true → MessageKey.FromMe (pesan CS/kita).
// fromMe=false → pesan customer (sender = chat user).
func (w *waInstance) RequestUnavailableMessage(chatUser, msgID string, fromMe bool) error {
	chatUser = NormalizePhone(strings.TrimSpace(chatUser))
	msgID = strings.TrimSpace(msgID)
	if chatUser == "" || msgID == "" {
		return errors.New("chat/msg id kosong")
	}
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	w.mu.Unlock()
	if !connected {
		return errors.New("WhatsApp belum tersambung")
	}
	chat := types.NewJID(chatUser, types.DefaultUserServer)
	var senderJID types.JID
	if fromMe {
		senderJID = types.EmptyJID
	} else {
		senderJID = chat
	}
	_, err := client.SendPeerMessage(context.Background(), client.BuildUnavailableMessageRequest(chat, senderJID, msgID))
	if err != nil {
		return err
	}
	log.Printf("WA agent %d: minta ulang pesan hilang chat=%s id=%s fromMe=%v", w.agentID, chatUser, msgID, fromMe)
	return nil
}

// EnsureMissingQuotedMessages memindai reply_to di thread yang belum punya baris
// chat_histories. Membuat stub dari reply_text agar UI langsung menampilkan bubble,
// lalu minta HP mengirim body penuh via PLACEHOLDER_MESSAGE_RESEND.
//
// Juga minta ulang body+ContextInfo untuk pesan customer yang sudah ada tapi
// reply_to masih kosong (stub teks-only dari quote CS, tanpa nested quote).
func (w *waInstance) EnsureMissingQuotedMessages(sender string, limit int) (created, requested int) {
	sender = NormalizePhone(strings.TrimSpace(sender))
	if sender == "" {
		return 0, 0
	}
	if limit <= 0 || limit > 40 {
		limit = 20
	}
	type quoteRow struct {
		ReplyTo   string
		ReplyText string
		CreatedAt time.Time
		FromHuman bool
	}
	var quotes []quoteRow
	database.DB.Raw(`
		SELECT reply_to, reply_text, created_at, from_human
		FROM chat_histories
		WHERE agent_id = ? AND sender = ?
			AND reply_to IS NOT NULL AND TRIM(reply_to) <> ''
		ORDER BY created_at DESC
		LIMIT ?
	`, w.agentID, sender, limit*3).Scan(&quotes)

	seen := map[string]bool{}
	for _, q := range quotes {
		rt := strings.TrimSpace(q.ReplyTo)
		if rt == "" || seen[rt] {
			continue
		}
		seen[rt] = true
		var n int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ? AND wa_msg_id = ?", w.agentID, sender, rt).
			Count(&n)
		if n > 0 {
			continue
		}
		preview := strings.TrimSpace(q.ReplyText)
		if preview == "" {
			preview = "Pesan dikutip (mengambil dari HP…)"
		}
		// CS (from_human) mengutip customer → stub customer (FromMe=false).
		// Customer mengutip CS → stub CS (FromMe=true).
		stubFromMe := !q.FromHuman
		createdAt := q.CreatedAt.Add(-time.Second)
		if createdAt.IsZero() || createdAt.Year() < 2000 {
			createdAt = time.Now().Add(-time.Minute)
		}
		row := models.ChatHistory{
			AgentID: w.agentID, Sender: sender,
			WAMsgID: rt, DeliveryStatus: "sent", CreatedAt: createdAt,
			FromHuman: stubFromMe,
		}
		if stubFromMe {
			row.Reply = preview
		} else {
			row.Message = preview
		}
		if err := database.DB.Create(&row).Error; err != nil {
			log.Printf("WA agent %d: gagal stub quote %s: %v", w.agentID, rt, err)
			continue
		}
		created++
		if err := w.RequestUnavailableMessage(sender, rt, stubFromMe); err != nil {
			log.Printf("WA agent %d: request unavailable %s: %v", w.agentID, rt, err)
		} else {
			requested++
		}
		if created >= limit {
			break
		}
	}
	// Pass 2: pesan customer yang sudah ada (sering stub dari pass 1) tapi belum
	// punya reply_to — minta ulang ke HP agar ContextInfo (nested quote) terisi.
	requested += w.refreshMissingReplyContext(sender, limit, seen)
	return created, requested
}

// refreshMissingReplyContext meminta ulang pesan yang di-quote orang lain (ada
// baris dengan reply_to = wa_msg_id-nya) tapi baris itu sendiri belum punya
// reply_to. Kasus umum: stub teks-only "Update nya…" tanpa nested quote "Source code".
func (w *waInstance) refreshMissingReplyContext(sender string, limit int, already map[string]bool) (requested int) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 12 {
		limit = 12
	}
	type row struct {
		WAMsgID   string
		FromHuman bool
	}
	var rows []row
	// Target: pesan yang dikutip (quoter.reply_to = target.wa_msg_id) tapi target
	// belum menyimpan context reply-nya sendiri.
	database.DB.Raw(`
		SELECT t.wa_msg_id, t.from_human
		FROM chat_histories t
		INNER JOIN chat_histories q
			ON q.agent_id = t.agent_id AND q.sender = t.sender
			AND q.reply_to = t.wa_msg_id
		WHERE t.agent_id = ? AND t.sender = ?
			AND t.wa_msg_id IS NOT NULL AND TRIM(t.wa_msg_id) <> ''
			AND (t.reply_to IS NULL OR TRIM(t.reply_to) = '')
			AND (t.reply_text IS NULL OR TRIM(t.reply_text) = '')
		GROUP BY t.wa_msg_id, t.from_human
		ORDER BY MAX(t.created_at) DESC
		LIMIT ?
	`, w.agentID, sender, limit).Scan(&rows)

	if already == nil {
		already = map[string]bool{}
	}
	for _, r := range rows {
		id := strings.TrimSpace(r.WAMsgID)
		if id == "" || already[id] {
			continue
		}
		already[id] = true
		if err := w.RequestUnavailableMessage(sender, id, r.FromHuman); err != nil {
			log.Printf("WA agent %d: refresh context %s: %v", w.agentID, id, err)
			continue
		}
		requested++
	}
	if requested > 0 {
		log.Printf("WA agent %d: minta ulang context quote chat=%s n=%d", w.agentID, sender, requested)
	}
	return requested
}

// RequestChatCatchUp meminta ulang riwayat satu chat dari HP.
//
// HISTORY_SYNC_ON_DEMAND mengembalikan pesan SEBELUM anchor. Untuk celah:
//   - ke belakang/tengah: anchor = pesan lokal terbaru (dan anchor pesan ke-2)
//   - ke depan (tip): cursor timestamp di masa "setelah" last activity WA + ID sintetik
//   - last resort: FULL_HISTORY_SYNC_ON_DEMAND dari ~14 hari ke belakang
func (w *waInstance) RequestChatCatchUp(sender string, count int, waLastHint time.Time) error {
	w.historyRequestMu.Lock()
	defer w.historyRequestMu.Unlock()
	err := w.requestChatCatchUpLocked(sender, count, waLastHint, true)
	if err != nil {
		w.failHistoryRequest(sender, err)
	}
	return err
}

// RequestRecentChatCatchUp adalah jalur ringan untuk rekonsiliasi otomatis setelah
// reconnect. Hanya satu jendela pesan terbaru yang diminta dan job dilewati bila
// sinkronisasi manual sedang aktif. Perbaikan timeline/deep history tetap tersedia
// melalui RequestDeepHistorySync saat operator memang memintanya.
func (w *waInstance) RequestRecentChatCatchUp(sender string, count int, waLastHint time.Time) error {
	if !w.historyRequestMu.TryLock() {
		return ErrHistorySyncBusy
	}
	defer w.historyRequestMu.Unlock()
	return w.requestRecentChatCatchUpLocked(sender, count, waLastHint)
}

func (w *waInstance) requestRecentChatCatchUpLocked(sender string, count int, waLastHint time.Time) error {
	sender = NormalizeInboxSender(sender)
	if sender == "" {
		return errors.New("alamat percakapan kosong")
	}
	if count <= 0 || count > 100 {
		count = 100
	}
	chat, err := recipientJID(sender)
	if err != nil {
		return err
	}
	if tip := ChatWATipTime(w.agentID, sender); tip.After(waLastHint) {
		waLastHint = tip
	}
	// Untuk grup yang sudah memiliki riwayat lokal, gunakan stanza ID asli.
	// BuildHistorySyncRequest resmi mensyaratkan pesan acuan nyata dan akan
	// mengembalikan hingga count pesan sebelum acuan tersebut. Ini jauh lebih
	// dapat diandalkan daripada ID tip sintetis pada primary device tertentu.
	if IsGroupJID(sender) {
		var newest models.ChatHistory
		if err := database.DB.
			Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL", w.agentID, sender).
			Order("created_at DESC, id DESC").
			First(&newest).Error; err == nil {
			fromMe := strings.TrimSpace(newest.Message) == "" && strings.TrimSpace(newest.Reply) != ""
			anchor := types.MessageInfo{
				MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe},
				ID:            types.MessageID(newest.WAMsgID),
				Timestamp:     newest.CreatedAt,
			}
			return w.sendHistoryOnDemand(anchor, count, sender, "recent_group", false)
		}
	}
	now := time.Now()
	tipAt := now.Add(2 * time.Minute)
	if !waLastHint.IsZero() &&
		waLastHint.After(now.Add(-24*time.Hour)) &&
		waLastHint.Before(now.Add(5*time.Minute)) {
		tipAt = waLastHint.Add(2 * time.Minute)
	}
	tip := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			IsFromMe: false,
		},
		ID:        types.MessageID(fmt.Sprintf("3EB0AUTO%X", time.Now().UnixNano())),
		Timestamp: tipAt,
	}
	return w.sendHistoryOnDemand(tip, count, sender, "auto_catch_up", true)
}

// ReserveRecentHistorySync mengambil slot sebelum endpoint mengembalikan 202.
// Dipakai grup agar sinkronisasi terbaru berjalan di background dan klik Inbox
// tidak menunggu timeout perangkat utama.
func (w *waInstance) ReserveRecentHistorySync(sender string) (HistorySyncStatus, error) {
	sender = NormalizeInboxSender(sender)
	if sender == "" {
		return w.HistorySyncStatus(), errors.New("alamat percakapan kosong")
	}
	if !w.historyRequestMu.TryLock() {
		return w.HistorySyncStatus(), ErrHistorySyncBusy
	}

	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		w.historyRequestMu.Unlock()
		return w.HistorySyncStatus(), errors.New("WhatsApp belum tersambung")
	}
	now := time.Now()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "recent", Sender: sender, StartedAt: &now,
		Message: "Mengambil hingga 100 pesan grup terbaru dari HP…",
	}
	status := w.historyStatus
	w.mu.Unlock()
	return status, nil
}

func (w *waInstance) RunReservedRecentHistorySync(sender string) error {
	defer w.historyRequestMu.Unlock()
	err := w.requestRecentChatCatchUpLocked(sender, groupHistoryPerChatLimit, ChatWATipTime(w.agentID, sender))
	if err != nil {
		w.failHistoryRequest(sender, err)
	}
	return err
}

func (w *waInstance) requestChatCatchUpLocked(sender string, count int, waLastHint time.Time, allowFullHistory bool) error {
	sender = NormalizePhone(strings.TrimSpace(sender))
	if sender == "" {
		return errors.New("nomor percakapan kosong")
	}
	count = clampHistoryOnDemandCount(count)
	// Gabungkan hint dengan whats_app_state_at (sering lebih akurat untuk ujung chat).
	if tip := ChatWATipTime(w.agentID, sender); tip.After(waLastHint) {
		waLastHint = tip
	}
	chat := types.NewJID(sender, types.DefaultUserServer)
	var recent []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND wa_msg_id <> ''", w.agentID, sender).
		Order("created_at DESC, id DESC").Limit(5).Find(&recent)
	if len(recent) == 0 {
		return errHistoryAnchorUnavailable
	}
	var messagesBefore int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&messagesBefore)
	newest := recent[0]
	fromMe := strings.TrimSpace(newest.Message) == "" && strings.TrimSpace(newest.Reply) != ""
	var syncErr error

	// Pass 1: lubang sebelum pesan lokal terbaru.
	info := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe},
		ID:            types.MessageID(newest.WAMsgID),
		Timestamp:     newest.CreatedAt,
	}
	if err := w.sendHistoryOnDemand(info, count, sender, "catch_up", true); err != nil {
		return err
	}

	// Pass 1b: lubang tepat sebelum pesan lokal ke-2 (sering berisi "masih seperti ini mas"
	// yang hilang di antara cluster 12:34).
	if len(recent) >= 2 {
		sec := recent[1]
		secFromMe := strings.TrimSpace(sec.Message) == "" && strings.TrimSpace(sec.Reply) != ""
		secInfo := types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, IsFromMe: secFromMe},
			ID:            types.MessageID(sec.WAMsgID),
			Timestamp:     sec.CreatedAt,
		}
		if err := w.sendHistoryOnDemand(secInfo, count, sender, "catch_up_mid", true); err != nil {
			syncErr = errors.Join(syncErr, err)
			if errors.Is(err, errHistoryDeviceNoResponse) {
				return syncErr
			}
		}
	}

	// Pass 2: ujung chat — cursor "setelah" tip WA / sekarang.
	tipTS := time.Now().Add(2 * time.Minute)
	if !waLastHint.IsZero() && waLastHint.After(newest.CreatedAt) {
		tipTS = waLastHint.Add(10 * time.Minute)
	}
	tip := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: false},
		ID:            types.MessageID(fmt.Sprintf("3EB0TIP%X", time.Now().UnixNano())),
		Timestamp:     tipTS,
	}
	if err := w.sendHistoryOnDemand(tip, count, sender, "catch_up_tip", true); err != nil {
		syncErr = errors.Join(syncErr, err)
		if errors.Is(err, errHistoryDeviceNoResponse) {
			return syncErr
		}
	}

	// Pass 3: full history on-demand bila masih ada celah ujung (whats_app_state_at /
	// last_msg_at lebih baru) ATAU tidak ada import sama sekali di pass sebelumnya.
	var messagesAfterTip int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&messagesAfterTip)
	stillGap := ChatPreviewStale(w.agentID, sender) || (!waLastHint.IsZero() && waLastHint.After(newest.CreatedAt.Add(2*time.Minute)))
	if allowFullHistory && (stillGap || messagesAfterTip <= messagesBefore) {
		from := newest.CreatedAt.Add(-30 * 24 * time.Hour)
		if from.Before(time.Now().Add(-90 * 24 * time.Hour)) {
			from = time.Now().Add(-90 * 24 * time.Hour)
		}
		if err := w.sendFullHistoryOnDemand(sender, from, 45); err != nil {
			log.Printf("WA agent %d: full history catch-up %s: %v", w.agentID, sender, err)
			syncErr = errors.Join(syncErr, err)
		}
	}

	// Deep sync menjalankan quote/timeline repair satu kali setelah pagination.
	// Catch-up standalone tetap memperoleh fallback lengkap di jalur ini.
	var repairErr error
	if allowFullHistory {
		// Pass 4: pesan yang di-quote tapi barisnya hilang (reply_to tanpa target).
		// Stub + PLACEHOLDER_MESSAGE_RESEND — pola resmi whatsmeow untuk pesan missing.
		if c, r := w.EnsureMissingQuotedMessages(sender, 15); c > 0 || r > 0 {
			log.Printf("WA agent %d: quote missing chat=%s stub=%d requested=%d", w.agentID, sender, c, r)
		}
		// Pass 5: isi lubang TENGAH timeline + koreksi timestamp stub.
		_, _, repairErr = w.requestTimelineRepairLocked(sender, 12)
	}
	finalErr := errors.Join(syncErr, repairErr)
	var messagesAfter int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&messagesAfter)
	imported := int(messagesAfter - messagesBefore)
	if imported < 0 {
		imported = 0
	}
	stillStale := ChatPreviewStale(w.agentID, sender)
	w.mu.Lock()
	w.historyStatus.Imported = imported
	w.historyStatus.StillStale = stillStale
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
	w.mu.Unlock()
	if finalErr != nil {
		w.failHistoryRequest(sender, finalErr)
	}
	return finalErr
}

// chatMsgFromMe true bila baris Inbox adalah pesan dari kita (CS/bot).
func chatMsgFromMe(m models.ChatHistory) bool {
	if m.FromHuman {
		return true
	}
	return strings.TrimSpace(m.Message) == "" && strings.TrimSpace(m.Reply) != ""
}

// RequestTimelineRepair memperbaiki timeline agar mendekati WhatsApp Web:
//  1. HISTORY_SYNC_ON_DEMAND di setiap celah ≥45 menit (pesan tengah yang hilang)
//  2. PLACEHOLDER_MESSAGE_RESEND untuk stub quote / placeholder / timestamp mencurigakan
//     (cluster ±3 detik dengan balasan CS — sering hasil stub CreatedAt = CS−1s)
func (w *waInstance) RequestTimelineRepair(sender string, maxRequests int) (historyReqs, unavailableReqs int) {
	w.historyRequestMu.Lock()
	defer w.historyRequestMu.Unlock()
	historyReqs, unavailableReqs, err := w.requestTimelineRepairLocked(sender, maxRequests)
	if err != nil {
		log.Printf("WA agent %d: timeline repair chat=%s tidak lengkap: %v", w.agentID, sender, err)
		w.failHistoryRequest(sender, fmt.Errorf("perbaikan timeline tidak lengkap: %w", err))
	}
	return historyReqs, unavailableReqs
}

func (w *waInstance) requestTimelineRepairLocked(sender string, maxRequests int) (historyReqs, unavailableReqs int, syncErr error) {
	sender = NormalizePhone(strings.TrimSpace(sender))
	if sender == "" {
		return 0, 0, errors.New("nomor percakapan kosong")
	}
	if maxRequests <= 0 || maxRequests > 25 {
		maxRequests = 12
	}
	var rows []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL", w.agentID, sender).
		Order("created_at ASC, id ASC").Find(&rows)
	if len(rows) == 0 {
		return 0, 0, nil
	}
	chat := types.NewJID(sender, types.DefaultUserServer)
	seenHist := map[string]bool{}
	// --- Pass A: history di setiap gap besar (pesan "Siap"/"Kirain" di antara cluster) ---
	for i := 1; i < len(rows) && historyReqs < maxRequests; i++ {
		gap := rows[i].CreatedAt.Sub(rows[i-1].CreatedAt)
		if gap < 45*time.Minute {
			continue
		}
		anchor := rows[i]
		id := strings.TrimSpace(anchor.WAMsgID)
		if id == "" || seenHist[id] {
			continue
		}
		seenHist[id] = true
		fromMe := chatMsgFromMe(anchor)
		info := types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe},
			ID:            types.MessageID(id),
			Timestamp:     anchor.CreatedAt,
		}
		// count besar: isi lubang tengah (bukan cuma 50).
		if err := w.sendHistoryOnDemand(info, historyOnDemandPageSize, sender, fmt.Sprintf("gap_fill_%d", historyReqs+1), false); err != nil {
			log.Printf("WA agent %d: gap-fill %s before %s: %v", w.agentID, sender, id, err)
			syncErr = errors.Join(syncErr, err)
			break
		}
		historyReqs++
		log.Printf("WA agent %d: gap-fill chat=%s before=%s gap=%s", w.agentID, sender, id, gap.Round(time.Minute))
	}

	// --- Pass B: minta ulang body+timestamp asli untuk stub/quote mencurigakan ---
	seenUnavail := map[string]bool{}
	requestUnavail := func(msgID string, fromMe bool) {
		msgID = strings.TrimSpace(msgID)
		if msgID == "" || seenUnavail[msgID] || unavailableReqs >= maxRequests {
			return
		}
		seenUnavail[msgID] = true
		if err := w.RequestUnavailableMessage(sender, msgID, fromMe); err != nil {
			log.Printf("WA agent %d: repair unavailable %s: %v", w.agentID, msgID, err)
			syncErr = errors.Join(syncErr, err)
			return
		}
		unavailableReqs++
	}
	for i, row := range rows {
		text := strings.TrimSpace(row.Message)
		if text == "" {
			text = strings.TrimSpace(row.Reply)
		}
		// Placeholder quote / body belum lengkap.
		if strings.Contains(text, "Pesan dikutip") || strings.Contains(text, "mengambil dari HP") {
			requestUnavail(row.WAMsgID, chatMsgFromMe(row))
			continue
		}
		// Stub quote: customer message created ≈ CS reply that quotes it (≤3s).
		// Timestamp sering = CS.CreatedAt−1s → geser ke "Hari ini" padahal aslinya kemarin.
		if row.FromHuman {
			continue
		}
		rt := strings.TrimSpace(row.WAMsgID)
		if rt == "" {
			continue
		}
		// Dicari sebagai target quote?
		var quoter models.ChatHistory
		if database.DB.Where("agent_id = ? AND sender = ? AND reply_to = ?", w.agentID, sender, rt).
			Order("created_at ASC").First(&quoter).Error != nil {
			// Juga: berdempetan ±3s dengan pesan CS berikutnya (pola stub EnsureMissing).
			if i+1 < len(rows) {
				next := rows[i+1]
				if chatMsgFromMe(next) && next.CreatedAt.Sub(row.CreatedAt) >= 0 && next.CreatedAt.Sub(row.CreatedAt) <= 3*time.Second {
					requestUnavail(row.WAMsgID, false)
					// Sekalian minta ulang balasan CS agar timestamp-nya ikut dikoreksi.
					requestUnavail(next.WAMsgID, true)
				}
			}
			continue
		}
		dt := quoter.CreatedAt.Sub(row.CreatedAt)
		if dt < 0 {
			dt = -dt
		}
		if dt <= 5*time.Second {
			requestUnavail(row.WAMsgID, false)
			requestUnavail(quoter.WAMsgID, chatMsgFromMe(quoter))
		}
	}
	if historyReqs > 0 || unavailableReqs > 0 {
		log.Printf("WA agent %d: timeline repair chat=%s history=%d unavailable=%d", w.agentID, sender, historyReqs, unavailableReqs)
	}
	return historyReqs, unavailableReqs, syncErr
}

// RequestDeepHistorySync menarik riwayat sedalam yang HP mau berbagi untuk satu chat:
// catch-up ujung, lalu halaman-on-demand ke belakang sampai habis, lalu FULL_HISTORY
// jendela panjang. Dipakai tombol sinkron manual — tidak dibatasi "50 pesan" total.
func (w *waInstance) RequestDeepHistorySync(sender string) error {
	w.historyRequestMu.Lock()
	defer w.historyRequestMu.Unlock()
	return w.requestDeepHistorySyncLocked(sender)
}

var ErrHistorySyncBusy = errors.New("sinkronisasi WhatsApp lain masih berjalan")

// ReserveDeepHistorySync mengambil slot job sebelum handler mengembalikan HTTP 202.
// Tanpa reservasi ini, beberapa klik cepat dapat sama-sama melihat status lama lalu
// mengantre deep sync yang berjalan berurutan dan terlihat seperti macet.
func (w *waInstance) ReserveDeepHistorySync(sender string) (HistorySyncStatus, error) {
	sender = NormalizePhone(strings.TrimSpace(sender))
	if sender == "" {
		return w.HistorySyncStatus(), errors.New("nomor percakapan kosong")
	}
	if !w.historyRequestMu.TryLock() {
		return w.HistorySyncStatus(), ErrHistorySyncBusy
	}

	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		w.historyRequestMu.Unlock()
		return w.HistorySyncStatus(), errors.New("WhatsApp belum tersambung")
	}
	now := time.Now()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "deep", Sender: sender, StartedAt: &now,
		Message: "Mengambil riwayat lengkap dari HP…",
	}
	status := w.historyStatus
	w.mu.Unlock()
	return status, nil
}

// RunReservedDeepHistorySync hanya dipanggil setelah ReserveDeepHistorySync sukses.
// sync.Mutex tidak terikat goroutine, sehingga handler dapat menyerahkan slot ini
// kepada worker tanpa celah tempat job lain menyelip.
func (w *waInstance) RunReservedDeepHistorySync(sender string) error {
	defer w.historyRequestMu.Unlock()
	return w.requestDeepHistorySyncLocked(sender)
}

func (w *waInstance) requestDeepHistorySyncLocked(sender string) error {
	sender = NormalizePhone(strings.TrimSpace(sender))
	if sender == "" {
		return errors.New("nomor percakapan kosong")
	}
	nowStart := time.Now()
	w.mu.Lock()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "deep", Sender: sender, StartedAt: &nowStart,
		Message: "Mengambil riwayat lengkap dari HP…",
	}
	w.mu.Unlock()

	var beforeCount int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&beforeCount)

	var syncErr error
	var recoverableCatchUpErr error
	waTip := ChatWATipTime(w.agentID, sender)
	// Pass 1: tutup celah ujung + lubang tengah (count besar per request).
	stopRequests := false
	if err := w.requestChatCatchUpLocked(sender, historyOnDemandPageSize, waTip, false); err != nil {
		// Lanjut deep-page meski catch-up gagal (mis. belum ada acuan) — coba di bawah.
		log.Printf("WA agent %d: deep catch-up %s: %v", w.agentID, sender, err)
		if errors.Is(err, errHistoryAnchorUnavailable) {
			// Chat tanpa row lokal memang tidak punya anchor. FULL_HISTORY di pass 3
			// adalah jalur pemulihan resminya; jangan menandai gagal bila fallback itu
			// berhasil menerima respons dari perangkat utama.
			recoverableCatchUpErr = err
		} else {
			syncErr = errors.Join(syncErr, err)
			// Saat perangkat tidak menjawab, fallback berikutnya hampir pasti hanya
			// menambah 12/25 detik dan pesan error duplikat. Berhenti adaptif.
			stopRequests = true
		}
	}

	// Pass 2: paginate ke belakang dari pesan lokal tertua sampai HP tidak menambah baris.
	// HISTORY_SYNC_ON_DEMAND = pesan SEBELUM anchor; anchor = oldest lokal tiap putaran.
	emptyStreak := 0
	for page := 0; !stopRequests && page < historyDeepMaxPages; page++ {
		var oldest models.ChatHistory
		if err := database.DB.
			Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL", w.agentID, sender).
			Order("created_at ASC, id ASC").
			First(&oldest).Error; err != nil {
			break
		}
		fromMe := strings.TrimSpace(oldest.Message) == "" && strings.TrimSpace(oldest.Reply) != ""
		info := types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat: types.NewJID(sender, types.DefaultUserServer), IsFromMe: fromMe,
			},
			ID:        types.MessageID(oldest.WAMsgID),
			Timestamp: oldest.CreatedAt,
		}
		var midCount int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ?", w.agentID, sender).
			Count(&midCount)

		mode := fmt.Sprintf("deep_older_%d", page+1)
		if err := w.sendHistoryOnDemand(info, historyOnDemandPageSize, sender, mode, false); err != nil {
			log.Printf("WA agent %d: deep page %d %s: %v", w.agentID, page+1, sender, err)
			syncErr = errors.Join(syncErr, err)
			stopRequests = true
			break
		}
		var afterPage int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ?", w.agentID, sender).
			Count(&afterPage)
		gained := afterPage - midCount
		log.Printf("WA agent %d: deep page %d chat=%s +%d (total=%d)", w.agentID, page+1, sender, gained, afterPage)
		if gained <= 0 {
			emptyStreak++
			if emptyStreak >= 2 {
				break
			}
			// Coba anchor pesan ke-2 tertua (kadang oldest tidak diakui HP).
			var second models.ChatHistory
			if err := database.DB.
				Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL AND id <> ?",
					w.agentID, sender, oldest.ID).
				Order("created_at ASC, id ASC").
				First(&second).Error; err != nil {
				break
			}
			secFromMe := strings.TrimSpace(second.Message) == "" && strings.TrimSpace(second.Reply) != ""
			secInfo := types.MessageInfo{
				MessageSource: types.MessageSource{
					Chat: types.NewJID(sender, types.DefaultUserServer), IsFromMe: secFromMe,
				},
				ID:        types.MessageID(second.WAMsgID),
				Timestamp: second.CreatedAt,
			}
			if err := w.sendHistoryOnDemand(secInfo, historyOnDemandPageSize, sender, mode+"_alt", false); err != nil {
				syncErr = errors.Join(syncErr, err)
				stopRequests = true
				break
			}
			continue
		}
		emptyStreak = 0
	}

	// Pass 3: FULL_HISTORY jendela panjang (HP mengirim sesuai data yang ada di jendela).
	from := time.Now().Add(-time.Duration(historyDeepFullDays) * 24 * time.Hour)
	if !stopRequests {
		if err := w.sendFullHistoryOnDemand(sender, from, uint32(historyDeepFullDays)); err != nil {
			log.Printf("WA agent %d: deep full-history %s: %v", w.agentID, sender, err)
			syncErr = errors.Join(syncErr, err)
			stopRequests = true
			if recoverableCatchUpErr != nil {
				syncErr = errors.Join(syncErr, recoverableCatchUpErr)
			}
		}
	}

	// Pass 4: quote / nested context.
	if !stopRequests {
		if c, r := w.EnsureMissingQuotedMessages(sender, 25); c > 0 || r > 0 {
			log.Printf("WA agent %d: deep quote chat=%s stub=%d requested=%d", w.agentID, sender, c, r)
		}
	}
	// Pass 5: isi lubang tengah + minta ulang timestamp asli (stub yang digeser hari).
	if !stopRequests {
		if h, u, err := w.requestTimelineRepairLocked(sender, 20); h > 0 || u > 0 {
			log.Printf("WA agent %d: deep timeline repair chat=%s hist=%d unavail=%d", w.agentID, sender, h, u)
			if err != nil {
				syncErr = errors.Join(syncErr, err)
			}
		} else if err != nil {
			syncErr = errors.Join(syncErr, err)
		}
	}

	var afterCount int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&afterCount)
	gained := int(afterCount - beforeCount)
	if gained < 0 {
		gained = 0
	}
	// Jangan pernah menurunkan tip resmi WhatsApp ke data lokal. Bila HP tidak
	// menjawab atau masih ada gap, status harus tetap terlihat oleh operator.
	stillStale := ChatPreviewStale(w.agentID, sender)

	finished := time.Now()
	w.mu.Lock()
	w.historyStatus.Mode = "deep"
	w.historyStatus.Sender = sender
	w.historyStatus.Imported = gained
	w.historyStatus.FinishedAt = &finished
	w.historyStatus.StillStale = stillStale
	if syncErr != nil {
		w.historyStatus.State = "failed"
		w.historyStatus.Error = historySyncFailureMessage(syncErr)
	} else {
		w.historyStatus.State = "completed"
		w.historyStatus.Error = ""
	}
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
	w.mu.Unlock()
	if syncErr != nil {
		log.Printf("WA agent %d: deep history chat=%s tidak lengkap +%d pesan: %v",
			w.agentID, sender, gained, syncErr)
		return syncErr
	}
	log.Printf("WA agent %d: deep history chat=%s +%d pesan (sebelum=%d sesudah=%d)",
		w.agentID, sender, gained, beforeCount, afterCount)
	return nil
}

func clampHistoryOnDemandCount(count int) int {
	if count <= 0 {
		return historyOnDemandPageSize
	}
	if count > historyOnDemandMaxCount {
		return historyOnDemandMaxCount
	}
	return count
}

func (w *waInstance) addHistoryWaiter(sender string) chan struct{} {
	waiter := make(chan struct{})
	w.historyWaitersMu.Lock()
	if w.historyWaiters == nil {
		w.historyWaiters = make(map[string][]chan struct{})
	}
	w.historyWaiters[sender] = append(w.historyWaiters[sender], waiter)
	w.historyWaitersMu.Unlock()
	return waiter
}

func (w *waInstance) failHistoryRequest(sender string, requestErr error) {
	finished := time.Now()
	stale := ChatPreviewStale(w.agentID, sender)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.historyStatus.State = "failed"
	w.historyStatus.Sender = sender
	if requestErr != nil {
		w.historyStatus.Error = historySyncFailureMessage(requestErr)
	}
	w.historyStatus.FinishedAt = &finished
	w.historyStatus.StillStale = stale
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
}

func (w *waInstance) completeHistoryRequest(sender string) error {
	finished := time.Now()
	stale := ChatPreviewStale(w.agentID, sender)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.historyStatus.State == "failed" {
		if strings.TrimSpace(w.historyStatus.Error) != "" {
			return errors.New(w.historyStatus.Error)
		}
		return errors.New("gagal mengimpor riwayat WhatsApp")
	}
	w.historyStatus.State = "completed"
	w.historyStatus.Sender = sender
	w.historyStatus.FinishedAt = &finished
	w.historyStatus.StillStale = stale
	w.historyStatus.Error = ""
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
	return nil
}

// sendFullHistoryOnDemand meminta FULL_HISTORY_SYNC_ON_DEMAND dari HP
// (jendela waktu), berguna menutup celah ujung yang tidak terjangkau on-demand per-anchor.
func (w *waInstance) sendFullHistoryOnDemand(sender string, from time.Time, days uint32) error {
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		err := errors.New("WhatsApp belum tersambung")
		w.failHistoryRequest(sender, err)
		return err
	}
	now := time.Now()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "full_catch_up", Sender: sender, StartedAt: &now,
		Imported: 0, Skipped: 0,
	}
	w.mu.Unlock()
	if days == 0 {
		days = 14
	}
	if from.IsZero() {
		from = time.Now().Add(-14 * 24 * time.Hour)
	}
	waiter := w.addHistoryWaiter(sender)
	defer w.removeHistoryWaiter(sender, waiter)
	req := &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: waProto.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waProto.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waProto.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
				FullHistorySyncOnDemandRequest: &waProto.PeerDataOperationRequestMessage_FullHistorySyncOnDemandRequest{
					RequestMetadata: &waProto.FullHistorySyncOnDemandRequestMetadata{
						RequestID: proto.String(fmt.Sprintf("full-%s-%d", sender, now.UnixNano())),
					},
					HistorySyncConfig: store.DeviceProps.GetHistorySyncConfig(),
					FullHistorySyncOnDemandConfig: &waProto.FullHistorySyncOnDemandConfig{
						HistoryFromTimestamp: proto.Uint64(uint64(from.Unix())),
						HistoryDurationDays:  proto.Uint32(days),
					},
				},
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), fullHistoryOnDemandTimeout)
	defer cancel()
	if _, err := client.SendPeerMessage(ctx, req); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("%w (permintaan full history, batas %s): %v", errHistoryDeviceNoResponse, fullHistoryOnDemandTimeout, err)
		}
		w.failHistoryRequest(sender, err)
		return err
	}
	select {
	case <-waiter:
		return w.completeHistoryRequest(sender)
	case <-ctx.Done():
		err := fmt.Errorf("%w (full history, batas %s)", errHistoryDeviceNoResponse, fullHistoryOnDemandTimeout)
		w.failHistoryRequest(sender, err)
		return err
	}
}

func (w *waInstance) sendHistoryOnDemand(anchor types.MessageInfo, count int, sender, mode string, catchUp bool) error {
	// Serialisasi job dilakukan oleh historyRequestMu di entrypoint publik.
	// historyStatus adalah data presentasi UI, bukan lock: memakai status sebagai
	// lock membuat deep sync menunggu status "syncing" miliknya sendiri.
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		err := errors.New("WhatsApp belum tersambung")
		w.failHistoryRequest(sender, err)
		return err
	}
	count = clampHistoryOnDemandCount(count)
	now := time.Now()
	w.historySeq++
	// Pertahankan Imported kumulatif antar halaman deep_older_* saja.
	prevImported := 0
	if strings.HasPrefix(mode, "deep_older") {
		prevImported = w.historyStatus.Imported
	}
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: mode, Sender: sender, StartedAt: &now,
		Imported: prevImported, Skipped: 0,
	}
	w.mu.Unlock()

	waiter := w.addHistoryWaiter(sender)
	defer w.removeHistoryWaiter(sender, waiter)
	var req *waProto.Message
	if catchUp {
		req = buildCatchUpHistoryRequest(anchor, count)
	} else {
		req = client.BuildHistorySyncRequest(&anchor, count)
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyOnDemandTimeout)
	defer cancel()
	if _, err := client.SendPeerMessage(ctx, req); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("%w (permintaan, batas %s): %v", errHistoryDeviceNoResponse, historyOnDemandTimeout, err)
		}
		w.failHistoryRequest(sender, err)
		return err
	}
	select {
	case <-waiter:
		return w.completeHistoryRequest(sender)
	case <-ctx.Done():
		err := fmt.Errorf("%w (batas %s)", errHistoryDeviceNoResponse, historyOnDemandTimeout)
		w.failHistoryRequest(sender, err)
		return err
	}
}

// buildCatchUpHistoryRequest mirip BuildHistorySyncRequest, tetapi timestamp
// upper-bound diarahkan ke "ujung" chat, bukan created_at anchor lokal. Jangan
// meminta inline response: whatsmeow menunggu event HistorySync ON_DEMAND.
func buildCatchUpHistoryRequest(lastKnown types.MessageInfo, count int) *waProto.Message {
	ts := lastKnown.Timestamp
	if ts.IsZero() {
		ts = time.Now().Add(2 * time.Minute)
	}
	return &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: waProto.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waProto.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waProto.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND.Enum(),
				HistorySyncOnDemandRequest: &waProto.PeerDataOperationRequestMessage_HistorySyncOnDemandRequest{
					ChatJID:              proto.String(lastKnown.Chat.String()),
					OldestMsgID:          proto.String(string(lastKnown.ID)),
					OldestMsgFromMe:      proto.Bool(lastKnown.IsFromMe),
					OnDemandMsgCount:     proto.Int32(int32(count)),
					OldestMsgTimestampMS: proto.Int64(ts.Unix()),
				},
			},
		},
	}
}

// SyncConversationUnread meminta metadata satu percakapan dari perangkat utama
// dan menunggu sampai History Sync mengembalikan state unread-nya. Jalur ini
// dipakai saat bootstrap setelah scan dan diserialkan agar tidak membanjiri HP.
func (w *waInstance) SyncConversationUnread(anchor types.MessageInfo, sender string, timeout time.Duration) error {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return errors.New("nomor percakapan kosong")
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if !w.historyRequestMu.TryLock() {
		return ErrHistorySyncBusy
	}
	defer w.historyRequestMu.Unlock()
	w.historyProbeMu.Lock()
	defer w.historyProbeMu.Unlock()

	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	w.mu.Unlock()
	if !connected {
		return errors.New("WhatsApp belum tersambung")
	}

	waiter := make(chan struct{})
	w.historyWaitersMu.Lock()
	w.historyWaiters[sender] = append(w.historyWaiters[sender], waiter)
	w.historyWaitersMu.Unlock()
	defer w.removeHistoryWaiter(sender, waiter)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// count=30: sekaligus tarik potongan riwayat di sekitar anchor (bukan hanya 1
	// pesan metadata) agar catch-up preview lebih cepat saat bootstrap unread.
	if _, err := client.SendPeerMessage(ctx, client.BuildHistorySyncRequest(&anchor, 30)); err != nil {
		return err
	}
	select {
	case <-waiter:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("status unread tidak diterima: %w", ctx.Err())
	}
}

func (w *waInstance) notifyHistoryChatState(sender string) {
	w.historyWaitersMu.Lock()
	waiters := w.historyWaiters[sender]
	delete(w.historyWaiters, sender)
	w.historyWaitersMu.Unlock()
	for _, waiter := range waiters {
		close(waiter)
	}
}

func (w *waInstance) notifyAllHistoryWaiters() {
	w.historyWaitersMu.Lock()
	unique := make(map[chan struct{}]struct{})
	for sender, waiters := range w.historyWaiters {
		for _, waiter := range waiters {
			unique[waiter] = struct{}{}
		}
		delete(w.historyWaiters, sender)
	}
	w.historyWaitersMu.Unlock()
	for waiter := range unique {
		close(waiter)
	}
}

func (w *waInstance) removeHistoryWaiter(sender string, target chan struct{}) {
	w.historyWaitersMu.Lock()
	defer w.historyWaitersMu.Unlock()
	waiters := w.historyWaiters[sender]
	for i, waiter := range waiters {
		if waiter == target {
			waiters = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(waiters) == 0 {
		delete(w.historyWaiters, sender)
	} else {
		w.historyWaiters[sender] = waiters
	}
}

// reconnectIfNeeded menyambung ulang bila sesi SEHARUSNYA tersambung (intent status "connected",
// device sudah login) tapi socket-nya terputus. Aman: sesi yang di-suspend/logout punya status
// "disconnected" + client nil, jadi dilewati (watchdog tidak melawan disconnect yang disengaja).
func (w *waInstance) reconnectIfNeeded() {
	w.mu.Lock()
	client, intend := w.client, w.status == "connected"
	w.mu.Unlock()
	if !intend || client == nil || client.Store.ID == nil || client.IsConnected() {
		return
	}
	log.Printf("Watchdog: WA agent %d terputus — mencoba menyambung ulang", w.agentID)
	if err := client.Connect(); err != nil {
		log.Printf("Watchdog: reconnect agent %d gagal: %v", w.agentID, err)
	}
}

// StartReconnectWatchdog memantau semua sesi WA tiap interval & menyambung ulang yang terputus
// tanpa perlu restart server (menutup celah "bot diam-diam offline").
func StartReconnectWatchdog(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		for range t.C {
			globalMu.Lock()
			list := make([]*waInstance, 0, len(instances))
			for _, w := range instances {
				list = append(list, w)
			}
			globalMu.Unlock()
			for _, w := range list {
				w.reconnectIfNeeded()
			}
		}
	}()
}

// GetInfo mengembalikan nomor & nama profil WhatsApp yang sedang terhubung.
func (w *waInstance) GetInfo() (number, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client == nil || w.client.Store.ID == nil {
		return "", ""
	}
	return w.client.Store.ID.User, w.client.Store.PushName
}

// Logout memutus & menghapus sesi WhatsApp (unlink). Setelah ini perlu scan QR lagi untuk relink.
func (w *waInstance) Logout() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client != nil {
		ctx := context.Background()
		if w.client.IsLoggedIn() {
			_ = w.client.Logout(ctx)
		} else {
			w.client.Disconnect()
		}
		w.client = nil
	}
	w.qrCode = ""
	w.status = "disconnected"
	return nil
}

// SendReply mengirim balasan yang mengutip pesan tertentu (reply-to).
func (w *waInstance) SendReply(toNumber, message, replyToID string) error {
	_, err := w.SendReplyAndGetID(toNumber, message, replyToID)
	return err
}

// SendReplyAndGetID mengirim reply native dan mengembalikan ID untuk receipt/revoke.
func (w *waInstance) SendReplyAndGetID(toNumber, message, replyToID string) (string, error) {
	jid, err := recipientJID(toNumber)
	if err != nil {
		return "", err
	}
	return w.sendMessageWithDelayAndGetID(jid, message, humanDelay(message), replyToID)
}

// SendImmediateReplyAndGetID dipakai saat operator sudah selesai mengetik di
// dashboard. Jangan tambahkan simulasi mengetik kedua setelah tombol Kirim
// ditekan; presence selama operator mengetik sudah dikirim oleh endpoint
// /typing dan jeda tambahan membuat composer terlihat macet.
func (w *waInstance) SendImmediateReplyAndGetID(toNumber, message, replyToID string) (string, error) {
	jid, err := recipientJID(toNumber)
	if err != nil {
		return "", err
	}
	return w.sendMessageWithDelayAndGetID(jid, message, 0, replyToID)
}

// SendReplyToJID mengirim balasan ke JID penuh, termasuk grup.
func (w *waInstance) SendReplyToJID(jidStr, message, replyToID string) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return fmt.Errorf("JID tujuan tidak valid (%q): %w", jidStr, err)
	}
	return w.SendMessage(jid, message, replyToID)
}

// Typing mengirim indikator "mengetik" ke kontak.
func (w *waInstance) Typing(toNumber string, composing bool) error {
	return w.TypingContext(context.Background(), toNumber, composing)
}

// TypingContext membuat presence dapat ikut dibatalkan ketika browser menutup
// request atau burst typing baru menggantikan request sebelumnya.
func (w *waInstance) TypingContext(parent context.Context, toNumber string, composing bool) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	jid, err := recipientJID(toNumber)
	if err != nil {
		return err
	}
	state := types.ChatPresenceComposing
	if !composing {
		state = types.ChatPresencePaused
	}
	// Presence bersifat best effort. Socket yang sedang bermasalah tidak boleh
	// menahan endpoint /typing dan menumpuk request browser saat CS mengetik.
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	return client.SendChatPresence(ctx, jid, state, types.ChatPresenceMediaText)
}

// RevokeMessage menghapus (unsend) pesan yang sudah dikirim ke kontak.
func (w *waInstance) RevokeMessage(toNumber string, msgID types.MessageID) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	jid, err := recipientJID(toNumber)
	if err != nil {
		return err
	}
	ownJID := client.Store.ID
	if ownJID == nil {
		return fmt.Errorf("akun WA belum login")
	}
	_, err = client.SendMessage(context.Background(), jid, client.BuildRevoke(jid, *ownJID, msgID))
	return err
}

// SendText mengirim pesan ke nomor bare (mis "628123") tanpa pemanggil perlu menyusun JID.
func (w *waInstance) SendText(toNumber, message string) error {
	return w.SendMessage(types.NewJID(toNumber, types.DefaultUserServer), message)
}

// SendTextToJID mengirim teks ke JID apa pun: grup ("..@g.us") maupun nomor ("..@s.whatsapp.net").
// Dipakai broadcast grup, di mana penerima berupa JID grup, bukan nomor telepon.
func (w *waInstance) SendTextToJID(jidStr, message string) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return fmt.Errorf("JID tujuan tidak valid (%q): %w", jidStr, err)
	}
	return w.SendMessage(jid, message)
}

// WAServerErrorCode mengambil kode penolakan yang dikirim server WhatsApp dari error
// SendMessage. Contoh: "server returned error 463" -> 463.
func WAServerErrorCode(err error) (int, bool) {
	if err == nil || !errors.Is(err, whatsmeow.ErrServerReturnedError) {
		return 0, false
	}
	parts := strings.Fields(err.Error())
	if len(parts) == 0 {
		return 0, false
	}
	code, parseErr := strconv.Atoi(parts[len(parts)-1])
	return code, parseErr == nil
}

// IsGroupJID mengembalikan true bila s berupa JID grup WhatsApp ("...@g.us").
// Dipakai untuk membedakan penerima grup dari nomor pribadi pada alur blast.
func IsGroupJID(s string) bool {
	return strings.HasSuffix(s, "@g.us")
}

// NormalizeInboxSender mempertahankan alamat thread grup, sedangkan thread
// personal tetap dinormalisasi ke nomor internasional. Jangan memakai
// NormalizePhone langsung untuk sender Inbox karena "...@g.us" akan kehilangan
// suffix dan masuk ke thread yang salah.
func NormalizeInboxSender(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "+")
	if IsGroupJID(value) {
		jid, err := types.ParseJID(value)
		if err != nil || jid.Server != types.GroupServer || jid.User == "" {
			return ""
		}
		return jid.String()
	}
	return NormalizePhone(value)
}

// recipientJID menerima nomor biasa maupun JID lengkap. Inbox memakai helper
// ini agar thread personal dan grup melewati jalur kirim yang sama.
func recipientJID(value string) (types.JID, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "+")
	if value == "" {
		return types.EmptyJID, errors.New("tujuan WhatsApp kosong")
	}
	if strings.Contains(value, "@") {
		jid, err := types.ParseJID(value)
		if err != nil {
			return types.EmptyJID, fmt.Errorf("JID tujuan tidak valid (%q): %w", value, err)
		}
		return jid, nil
	}
	return types.NewJID(value, types.DefaultUserServer), nil
}

// NormalizePhone membersihkan nomor jadi format digit internasional (mis. "08.." -> "628..").
func NormalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case d == "":
		return ""
	case strings.HasPrefix(d, "0"):
		return "62" + d[1:]
	case strings.HasPrefix(d, "8"): // nomor lokal tanpa awalan 0/62
		return "62" + d
	default:
		return d
	}
}

// ValidatePhoneForWA menilai apakah nomor (setelah dinormalisasi) laik untuk dikirimi
// pesan WhatsApp. Aturannya:
//   - Tidak boleh kosong.
//   - Digit pertama harus 1–9 (bukan '0' atau '+', karena NormalizePhone sudah membuang non-digit).
//   - Untuk awalan '62' (kode negara Indonesia): panjang 11–13 digit.
//     Tujuannya menolak nomor yang terlalu panjang (mis. 15 digit) yang lolos dari
//     filter WA tapi jelas salah ketik atau karakter tercampur.
//   - Untuk negara lain (awalan 1–9 selain '62'): panjang 10–15 digit (range generik E.164).
//
// Mengembalikan (true, "") jika lolos; sebaliknya (false, alasan) untuk ditampilkan
// ke UI / log sebagai pesan gagal yang manusiawi.
func ValidatePhoneForWA(normalized string) (bool, string) {
	if normalized == "" {
		return false, "nomor kosong"
	}
	first := normalized[0]
	if first < '1' || first > '9' {
		return false, "awalan harus 1–9"
	}
	if strings.HasPrefix(normalized, "62") {
		if len(normalized) < 11 || len(normalized) > 13 {
			return false, "panjang nomor Indonesia harus 11–13 digit"
		}
	} else if len(normalized) < 10 || len(normalized) > 15 {
		return false, "panjang nomor harus 10–15 digit"
	}
	return true, ""
}

// CheckOnWhatsApp memeriksa apakah tiap nomor benar-benar terdaftar di WhatsApp.
// Mengembalikan map nomor-ternormalisasi -> terdaftar. Berguna untuk memvalidasi
// daftar penerima sebelum blast: nomor yang tidak terdaftar bisa dibuang lebih awal
// sehingga mengurangi pengiriman gagal & risiko pembatasan nomor pengirim.
func (w *waInstance) CheckOnWhatsApp(numbers []string) (map[string]bool, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	// Normalisasi + dedup; whatsmeow mengharap format "+E164".
	queries := make([]string, 0, len(numbers))
	seen := map[string]bool{}
	for _, n := range numbers {
		norm := NormalizePhone(n)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		queries = append(queries, "+"+norm)
	}
	out := make(map[string]bool, len(queries))
	if len(queries) == 0 {
		return out, nil
	}
	resp, err := client.IsOnWhatsApp(context.Background(), queries)
	if err != nil {
		return nil, err
	}
	for _, r := range resp {
		out[NormalizePhone(r.Query)] = r.IsIn
	}
	return out, nil
}

// WAGroup = grup yang diikuti akun WhatsApp tertaut.
type WAGroup struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	Participants int    `json:"participants"`
	// BotIsAdmin = apakah nomor yang tertaut (Wai) menjadi admin di grup ini.
	// Penentu apakah fitur moderasi (kick/hapus) bisa dijalankan di grup tersebut.
	BotIsAdmin bool `json:"bot_is_admin"`
}

// legacyGroupSenderAlias mengembalikan alias digit yang dahulu tercipta ketika
// normalisasi personal keliru diterapkan pada JID grup. Daftar grup resmi tetap
// menjadi bukti utama; angka yang masih valid sebagai E.164 tidak disentuh.
func legacyGroupSenderAlias(groupJID string) string {
	jid, err := types.ParseJID(strings.TrimSpace(groupJID))
	if err != nil || jid.Server != types.GroupServer || jid.User == "" {
		return ""
	}
	alias := NormalizePhone(jid.User)
	if alias == "" {
		return ""
	}
	if valid, _ := ValidatePhoneForWA(alias); valid {
		return ""
	}
	return alias
}

func (w *waInstance) mergeLegacyGroupThreads(groups []WAGroup) {
	w.groupAliasMu.Lock()
	defer w.groupAliasMu.Unlock()
	for _, group := range groups {
		alias := legacyGroupSenderAlias(group.JID)
		if alias == "" {
			continue
		}
		merged, err := database.MergeLegacyGroupThread(w.agentID, alias, group.JID)
		if err != nil {
			log.Printf("WA agent %d: gagal menggabungkan alias grup %s: %v", w.agentID, group.JID, err)
			continue
		}
		if merged > 0 {
			log.Printf("WA agent %d: %d baris alias grup digabung ke %s", w.agentID, merged, group.JID)
		}
	}
}

// botIsAdminOf mengecek apakah akun yang tertaut adalah admin/super-admin di grup g.
// Mencocokkan identitas bot lewat nomor telepon (Store.ID) maupun LID (Store.LID),
// karena anggota grup bisa beralamat sebagai nomor atau LID.
func botIsAdminOf(client *whatsmeow.Client, g *types.GroupInfo) bool {
	if client.Store.ID == nil {
		return false
	}
	selfPN := client.Store.ID.User
	selfLID := client.Store.LID.User // kosong kalau belum punya LID
	for _, p := range g.Participants {
		isSelf := p.JID.User == selfPN || p.PhoneNumber.User == selfPN
		if selfLID != "" && p.LID.User == selfLID {
			isSelf = true
		}
		if isSelf {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// GetGroups mengambil daftar grup yang diikuti beserta status admin bot di tiap grup.
func (w *waInstance) GetGroups() ([]WAGroup, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	groups, err := client.GetJoinedGroups(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]WAGroup, 0, len(groups))
	names := make(map[string]string, len(groups))
	for _, g := range groups {
		names[g.JID.String()] = g.Name
		out = append(out, WAGroup{
			JID: g.JID.String(), Name: g.Name, Participants: len(g.Participants),
			BotIsAdmin: botIsAdminOf(client, g),
		})
	}
	w.groupNamesMu.Lock()
	w.groupNames = names
	w.groupNamesMu.Unlock()
	// Selesaikan migrasi sebelum daftar grup dikonsumsi Inbox agar thread bernama
	// dan alias angka tidak sempat tampil sebagai dua kontak berbeda.
	w.mergeLegacyGroupThreads(out)
	return out, nil
}

// GroupName membaca nama grup dari cache koneksi; tidak melakukan request
// jaringan saat endpoint Inbox dipolling.
func (w *waInstance) GroupName(groupJID string) string {
	w.groupNamesMu.RLock()
	defer w.groupNamesMu.RUnlock()
	return w.groupNames[groupJID]
}

// CachedGroups mengembalikan snapshot grup tanpa request jaringan. Dipakai
// polling Inbox agar daftar grup tetap ringan.
func (w *waInstance) CachedGroups() []WAGroup {
	w.groupNamesMu.RLock()
	out := make([]WAGroup, 0, len(w.groupNames))
	for jid, name := range w.groupNames {
		out = append(out, WAGroup{JID: jid, Name: name})
	}
	w.groupNamesMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// FormatGroupInboxText menambahkan identitas anggota pada bubble masuk grup.
// Pesan keluar tidak memakai helper ini karena sudah berada di sisi kanan thread.
func FormatGroupInboxText(text, senderName, senderPhone string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		text = "Pesan grup"
	}
	if senderName = strings.TrimSpace(senderName); senderName != "" {
		return senderName + ": " + text
	}
	if senderPhone = NormalizePhone(senderPhone); senderPhone != "" {
		return "+" + senderPhone + ": " + text
	}
	return text
}

// groupMessageText mengambil teks/caption pesan grup TANPA mengunduh media (hemat & cepat).
func groupMessageText(v *events.Message) string {
	m := v.Message
	if m == nil {
		return ""
	}
	if t := m.GetConversation(); t != "" {
		return t
	}
	if e := m.GetExtendedTextMessage(); e != nil {
		return e.GetText()
	}
	if img := m.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := m.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	return ""
}

// GroupModerationInfo mengembalikan himpunan identitas admin grup + apakah bot sendiri admin.
// Satu panggilan GetGroupInfo dipakai untuk keduanya; pemanggil sebaiknya men-cache hasilnya.
func (w *waInstance) GroupModerationInfo(groupJID string) (admins map[string]bool, botIsAdmin bool, err error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, false, fmt.Errorf("client WA tidak terhubung")
	}
	gjid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, false, err
	}
	info, err := client.GetGroupInfo(context.Background(), gjid)
	if err != nil {
		return nil, false, err
	}
	admins = map[string]bool{}
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			if p.JID.User != "" {
				admins[p.JID.User] = true
			}
			if p.PhoneNumber.User != "" {
				admins[p.PhoneNumber.User] = true
			}
			if p.LID.User != "" {
				admins[p.LID.User] = true
			}
		}
	}
	return admins, botIsAdminOf(client, info), nil
}

// DeleteGroupMessage menghapus (revoke) pesan anggota lain di grup — butuh bot jadi admin.
func (w *waInstance) DeleteGroupMessage(groupJID, senderJID, msgID string) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	gjid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	sjid, err := types.ParseJID(senderJID)
	if err != nil {
		return err
	}
	_, err = client.SendMessage(context.Background(), gjid, client.BuildRevoke(gjid, sjid, types.MessageID(msgID)))
	return err
}

// KickFromGroup mengeluarkan satu anggota dari grup — butuh bot jadi admin.
func (w *waInstance) KickFromGroup(groupJID, userJID string) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	gjid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	ujid, err := types.ParseJID(userJID)
	if err != nil {
		return err
	}
	_, err = client.UpdateGroupParticipants(context.Background(), gjid, []types.JID{ujid}, whatsmeow.ParticipantChangeRemove)
	return err
}

// GetGroupMembers mengambil nomor anggota sebuah grup (untuk dijadikan penerima).
func (w *waInstance) GetGroupMembers(jidStr string) ([]WAContact, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return nil, err
	}
	gi, err := client.GetGroupInfo(context.Background(), jid)
	if err != nil {
		return nil, err
	}
	out := make([]WAContact, 0, len(gi.Participants))
	for _, p := range gi.Participants {
		num := ""
		if p.PhoneNumber.User != "" {
			num = p.PhoneNumber.User
		} else if p.JID.Server == types.DefaultUserServer {
			num = p.JID.User
		}
		if num == "" {
			continue
		}
		out = append(out, WAContact{Number: num, Name: p.DisplayName})
	}
	return out, nil
}

// SyncLabels mengambil snapshot label terbaru dari app-state WhatsApp. Snapshot
// dikembalikan ke handler agar tabel aplikasi bisa diganti secara transaksional;
// FetchAppState di sini sengaja tidak langsung mendispatch event satu per satu.
func (w *waInstance) SyncLabels(ctx context.Context) ([]WALabelSnapshot, []WALabelContactSnapshot, error) {
	w.labelSyncMu.Lock()
	defer w.labelSyncMu.Unlock()

	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return nil, nil, errors.New("WhatsApp belum tersambung")
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	client.EmitAppStateEventsOnFullSync = true
	eventsToApply, err := client.DangerousInternals().FetchAppState(ctx, appstate.WAPatchRegular, true, false)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal sinkronisasi label WhatsApp: %w", err)
	}

	labels := make(map[string]WALabelSnapshot)
	contacts := make(map[string]WALabelContactSnapshot)
	for _, raw := range eventsToApply {
		switch evt := raw.(type) {
		case *events.LabelEdit:
			if evt.Action == nil || evt.LabelID == "" {
				continue
			}
			if evt.Action.GetDeleted() {
				delete(labels, evt.LabelID)
				for key, relation := range contacts {
					if relation.LabelID == evt.LabelID {
						delete(contacts, key)
					}
				}
				continue
			}
			labels[evt.LabelID] = WALabelSnapshot{
				LabelID: evt.LabelID,
				Name:    evt.Action.GetName(),
				Color:   int(evt.Action.GetColor()),
			}
		case *events.LabelAssociationChat:
			if evt.Action == nil || evt.LabelID == "" {
				continue
			}
			number, mapErr := phoneNumberForJID(ctx, client, evt.JID)
			if mapErr != nil {
				log.Printf("WA agent %d: lewati relasi label %s untuk JID %s: %v", w.agentID, evt.LabelID, evt.JID, mapErr)
				continue
			}
			if number == "" {
				continue
			}
			key := evt.LabelID + "\x00" + number
			if evt.Action.GetLabeled() {
				contacts[key] = WALabelContactSnapshot{LabelID: evt.LabelID, Number: number}
			} else {
				delete(contacts, key)
			}
		}
	}

	labelList := make([]WALabelSnapshot, 0, len(labels))
	for _, label := range labels {
		labelList = append(labelList, label)
	}
	sort.Slice(labelList, func(i, j int) bool {
		return strings.ToLower(labelList[i].Name) < strings.ToLower(labelList[j].Name)
	})
	contactList := make([]WALabelContactSnapshot, 0, len(contacts))
	for _, relation := range contacts {
		if _, exists := labels[relation.LabelID]; exists {
			contactList = append(contactList, relation)
		}
	}
	sort.Slice(contactList, func(i, j int) bool {
		if contactList[i].LabelID == contactList[j].LabelID {
			return contactList[i].Number < contactList[j].Number
		}
		return contactList[i].LabelID < contactList[j].LabelID
	})
	return labelList, contactList, nil
}

func phoneNumberForJID(ctx context.Context, client *whatsmeow.Client, jid types.JID) (string, error) {
	jid = jid.ToNonAD()
	switch jid.Server {
	case types.DefaultUserServer, types.LegacyUserServer:
		return jid.User, nil
	case types.HiddenUserServer:
		if client == nil || client.Store == nil || client.Store.LIDs == nil {
			return "", errors.New("penyimpanan mapping LID belum tersedia")
		}
		pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
		if err != nil {
			return "", err
		}
		if pn.IsEmpty() {
			return "", errors.New("mapping nomor untuk LID belum tersedia")
		}
		return pn.User, nil
	default:
		return "", nil
	}
}

// WAContact = satu kontak dari buku alamat akun WhatsApp yang tertaut.
type WAContact struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

// GetContacts mengambil daftar kontak (buku alamat) dari akun WhatsApp yang tertaut.
func (w *waInstance) GetContacts() ([]WAContact, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	all, err := client.Store.Contacts.GetAllContacts(context.Background())
	if err != nil {
		return nil, err
	}
	byNumber := make(map[string]string, len(all))
	for jid, info := range all {
		if jid.User == "" {
			continue
		}
		number := ""
		switch jid.Server {
		case types.DefaultUserServer:
			number = jid.User
		case types.HiddenUserServer:
			if pn, mapErr := client.Store.LIDs.GetPNForLID(context.Background(), jid); mapErr == nil && !pn.IsEmpty() {
				number = pn.User
			}
		default:
			continue
		}
		if number == "" {
			continue
		}
		name := info.FullName
		for _, alt := range []string{info.FirstName, info.PushName, info.BusinessName} {
			if name == "" {
				name = alt
			}
		}
		name = strings.TrimSpace(name)
		if current := byNumber[number]; current == "" || len([]rune(name)) > len([]rune(current)) {
			byNumber[number] = name
		}
	}
	out := make([]WAContact, 0, len(byNumber))
	for number, name := range byNumber {
		out = append(out, WAContact{Number: number, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// NumberCheck = hasil pengecekan satu nomor di WhatsApp.
type NumberCheck struct {
	Input      string `json:"input"`
	Number     string `json:"number"` // nomor ternormalisasi untuk dikirim
	Registered bool   `json:"registered"`
}

// CheckNumbers memeriksa daftar nomor apakah terdaftar di WhatsApp (IsOnWhatsApp).
func (w *waInstance) CheckNumbers(numbers []string) ([]NumberCheck, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	queries := make([]string, 0, len(numbers))
	for _, n := range numbers {
		if d := NormalizePhone(n); d != "" {
			queries = append(queries, "+"+d)
		}
	}
	if len(queries) == 0 {
		return nil, nil
	}
	resp, err := client.IsOnWhatsApp(context.Background(), queries)
	if err != nil {
		return nil, err
	}
	out := make([]NumberCheck, 0, len(resp))
	for _, r := range resp {
		num := strings.TrimPrefix(r.Query, "+")
		if r.IsIn && r.JID.User != "" {
			num = r.JID.User
		}
		out = append(out, NumberCheck{Input: r.Query, Number: num, Registered: r.IsIn})
	}
	return out, nil
}

// LIDForPN mengembalikan LID (angka) untuk satu nomor telepon, bila whatsmeow punya pemetaannya.
// Dipakai mencocokkan riwayat chat lama yang tersimpan sebagai LID, bukan nomor telepon.
func (w *waInstance) LIDForPN(phone string) string {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || client.Store == nil {
		return ""
	}
	lid, err := client.Store.LIDs.GetLIDForPN(context.Background(), types.NewJID(phone, types.DefaultUserServer))
	if err != nil || lid.IsEmpty() {
		return ""
	}
	return lid.User
}

// PNForLID mengembalikan nomor telepon untuk sebuah LID (kebalikan LIDForPN).
// Dipakai merapikan data lama yang menyimpan pengirim sebagai LID.
func (w *waInstance) PNForLID(lid string) string {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || client.Store == nil {
		return ""
	}
	pn, err := client.Store.LIDs.GetPNForLID(context.Background(), types.NewJID(lid, types.HiddenUserServer))
	if err != nil || pn.IsEmpty() {
		return ""
	}
	return pn.User
}

// extractIncoming mengubah pesan WA jadi IncomingMessage (teks atau media yang sudah di-download).
func (w *waInstance) extractIncoming(v *events.Message) (IncomingMessage, bool) {
	if v == nil {
		return IncomingMessage{}, false
	}
	if isProtocolSystemNotification(v.SourceWebMsg) {
		return IncomingMessage{}, false
	}
	m := unwrapHistoryMessage(v.Message)
	if m == nil {
		return IncomingMessage{}, false
	}
	if t := m.GetConversation(); t != "" {
		text := normalizeLocationLinkText(t)
		text = cleanHistoryExportFormat(text)
		return IncomingMessage{Text: text}, true
	}
	if ext := m.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		ci := ext.GetContextInfo()
		return IncomingMessage{
			Text:      normalizeLocationLinkText(ext.GetText()),
			ReplyTo:   contextReplyID(ci),
			ReplyText: contextReplyPreview(ci),
		}, true
	}
	if text, actionID, replyTo, ok := interactiveReplyText(m); ok {
		return IncomingMessage{Text: text, ActionID: actionID, ReplyTo: replyTo}, true
	}
	ctx := context.Background()
	mediaMetadata, metadataErr := proto.Marshal(m)
	if metadataErr != nil {
		log.Printf("WA agent %d: gagal menyimpan metadata media: %v", w.agentID, metadataErr)
		mediaMetadata = nil
	}
	switch {
	case m.GetLocationMessage() != nil:
		loc := m.GetLocationMessage()
		ci := loc.GetContextInfo()
		return IncomingMessage{
			Text:      locationContext(loc.GetName(), loc.GetAddress(), loc.GetComment(), loc.GetURL(), loc.GetDegreesLatitude(), loc.GetDegreesLongitude(), false),
			MediaType: "location",
			ReplyTo:   contextReplyID(ci),
			ReplyText: contextReplyPreview(ci),
		}, true
	case m.GetLiveLocationMessage() != nil:
		loc := m.GetLiveLocationMessage()
		ci := loc.GetContextInfo()
		return IncomingMessage{
			Text:      locationContext("", "", loc.GetCaption(), "", loc.GetDegreesLatitude(), loc.GetDegreesLongitude(), true),
			MediaType: "location",
			ReplyTo:   contextReplyID(ci),
			ReplyText: contextReplyPreview(ci),
		}, true
	case m.GetImageMessage() != nil:
		img := m.GetImageMessage()
		ci := img.GetContextInfo()
		in := IncomingMessage{
			Text: img.GetCaption(), MediaType: "image", Mimetype: img.GetMimetype(),
			MediaMetadata: mediaMetadata,
			ReplyTo:       contextReplyID(ci), ReplyText: contextReplyPreview(ci),
		}
		data, err := w.downloadIncomingMedia(ctx, img)
		if err != nil {
			log.Printf("WA agent %d: download gambar ditunda (pesan tetap disimpan): %v", w.agentID, err)
		} else {
			in.Data = data
		}
		return in, true
	case m.GetDocumentMessage() != nil:
		doc := m.GetDocumentMessage()
		ci := doc.GetContextInfo()
		in := IncomingMessage{
			Text: doc.GetCaption(), MediaType: "document", Mimetype: doc.GetMimetype(), FileName: doc.GetFileName(),
			MediaMetadata: mediaMetadata,
			ReplyTo:       contextReplyID(ci), ReplyText: contextReplyPreview(ci),
		}
		data, err := w.downloadIncomingMedia(ctx, doc)
		if err != nil {
			log.Printf("WA agent %d: download dokumen ditunda (pesan tetap disimpan): %v", w.agentID, err)
		} else {
			in.Data = data
		}
		return in, true
	case m.GetVideoMessage() != nil:
		vid := m.GetVideoMessage()
		ci := vid.GetContextInfo()
		in := IncomingMessage{
			Text: vid.GetCaption(), MediaType: "video", Mimetype: vid.GetMimetype(),
			MediaMetadata: mediaMetadata,
			ReplyTo:       contextReplyID(ci), ReplyText: contextReplyPreview(ci),
		}
		data, err := w.downloadIncomingMedia(ctx, vid)
		if err != nil {
			log.Printf("WA agent %d: download video ditunda (pesan tetap disimpan): %v", w.agentID, err)
		} else {
			in.Data = data
		}
		return in, true
	case m.GetAudioMessage() != nil:
		aud := m.GetAudioMessage()
		ci := aud.GetContextInfo()
		in := IncomingMessage{
			MediaType: "audio", Mimetype: aud.GetMimetype(), MediaMetadata: mediaMetadata,
			ReplyTo: contextReplyID(ci), ReplyText: contextReplyPreview(ci),
		}
		data, err := w.downloadIncomingMedia(ctx, aud)
		if err != nil {
			log.Printf("WA agent %d: download audio ditunda (pesan tetap disimpan): %v", w.agentID, err)
		} else {
			in.Data = data
		}
		return in, true
	case m.GetStickerMessage() != nil:
		st := m.GetStickerMessage()
		ci := st.GetContextInfo()
		in := IncomingMessage{
			MediaType: "sticker", Mimetype: st.GetMimetype(), MediaMetadata: mediaMetadata,
			ReplyTo: contextReplyID(ci), ReplyText: contextReplyPreview(ci),
		}
		data, err := w.downloadIncomingMedia(ctx, st)
		if err != nil {
			log.Printf("WA agent %d: download stiker ditunda (pesan tetap disimpan): %v", w.agentID, err)
		} else {
			in.Data = data
		}
		return in, true
	}
	return IncomingMessage{}, false // tipe pesan lain diabaikan
}

func (w *waInstance) downloadIncomingMedia(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error) {
	if w == nil || w.client == nil {
		return nil, errors.New("client WhatsApp tidak tersedia")
	}
	return w.client.Download(ctx, msg)
}

func normalizeLocationLinkText(text string) string {
	// Label ringan di extract; pengayaan penuh (resolve short link + koordinat)
	// dilakukan di EnrichUserMessageForAI sebelum ChatWithKnowledge.
	lower := strings.ToLower(text)
	locationLink := strings.Contains(lower, "maps.app.goo.gl/") ||
		strings.Contains(lower, "google.com/maps") ||
		strings.Contains(lower, "maps.google.") ||
		strings.Contains(lower, "goo.gl/maps/") ||
		strings.Contains(lower, "google.co.id/maps")
	if !locationLink {
		return text
	}
	return "Pelanggan sudah membagikan link lokasi sebagai titik/alamat tujuan. Gunakan link ini dan jangan meminta pelanggan mengirim atau mengonfirmasi lokasi lagi.\n" + text
}

func contextReplyID(info *waProto.ContextInfo) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.GetStanzaID())
}

// contextReplyPreview merangkum pesan yang di-quote (teks, caption, atau label media)
// supaya Inbox bisa menampilkan quote yang intuitif tanpa bergantung pada lookup ID.
func contextReplyPreview(info *waProto.ContextInfo) string {
	if info == nil {
		return ""
	}
	q := info.GetQuotedMessage()
	if q == nil {
		return ""
	}
	if t := strings.TrimSpace(q.GetConversation()); t != "" {
		return truncateRunes(t, 160)
	}
	if ext := q.GetExtendedTextMessage(); ext != nil {
		if t := strings.TrimSpace(ext.GetText()); t != "" {
			return truncateRunes(t, 160)
		}
	}
	if img := q.GetImageMessage(); img != nil {
		if cap := strings.TrimSpace(img.GetCaption()); cap != "" {
			return "📷 " + truncateRunes(cap, 140)
		}
		return "📷 Foto"
	}
	if vid := q.GetVideoMessage(); vid != nil {
		if cap := strings.TrimSpace(vid.GetCaption()); cap != "" {
			return "🎥 " + truncateRunes(cap, 140)
		}
		return "🎥 Video"
	}
	if aud := q.GetAudioMessage(); aud != nil {
		if aud.GetPTT() {
			return "🎤 Pesan suara"
		}
		return "🎵 Audio"
	}
	if doc := q.GetDocumentMessage(); doc != nil {
		name := strings.TrimSpace(doc.GetFileName())
		if name == "" {
			name = "Dokumen"
		}
		return "📄 " + truncateRunes(name, 120)
	}
	if q.GetStickerMessage() != nil {
		return "🌟 Stiker"
	}
	if loc := q.GetLocationMessage(); loc != nil {
		if name := strings.TrimSpace(loc.GetName()); name != "" {
			return "📍 " + truncateRunes(name, 120)
		}
		return "📍 Lokasi"
	}
	if q.GetLiveLocationMessage() != nil {
		return "📍 Live location"
	}
	if text, _, _, ok := interactiveReplyText(q); ok && strings.TrimSpace(text) != "" {
		return truncateRunes(text, 160)
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

// locationContext membuat lokasi dapat langsung dipakai AI/alur checkout sebagai
// titik tujuan final. Link bawaan WhatsApp dipertahankan; bila kosong dibuatkan
// link Google Maps dari koordinat tanpa memerlukan API geocoding tambahan.
func locationContext(name, address, comment, rawURL string, latitude, longitude float64, live bool) string {
	kind := "Lokasi yang dibagikan pelanggan"
	if live {
		kind = "Live location yang dibagikan pelanggan"
	}
	lines := []string{kind + " (gunakan sebagai titik/alamat tujuan yang sudah diberikan; jangan meminta pelanggan mengirim atau mengonfirmasi lokasi lagi)."}
	if value := strings.TrimSpace(name); value != "" {
		lines = append(lines, "Nama lokasi: "+value)
	}
	if value := strings.TrimSpace(address); value != "" {
		lines = append(lines, "Alamat: "+value)
	}
	if value := strings.TrimSpace(comment); value != "" {
		lines = append(lines, "Catatan: "+value)
	}
	if latitude != 0 || longitude != 0 {
		lines = append(lines, fmt.Sprintf("Koordinat: %.7f, %.7f", latitude, longitude))
		if strings.TrimSpace(rawURL) == "" {
			rawURL = fmt.Sprintf("https://maps.google.com/?q=%.7f,%.7f", latitude, longitude)
		}
	}
	if value := strings.TrimSpace(rawURL); value != "" {
		lines = append(lines, "Google Maps: "+value)
	}
	return strings.Join(lines, "\n")
}

// interactiveReplyText mengubah klik tombol/list WhatsApp menjadi teks biasa agar
// seluruh alur yang sudah ada (AI, Auto-Reply, dan Alur Otomatis) dapat memprosesnya.
// Native Flow adalah format tombol modern, sementara tiga format lain dipertahankan
// untuk kompatibilitas dengan respons dari versi WhatsApp yang berbeda.
func interactiveReplyText(m *waProto.Message) (text, actionID, replyTo string, ok bool) {
	if response := m.GetInteractiveResponseMessage(); response != nil {
		if contextInfo := response.GetContextInfo(); contextInfo != nil {
			replyTo = contextInfo.GetStanzaID()
		}
		if body := strings.TrimSpace(response.GetBody().GetText()); body != "" {
			actionID = nativeFlowActionID(response.GetNativeFlowResponseMessage())
			return body, actionID, replyTo, true
		}
		if native := response.GetNativeFlowResponseMessage(); native != nil {
			var params struct {
				ID          string `json:"id"`
				DisplayText string `json:"display_text"`
				Title       string `json:"title"`
			}
			if err := json.Unmarshal([]byte(native.GetParamsJSON()), &params); err == nil {
				if label := strings.TrimSpace(params.DisplayText); label != "" {
					return label, strings.TrimSpace(params.ID), replyTo, true
				}
				if label := strings.TrimSpace(params.Title); label != "" {
					return label, strings.TrimSpace(params.ID), replyTo, true
				}
				if label := buttonIDText(params.ID); label != "" {
					return label, strings.TrimSpace(params.ID), replyTo, true
				}
			}
		}
	}
	if response := m.GetButtonsResponseMessage(); response != nil {
		if contextInfo := response.GetContextInfo(); contextInfo != nil {
			replyTo = contextInfo.GetStanzaID()
		}
		if label := strings.TrimSpace(response.GetSelectedDisplayText()); label != "" {
			return label, strings.TrimSpace(response.GetSelectedButtonID()), replyTo, true
		}
		if label := buttonIDText(response.GetSelectedButtonID()); label != "" {
			return label, strings.TrimSpace(response.GetSelectedButtonID()), replyTo, true
		}
	}
	if response := m.GetTemplateButtonReplyMessage(); response != nil {
		if contextInfo := response.GetContextInfo(); contextInfo != nil {
			replyTo = contextInfo.GetStanzaID()
		}
		if label := strings.TrimSpace(response.GetSelectedDisplayText()); label != "" {
			return label, strings.TrimSpace(response.GetSelectedID()), replyTo, true
		}
		if label := buttonIDText(response.GetSelectedID()); label != "" {
			return label, strings.TrimSpace(response.GetSelectedID()), replyTo, true
		}
	}
	if response := m.GetListResponseMessage(); response != nil {
		if contextInfo := response.GetContextInfo(); contextInfo != nil {
			replyTo = contextInfo.GetStanzaID()
		}
		if label := strings.TrimSpace(response.GetTitle()); label != "" {
			actionID = ""
			if selected := response.GetSingleSelectReply(); selected != nil {
				actionID = strings.TrimSpace(selected.GetSelectedRowID())
			}
			return label, actionID, replyTo, true
		}
		if selected := response.GetSingleSelectReply(); selected != nil {
			if label := buttonIDText(selected.GetSelectedRowID()); label != "" {
				return label, strings.TrimSpace(selected.GetSelectedRowID()), replyTo, true
			}
		}
	}
	return "", "", "", false
}

func nativeFlowActionID(native *waProto.InteractiveResponseMessage_NativeFlowResponseMessage) string {
	if native == nil {
		return ""
	}
	var params struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(native.GetParamsJSON()), &params) != nil {
		return ""
	}
	return strings.TrimSpace(params.ID)
}

func buttonIDText(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "product_order":
		return "Pesan Sekarang"
	case "product_question":
		return "Tanya Detail"
	default:
		return id
	}
}

// SendImage mengunggah & mengirim gambar ke nomor.
func (w *waInstance) SendImage(toNumber, caption, mimetype string, data []byte) error {
	_, err := w.SendImageAndGetID(toNumber, caption, mimetype, data)
	return err
}

func (w *waInstance) SendImageAndGetID(toNumber, caption, mimetype string, data []byte) (string, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("client WA tidak terhubung")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaWASendTimeout)
	defer cancel()
	up, err := client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("gagal upload gambar: %w", normalizeWASendError(err, mediaWASendTimeout))
	}
	jid, jidErr := recipientJID(toNumber)
	if jidErr != nil {
		return "", jidErr
	}
	resp, err := client.SendMessage(ctx, jid, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		},
	})
	if err != nil {
		return "", normalizeWASendError(err, mediaWASendTimeout)
	}
	return resp.ID, nil
}

// SendDocument mengunggah & mengirim file/dokumen ke nomor (caption opsional).
func (w *waInstance) SendDocument(toNumber, fileName, mimetype, caption string, data []byte) error {
	_, err := w.SendDocumentAndGetID(toNumber, fileName, mimetype, caption, data)
	return err
}

func (w *waInstance) SendDocumentAndGetID(toNumber, fileName, mimetype, caption string, data []byte) (string, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("client WA tidak terhubung")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaWASendTimeout)
	defer cancel()
	up, err := client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return "", fmt.Errorf("gagal upload dokumen: %w", normalizeWASendError(err, mediaWASendTimeout))
	}
	jid, jidErr := recipientJID(toNumber)
	if jidErr != nil {
		return "", jidErr
	}
	resp, err := client.SendMessage(ctx, jid, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			FileName:      proto.String(fileName),
			Title:         proto.String(fileName),
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		},
	})
	if err != nil {
		return "", normalizeWASendError(err, mediaWASendTimeout)
	}
	return resp.ID, nil
}

// SendVideo mengunggah & mengirim video ke nomor (caption opsional).
// Mengikuti pola whatsmeow: Upload(MediaVideo) lalu kirim VideoMessage dengan metadata hasil upload.
func (w *waInstance) SendVideo(toNumber, caption, mimetype string, data []byte) error {
	_, err := w.SendVideoAndGetID(toNumber, caption, mimetype, data)
	return err
}

func (w *waInstance) SendVideoAndGetID(toNumber, caption, mimetype string, data []byte) (string, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("client WA tidak terhubung")
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaWASendTimeout)
	defer cancel()
	up, err := client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return "", fmt.Errorf("gagal upload video: %w", normalizeWASendError(err, mediaWASendTimeout))
	}
	jid, jidErr := recipientJID(toNumber)
	if jidErr != nil {
		return "", jidErr
	}
	resp, err := client.SendMessage(ctx, jid, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		},
	})
	if err != nil {
		return "", normalizeWASendError(err, mediaWASendTimeout)
	}
	return resp.ID, nil
}

// PreparedMedia menyimpan hasil upload media SEKALI agar bisa dikirim ke banyak penerima
// tanpa upload ulang per penerima — penting untuk broadcast video/gambar besar.
type PreparedMedia struct {
	mediaType string // image, video, document
	mimetype  string
	fileName  string
	up        whatsmeow.UploadResponse
}

// PrepareMedia meng-upload media satu kali ke server WhatsApp; hasilnya dipakai SendPreparedMedia.
func (w *waInstance) PrepareMedia(mediaType, mimetype, fileName string, data []byte) (*PreparedMedia, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return nil, fmt.Errorf("client WA tidak terhubung")
	}
	mt := whatsmeow.MediaDocument
	switch mediaType {
	case "image":
		mt = whatsmeow.MediaImage
	case "video":
		mt = whatsmeow.MediaVideo
	}
	up, err := client.Upload(context.Background(), data, mt)
	if err != nil {
		return nil, fmt.Errorf("gagal upload media: %w", err)
	}
	return &PreparedMedia{mediaType: mediaType, mimetype: mimetype, fileName: fileName, up: up}, nil
}

// SendPreparedMedia mengirim media yang sudah di-upload ke satu nomor (TANPA upload ulang).
func (w *waInstance) SendPreparedMedia(toNumber, caption string, pm *PreparedMedia) error {
	return w.sendPreparedMediaTo(types.NewJID(toNumber, types.DefaultUserServer), caption, pm)
}

// SendPreparedMediaToJID mengirim media yang sudah di-upload ke JID apa pun (grup "..@g.us").
func (w *waInstance) SendPreparedMediaToJID(jidStr, caption string, pm *PreparedMedia) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return fmt.Errorf("JID tujuan tidak valid (%q): %w", jidStr, err)
	}
	return w.sendPreparedMediaTo(jid, caption, pm)
}

// sendPreparedMediaTo adalah inti pengiriman media ke satu JID (nomor atau grup).
func (w *waInstance) sendPreparedMediaTo(to types.JID, caption string, pm *PreparedMedia) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	up := pm.up
	var msg *waProto.Message
	switch pm.mediaType {
	case "image":
		msg = &waProto.Message{ImageMessage: &waProto.ImageMessage{
			Caption: proto.String(caption), Mimetype: proto.String(pm.mimetype),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}}
	case "video":
		msg = &waProto.Message{VideoMessage: &waProto.VideoMessage{
			Caption: proto.String(caption), Mimetype: proto.String(pm.mimetype),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}}
	default:
		msg = &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
			FileName: proto.String(pm.fileName), Title: proto.String(pm.fileName),
			Caption: proto.String(caption), Mimetype: proto.String(pm.mimetype),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}}
	}
	_, err := client.SendMessage(context.Background(), to, msg)
	return err
}

// Suspend memutus socket WA tanpa menghapus sesi (device tetap tersimpan di store).
// Dipakai saat langganan tenant tidak aktif; cukup Connect() lagi untuk menyambung tanpa scan QR.
func (w *waInstance) Suspend() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client != nil {
		w.client.Disconnect()
		w.client = nil
	}
	w.qrCode = ""
	w.status = "disconnected"
}

func (w *waInstance) SendMessage(to types.JID, message string, replyToID ...string) error {
	_, err := w.SendMessageAndGetID(to, message, replyToID...)
	return err
}

// SendMessageAndGetID mengirim teks ke JID yang sudah di-resolve dan
// mempertahankan stanza ID WhatsApp untuk identitas kanonik di Inbox.
func (w *waInstance) SendMessageAndGetID(to types.JID, message string, replyToID ...string) (string, error) {
	return w.sendMessageWithDelayAndGetID(to, message, humanDelay(message), replyToID...)
}

// SendSystemMessageAndGetID mengirim notifikasi sistem tanpa simulasi jeda
// mengetik. Operator tetap mendapat ID kanonik segera untuk pencatatan Inbox.
func (w *waInstance) SendSystemMessageAndGetID(to types.JID, message string) (string, error) {
	return w.sendMessageWithDelayAndGetID(to, message, 0)
}

// MarkIncomingRead mengirim read receipt untuk pesan yang benar-benar akan dibalas.
// Ini terpisah dari AutoRead: AutoRead membaca saat pesan tiba, sedangkan fungsi ini
// dipanggil pipeline balasan tepat sebelum indikator mengetik atau balasan dikirim.
func (w *waInstance) MarkIncomingRead(chat, sender types.JID, rawIDs []string) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	if chat.IsEmpty() || sender.IsEmpty() || len(rawIDs) == 0 {
		return nil
	}
	ids := make([]types.MessageID, 0, len(rawIDs))
	seen := map[string]bool{}
	for _, rawID := range rawIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" || seen[rawID] {
			continue
		}
		seen[rawID] = true
		ids = append(ids, types.MessageID(rawID))
	}
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.MarkRead(ctx, ids, time.Now(), chat, sender)
}

// MarkConversationRead menyinkronkan pembacaan dari dashboard ke WhatsApp.
// Untuk akun yang memakai LID, gunakan JID LID bila mapping tersedia agar
// receipt dikirim ke alamat chat yang sama dengan aplikasi WhatsApp.
func (w *waInstance) MarkConversationRead(phone string, rawIDs []string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" || len(rawIDs) == 0 {
		return nil
	}
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WhatsApp tidak terhubung")
	}
	jid, err := recipientJID(phone)
	if err != nil {
		return err
	}
	isGroup := jid.Server == types.GroupServer
	if !isGroup && client.Store != nil && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(context.Background(), jid); err == nil && !lid.IsEmpty() {
			jid = lid
		}
	}
	// Pesan grup berasal dari banyak participant. MarkChatAsRead menyinkronkan
	// posisi thread tanpa mengirim receipt dengan participant yang keliru.
	if !isGroup {
		if err := w.MarkIncomingRead(jid, jid, rawIDs); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.SendAppState(ctx, appstate.BuildMarkChatAsRead(jid, true, time.Now(), nil)); err != nil {
		return fmt.Errorf("gagal menyinkronkan status baca antar perangkat: %w", err)
	}
	return nil
}

// syncReadStates mengambil snapshot regular_low WhatsApp sekali setelah koneksi.
// Snapshot ini memuat markChatAsRead dari perangkat lain, sehingga counter lama
// di dashboard dapat direkonsiliasi walau event terjadi saat backend sedang mati.
func (w *waInstance) syncReadStates() error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return errors.New("WhatsApp belum tersambung")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client.EmitAppStateEventsOnFullSync = true
	if err := client.FetchAppState(ctx, appstate.WAPatchRegularLow, true, false); err != nil {
		return fmt.Errorf("gagal mengambil app-state status baca: %w", err)
	}
	return nil
}

// reconcileReadStates menjadi fallback saat notifikasi app-state dari server
// terlewat. Polling hanya meminta regular_low ketika masih ada badge unread,
// sehingga koneksi idle tidak diberi beban tambahan.
func (w *waInstance) reconcileReadStates(seq uint64) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.mu.Lock()
		client := w.client
		active := w.readPollSeq == seq && client != nil && client.IsConnected() && client.IsLoggedIn()
		w.mu.Unlock()
		if !active {
			return
		}
		var unreadStates int64
		if err := database.DB.Model(&models.InboxReadState{}).
			Where("agent_id = ? AND whats_app_unread_count > 0", w.agentID).
			Count(&unreadStates).Error; err != nil || unreadStates == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := client.FetchAppState(ctx, appstate.WAPatchRegularLow, false, false)
		cancel()
		if err != nil && !errors.Is(err, whatsmeow.ErrNotConnected) {
			log.Printf("WA agent %d: rekonsiliasi status baca tertunda: %v", w.agentID, err)
		}
	}
}

// SendMessageWithDelay mengirim balasan Alur Otomatis dengan rentang jeda yang
// dipilih user. Jeda ini menggantikan humanDelay default, jadi tidak ditumpuk dua kali.
func (w *waInstance) SendMessageWithDelay(to types.JID, message string, minSeconds, maxSeconds int) error {
	return w.SendMessageWithDelayGuarded(to, message, minSeconds, maxSeconds, nil)
}

// SendMessageWithDelayGuarded memeriksa ulang guard setelah indikator mengetik.
// Ini mencegah balasan AI terkirim bila admin mengambil alih saat AI sedang menunggu.
func (w *waInstance) SendMessageWithDelayGuarded(to types.JID, message string, minSeconds, maxSeconds int, guard func() bool) error {
	if minSeconds < 0 {
		minSeconds = 0
	}
	if minSeconds > 30 {
		minSeconds = 30
	}
	if maxSeconds > 30 {
		maxSeconds = 30
	}
	if maxSeconds < minSeconds {
		maxSeconds = minSeconds
	}
	delaySeconds := minSeconds
	if maxSeconds > minSeconds {
		delaySeconds += rand.Intn(maxSeconds - minSeconds + 1)
	}
	delay := time.Duration(delaySeconds) * time.Second
	if guard != nil && !guard() {
		return nil
	}
	if delay > 0 {
		w.mu.Lock()
		client := w.client
		w.mu.Unlock()
		if client == nil || !client.IsConnected() {
			return fmt.Errorf("client WA tidak terhubung")
		}
		ctx := context.Background()
		_ = client.SendPresence(ctx, types.PresenceAvailable)
		_ = client.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		time.Sleep(delay)
		_ = client.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText)
		if guard != nil && !guard() {
			return nil
		}
		return w.sendMessageWithDelay(to, message, 0)
	}
	return w.sendMessageWithDelay(to, message, 0)
}

func (w *waInstance) sendMessageWithDelay(to types.JID, message string, delay time.Duration, replyToID ...string) error {
	_, err := w.sendMessageWithDelayAndGetID(to, message, delay, replyToID...)
	return err
}

func normalizeWASendError(err error, timeout time.Duration) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf(
			"%w setelah %s; periksa chat sebelum mencoba ulang agar pesan tidak terkirim ganda",
			ErrWASendTimeout,
			timeout,
		)
	}
	return err
}

func waitWASendDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return normalizeWASendError(ctx.Err(), interactiveWASendTimeout)
	}
}

func (w *waInstance) sendMessageWithDelayAndGetID(to types.JID, message string, delay time.Duration, replyToID ...string) (string, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("client WA tidak terhubung")
	}

	ctx, cancel := context.WithTimeout(context.Background(), interactiveWASendTimeout)
	defer cancel()
	// Humanisasi: tampilkan "mengetik..." selama jeda, lalu kirim.
	_ = client.SendPresence(ctx, types.PresenceAvailable)
	if delay > 0 {
		_ = client.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		if err := waitWASendDelay(ctx, delay); err != nil {
			return "", err
		}
		_ = client.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText)
	}

	msg := &waProto.Message{
		Conversation: proto.String(message),
	}
	// Reply native: gunakan ExtendedTextMessage dengan ContextInfo.
	if len(replyToID) > 0 && replyToID[0] != "" {
		msg = &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(message),
				ContextInfo: &waProto.ContextInfo{
					StanzaID:    proto.String(replyToID[0]),
					Participant: proto.String(to.ToNonAD().String()),
				},
			},
		}
	}
	resp, err := client.SendMessage(ctx, to, msg)
	if err != nil {
		return "", normalizeWASendError(err, interactiveWASendTimeout)
	}
	return resp.ID, nil
}

// SendTextAndGetID mengirim teks dan mengembalikan ID pesan WhatsApp (untuk revoke).
func (w *waInstance) SendTextAndGetID(toNumber, message string) (string, error) {
	jid, err := recipientJID(toNumber)
	if err != nil {
		return "", err
	}
	return w.sendMessageWithDelayAndGetID(jid, message, humanDelay(message))
}

// SendImmediateTextAndGetID adalah jalur kirim manual operator. Jalur AI dan
// broadcast tetap dapat memakai humanDelay melalui SendTextAndGetID /
// SendMessageAndGetID.
func (w *waInstance) SendImmediateTextAndGetID(toNumber, message string) (string, error) {
	jid, err := recipientJID(toNumber)
	if err != nil {
		return "", err
	}
	return w.sendMessageWithDelayAndGetID(jid, message, 0)
}

// humanDelay meniru kecepatan mengetik manusia: jeda dasar acak + proporsional panjang pesan, dibatasi 6 detik.
func humanDelay(msg string) time.Duration {
	ms := 1500 + rand.Intn(1500) + len([]rune(msg))*25
	if ms > 6000 {
		ms = 6000
	}
	return time.Duration(ms) * time.Millisecond
}

// PostStatus memposting WhatsApp Status (Story) — satu postingan 24 jam yang dilihat kontak.
func (w *waInstance) PostStatus(text, mimetype string, media []byte) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	ctx := context.Background()
	var msg *waProto.Message
	if len(media) > 0 {
		up, err := client.Upload(ctx, media, whatsmeow.MediaImage)
		if err != nil {
			return fmt.Errorf("gagal upload gambar status: %w", err)
		}
		msg = &waProto.Message{ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(text),
			Mimetype:      proto.String(mimetype),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
		}}
	} else {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("status kosong")
		}
		msg = &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: proto.String(text)}}
	}
	_, err := client.SendMessage(ctx, types.StatusBroadcastJID, msg)
	return err
}

// SendContact mengirim kartu kontak (vCard) — dipakai broadcast "simpan kontak kami".
func (w *waInstance) SendContact(toNumber, text, displayName, number string) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	jid := types.NewJID(toNumber, types.DefaultUserServer)
	ctx := context.Background()
	msg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
		},
	}
	if displayName != "" && number != "" {
		msg.ContactMessage = &waProto.ContactMessage{
			DisplayName: proto.String(displayName),
			Vcard:       proto.String(buildVCard(displayName, number)),
		}
	}
	_, err := client.SendMessage(ctx, jid, msg)
	return err
}

// buildVCard menyusun vCard 3.0 minimal yang dikenali WhatsApp.
func buildVCard(name, number string) string {
	digits := digitsOnly(number)
	return "BEGIN:VCARD\n" +
		"VERSION:3.0\n" +
		"N:;" + name + ";;;\n" +
		"FN:" + name + "\n" +
		"TEL;type=CELL;type=VOICE;waid=" + digits + ":+" + digits + "\n" +
		"END:VCARD"
}

// digitsOnly menyaring string menjadi hanya angka.
type ReplyButton struct {
	ID   string
	Text string
}

// SendButtonsWithDelay menjaga pengalaman Alur Otomatis konsisten: read receipt
// dikirim oleh handler, lalu indikator mengetik tampil selama jeda sebelum tombol.
func (w *waInstance) SendButtonsWithDelay(toNumber, bodyText, footerText string, buttons []ReplyButton, minSeconds, maxSeconds int) error {
	if minSeconds < 0 {
		minSeconds = 0
	}
	if minSeconds > 30 {
		minSeconds = 30
	}
	if maxSeconds > 30 {
		maxSeconds = 30
	}
	if maxSeconds < minSeconds {
		maxSeconds = minSeconds
	}
	delaySeconds := minSeconds
	if maxSeconds > minSeconds {
		delaySeconds += rand.Intn(maxSeconds - minSeconds + 1)
	}
	to := types.NewJID(toNumber, types.DefaultUserServer)
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}
	ctx := context.Background()
	_ = client.SendPresence(ctx, types.PresenceAvailable)
	if delaySeconds > 0 {
		_ = client.SendChatPresence(ctx, to, types.ChatPresenceComposing, types.ChatPresenceMediaText)
		time.Sleep(time.Duration(delaySeconds) * time.Second)
		_ = client.SendChatPresence(ctx, to, types.ChatPresencePaused, types.ChatPresenceMediaText)
	}
	return w.SendButtons(toNumber, bodyText, footerText, buttons)
}

// SendButtons mengirim pesan interaktif dengan tombol via NativeFlowMessage (modern API).
func (w *waInstance) SendButtons(toNumber, bodyText, footerText string, buttons []ReplyButton) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("client WA tidak terhubung")
	}

	to := types.NewJID(toNumber, types.DefaultUserServer)

	btnList := make([]*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton, 0, len(buttons))
	for _, b := range buttons {
		params, err := json.Marshal(map[string]string{"display_text": b.Text, "id": b.ID})
		if err != nil {
			return fmt.Errorf("gagal menyusun tombol: %w", err)
		}
		btnList = append(btnList, &waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String("quick_reply"),
			ButtonParamsJSON: proto.String(string(params)),
		})
	}

	msg := &waProto.Message{
		InteractiveMessage: &waProto.InteractiveMessage{
			Header: &waProto.InteractiveMessage_Header{
				HasMediaAttachment: proto.Bool(false),
			},
			Body:   &waProto.InteractiveMessage_Body{Text: proto.String(bodyText)},
			Footer: &waProto.InteractiveMessage_Footer{Text: proto.String(footerText)},
			InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
					MessageVersion: proto.Int32(3),
					Buttons:        btnList,
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	additionalNodes := nativeFlowBizNodes()
	_, err := client.SendMessage(ctx, to, msg, whatsmeow.SendRequestExtra{AdditionalNodes: &additionalNodes})
	return err
}

// nativeFlowBizNodes memberi tahu server WhatsApp bahwa payload ini adalah
// interactive native-flow. whatsmeow menambahkan node bisnis otomatis untuk
// ButtonsMessage lama, tetapi belum mendeteksi InteractiveMessage modern.
func nativeFlowBizNodes() []waBinary.Node {
	return []waBinary.Node{{
		Tag: "biz",
		Content: []waBinary.Node{{
			Tag: "interactive",
			Attrs: waBinary.Attrs{
				"type": "native_flow",
				"v":    "1",
			},
			Content: []waBinary.Node{{
				Tag: "native_flow",
				Attrs: waBinary.Attrs{
					"name": "mixed",
					"v":    "9",
				},
			}},
		}},
	}}
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
