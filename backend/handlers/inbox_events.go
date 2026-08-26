package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kirimwa/backend/database"
	"kirimwa/backend/models"

	"github.com/gin-gonic/gin"
)

// InboxEvent adalah sinyal invalidasi ringan. Payload percakapan tetap diambil
// melalui endpoint Inbox biasa, sehingga stream tidak membawa isi chat/sensitif.
type InboxEvent struct {
	AgentID   uint   `json:"agent_id"`
	Sender    string `json:"sender"`
	Kind      string `json:"kind"`
	MessageID string `json:"message_id,omitempty"`
	Active    bool   `json:"active,omitempty"`
	Revision  uint64 `json:"revision"`
}

type inboxEventHub struct {
	mu          sync.RWMutex
	subscribers map[uint]map[chan InboxEvent]struct{}
	history     map[uint][]InboxEvent
	revision    atomic.Uint64
}

const inboxEventHistoryLimit = 256

var inboxEvents = inboxEventHub{
	subscribers: make(map[uint]map[chan InboxEvent]struct{}),
	history:     make(map[uint][]InboxEvent),
}

func (h *inboxEventHub) subscribe(agentID uint) (<-chan InboxEvent, func()) {
	return h.subscribeFrom(agentID, 0, false)
}

// subscribeFrom mendaftarkan stream dan, saat diminta, mengantrekan event yang
// terlewat setelah revision tertentu. Ini membuat reconnect proxy/browser tidak
// menghilangkan bunyi untuk pesan WA berikutnya.
func (h *inboxEventHub) subscribeFrom(agentID uint, afterRevision uint64, replay bool) (<-chan InboxEvent, func()) {
	ch := make(chan InboxEvent, inboxEventHistoryLimit+32)
	h.mu.Lock()
	if h.subscribers == nil {
		h.subscribers = make(map[uint]map[chan InboxEvent]struct{})
	}
	if h.subscribers[agentID] == nil {
		h.subscribers[agentID] = make(map[chan InboxEvent]struct{})
	}
	h.subscribers[agentID][ch] = struct{}{}
	if replay {
		for _, event := range h.history[agentID] {
			if event.Revision > afterRevision {
				ch <- event
			}
		}
	}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers[agentID], ch)
			if len(h.subscribers[agentID]) == 0 {
				delete(h.subscribers, agentID)
			}
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

func (h *inboxEventHub) publish(agentID uint, sender, kind, messageID string) InboxEvent {
	return h.dispatch(InboxEvent{
		AgentID:   agentID,
		Sender:    sender,
		Kind:      kind,
		MessageID: messageID,
	}, true)
}

// publishTransient mengirim state sesaat (mis. composing/paused) tanpa
// memasukkannya ke replay history. Klien yang reconnect tidak boleh menerima
// indikator mengetik yang sudah kedaluwarsa.
func (h *inboxEventHub) publishTransient(event InboxEvent) InboxEvent {
	return h.dispatch(event, false)
}

func (h *inboxEventHub) dispatch(event InboxEvent, replayable bool) InboxEvent {
	event.Sender = strings.TrimSpace(event.Sender)
	event.Kind = strings.TrimSpace(event.Kind)
	event.MessageID = strings.TrimSpace(event.MessageID)
	event.Revision = h.revision.Add(1)
	if event.Kind == "" {
		event.Kind = "conversation"
	}

	h.mu.Lock()
	if replayable {
		if h.history == nil {
			h.history = make(map[uint][]InboxEvent)
		}
		history := append(h.history[event.AgentID], event)
		if len(history) > inboxEventHistoryLimit {
			history = append([]InboxEvent(nil), history[len(history)-inboxEventHistoryLimit:]...)
		}
		h.history[event.AgentID] = history
	}
	for ch := range h.subscribers[event.AgentID] {
		// Stream adalah invalidasi, bukan event log. Bila klien lambat, event
		// berikutnya dengan revision lebih tinggi tetap akan memicu refresh.
		select {
		case ch <- event:
		default:
		}
	}
	h.mu.Unlock()
	return event
}

func publishInboxEvent(agentID uint, sender, kind string) {
	if agentID == 0 {
		return
	}
	inboxEvents.publish(agentID, sender, kind, "")
}

// publishIncomingInboxEvent adalah satu-satunya sinyal yang boleh memicu bunyi
// di browser. Event lain (receipt, sync, pesan keluar) tetap hanya me-refresh UI.
func publishIncomingInboxEvent(agentID uint, sender, messageID string) {
	if agentID == 0 {
		return
	}
	inboxEvents.publish(agentID, sender, "incoming", messageID)
}

func publishInboxTypingEvent(agentID uint, sender string, active bool) {
	if agentID == 0 || strings.TrimSpace(sender) == "" {
		return
	}
	inboxEvents.publishTransient(InboxEvent{
		AgentID: agentID,
		Sender:  sender,
		Kind:    "typing",
		Active:  active,
	})
}

// OnWAChatPresence menerima composing/paused asli dari WhatsApp. Event ini
// sengaja transient: tidak menyentuh DB dan tidak memicu refresh percakapan.
func OnWAChatPresence(agentID uint, sender string, active bool) {
	publishInboxTypingEvent(agentID, sender, active)
}

// OnWAMessageRevoked menyinkronkan "hapus untuk semua orang" dari pelanggan,
// HP utama, maupun WhatsApp Web lain. Retry pendek menutup race ketika revoke
// diterima tepat setelah pesan asli tetapi INSERT pesan masih berjalan.
func OnWAMessageRevoked(agentID uint, messageID string) {
	messageID = strings.TrimSpace(messageID)
	if agentID == 0 || messageID == "" {
		return
	}

	type revokedRow struct {
		Sender string
	}
	var rows []revokedRow
	for attempt := 0; attempt < 4; attempt++ {
		rows = rows[:0]
		result := database.DB.Model(&models.ChatHistory{}).
			Select("sender").
			Where("agent_id = ? AND wa_msg_id = ?", agentID, messageID).
			Find(&rows)
		if result.Error != nil {
			logIfErr(result.Error, "gagal mencari pesan WhatsApp yang dihapus")
			return
		}
		if len(rows) > 0 {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 40 * time.Millisecond)
	}
	if len(rows) == 0 {
		return
	}

	if err := database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND wa_msg_id = ?", agentID, messageID).
		Update("revoked", true).Error; err != nil {
		logIfErr(err, "gagal menandai pesan WhatsApp sebagai dihapus")
		return
	}

	seenSenders := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		sender := strings.TrimSpace(row.Sender)
		if sender == "" {
			continue
		}
		if _, seen := seenSenders[sender]; seen {
			continue
		}
		seenSenders[sender] = struct{}{}
		inboxEvents.publish(agentID, sender, "revoke", messageID)
	}
}

const inboxIncomingCursorBatch = 50

func inboxIncomingAfterID(raw string) (uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cursor notifikasi tidak valid")
	}
	return uint(value), nil
}

// InboxIncomingCursor adalah fallback ringan untuk jaringan/proxy yang tidak
// menjaga koneksi SSE. Cursor berasal dari primary key pesan masuk dan tidak
// bergantung pada unread, sehingga AI/read receipt tidak dapat menelan notifikasi.
func InboxIncomingCursor(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	rawAfterID, hasAfterID := c.GetQuery("after_id")
	afterID, err := inboxIncomingAfterID(rawAfterID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	type incomingRow struct {
		ID        uint      `json:"id"`
		Sender    string    `json:"sender"`
		WAMsgID   string    `json:"message_id,omitempty"`
		CreatedAt time.Time `json:"created_at"`
	}
	base := database.DB.Model(&models.ChatHistory{}).
		Select("id", "sender", "wa_msg_id", "created_at").
		Where("agent_id = ? AND live_incoming = ?", agentID, true)

	if !hasAfterID {
		var latest incomingRow
		result := base.Order("id DESC").Limit(1).Find(&latest)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca cursor notifikasi"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"cursor": latest.ID, "events": []incomingRow{}, "has_more": false,
		}})
		return
	}

	var rows []incomingRow
	result := base.Where("id > ?", afterID).
		Order("id ASC").
		Limit(inboxIncomingCursorBatch + 1).
		Find(&rows)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca notifikasi pesan masuk"})
		return
	}
	hasMore := len(rows) > inboxIncomingCursorBatch
	if hasMore {
		rows = rows[:inboxIncomingCursorBatch]
	}
	cursor := afterID
	if len(rows) > 0 {
		cursor = rows[len(rows)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"cursor": cursor, "events": rows, "has_more": hasMore,
	}})
}

// InboxEvents membuka stream SSE terautentikasi per-agent. Endpoint berada di
// group auth yang sama dengan endpoint Inbox lain, jadi currentAgentID tetap
// memverifikasi tenant dan assignment CS sebelum koneksi ditahan terbuka.
func InboxEvents(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming tidak didukung"})
		return
	}

	// Ambil cursor sebelum subscribe, lalu selalu replay dari cursor tersebut.
	// Dengan begitu event yang terbit tepat saat koneksi pertama dibuka tetap
	// masuk dari history atau channel subscriber, tidak jatuh di celah keduanya.
	afterRevision := inboxEvents.revision.Load()
	if rawSince := strings.TrimSpace(c.Query("since")); rawSince != "" {
		if parsed, err := strconv.ParseUint(rawSince, 10, 64); err == nil {
			afterRevision = parsed
		}
	}
	events, unsubscribe := inboxEvents.subscribeFrom(agentID, afterRevision, true)
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	currentRevision := inboxEvents.revision.Load()
	readyRevision := currentRevision
	if afterRevision <= currentRevision {
		// Jangan melompati event replay: klien akan memajukan cursor satu per satu.
		readyRevision = afterRevision
	}
	ready, _ := json.Marshal(InboxEvent{
		AgentID:  agentID,
		Kind:     "ready",
		Revision: readyRevision,
	})
	_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: inbox\ndata: %s\n\n", readyRevision, ready)
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err = fmt.Fprintf(c.Writer, "id: %d\nevent: inbox\ndata: %s\n\n", event.Revision, data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
