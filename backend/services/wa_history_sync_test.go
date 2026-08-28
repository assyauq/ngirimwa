package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestHistoricalMessageContentText(t *testing.T) {
	text, mediaType, _, _, replyTo, replyText := historicalMessageContent(&waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String("Halo dari history"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String("quoted-id"),
				QuotedMessage: &waProto.Message{Conversation: proto.String("Source code")},
			},
		},
	})
	if text != "Halo dari history" || mediaType != "" || replyTo != "quoted-id" || replyText != "Source code" {
		t.Fatalf("hasil parse tidak sesuai: text=%q media=%q reply=%q preview=%q", text, mediaType, replyTo, replyText)
	}
}

func TestHistoricalMessageContentMediaWithoutDownload(t *testing.T) {
	text, mediaType, fileName, mimetype, _, _ := historicalMessageContent(&waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			Caption:  proto.String("Invoice Juli"),
			FileName: proto.String("invoice.pdf"),
			Mimetype: proto.String("application/pdf"),
		},
	})
	if text != "Invoice Juli" || mediaType != "document" || fileName != "invoice.pdf" || mimetype != "application/pdf" {
		t.Fatalf("metadata media tidak sesuai: %q %q %q %q", text, mediaType, fileName, mimetype)
	}
	if !strings.Contains(historicalMediaLabel(mediaType, fileName), "invoice.pdf") {
		t.Fatal("label dokumen harus menyertakan nama file")
	}
}

func TestProtocolSystemNotificationUsesStubMetadata(t *testing.T) {
	if isProtocolSystemNotification(&waWeb.WebMessageInfo{}) {
		t.Fatal("stub UNKNOWN harus dianggap pesan biasa")
	}
	stub := waWeb.WebMessageInfo_BIZ_NAME_CHANGE
	if !isProtocolSystemNotification(&waWeb.WebMessageInfo{MessageStubType: &stub}) {
		t.Fatal("stub protokol non-UNKNOWN harus dianggap notifikasi sistem")
	}
}

func TestRevokedMessageIDFromProtocolMessage(t *testing.T) {
	revokeType := waProto.ProtocolMessage_REVOKE
	event := &events.Message{Message: &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: &revokeType,
			Key:  &waCommon.MessageKey{ID: proto.String("wamid-deleted")},
		},
	}}
	if got := revokedMessageID(event); got != "wamid-deleted" {
		t.Fatalf("target revoke tidak terbaca, got %q", got)
	}
}

func TestHistoryRevokedMessageIDUsesOriginalStanzaID(t *testing.T) {
	stub := waWeb.WebMessageInfo_REVOKE
	message := &waWeb.WebMessageInfo{
		Key:             &waCommon.MessageKey{ID: proto.String("wamid-history-deleted")},
		MessageStubType: &stub,
	}
	if got := historyRevokedMessageID(message); got != "wamid-history-deleted" {
		t.Fatalf("target revoke history tidak terbaca, got %q", got)
	}
}

func TestExtractIncomingDoesNotFilterShortTextMatchingPushName(t *testing.T) {
	w := &waInstance{}
	in, ok := w.extractIncoming(&events.Message{
		Info:    types.MessageInfo{PushName: "ArifKlik"},
		Message: &waProto.Message{Conversation: proto.String("ArifKlik")},
	})
	if !ok || in.Text != "ArifKlik" {
		t.Fatalf("pesan valid terbuang: ok=%v text=%q", ok, in.Text)
	}
}

func TestExtractIncomingRejectsProtocolStub(t *testing.T) {
	stub := waWeb.WebMessageInfo_BIZ_NAME_CHANGE
	w := &waInstance{}
	_, ok := w.extractIncoming(&events.Message{
		Message:      &waProto.Message{Conversation: proto.String("ArifKlik")},
		SourceWebMsg: &waWeb.WebMessageInfo{MessageStubType: &stub},
	})
	if ok {
		t.Fatal("notifikasi stub protokol tidak boleh menjadi bubble chat")
	}
}

func TestExtractIncomingPreservesStickerWhenDownloadUnavailable(t *testing.T) {
	w := &waInstance{} // client nil mensimulasikan unduhan pertama gagal
	in, ok := w.extractIncoming(&events.Message{
		Message: &waProto.Message{StickerMessage: &waProto.StickerMessage{
			Mimetype: proto.String("image/webp"),
		}},
	})
	if !ok {
		t.Fatal("envelope stiker harus tetap diteruskan saat download gagal")
	}
	if in.MediaType != "sticker" || in.Mimetype != "image/webp" {
		t.Fatalf("metadata stiker tidak sesuai: type=%q mime=%q", in.MediaType, in.Mimetype)
	}
	if len(in.Data) != 0 || len(in.MediaMetadata) == 0 {
		t.Fatalf("data=%d metadata=%d; metadata lazy-download wajib tersedia", len(in.Data), len(in.MediaMetadata))
	}
	var decoded waProto.Message
	if err := proto.Unmarshal(in.MediaMetadata, &decoded); err != nil || decoded.GetStickerMessage() == nil {
		t.Fatalf("metadata stiker tidak dapat dipakai ulang: %v", err)
	}
}

func TestOrderedLiveMessageTimeBreaksSameSecondTies(t *testing.T) {
	w := &waInstance{}
	base := time.Date(2026, time.July, 28, 14, 16, 0, 0, time.UTC)

	first := w.orderedLiveMessageTime("6282261008855", base)
	second := w.orderedLiveMessageTime("6282261008855", base)
	otherChat := w.orderedLiveMessageTime("6285608483004", base)
	nextSecond := w.orderedLiveMessageTime("6282261008855", base.Add(time.Second))

	if !first.Equal(base) {
		t.Fatalf("first timestamp = %s, want %s", first, base)
	}
	if !second.Equal(base.Add(time.Millisecond)) {
		t.Fatalf("second timestamp = %s, want one millisecond after first", second)
	}
	if !otherChat.Equal(base.Add(2 * time.Millisecond)) {
		t.Fatalf("other chat must preserve global chat-list order, got %s", otherChat)
	}
	if !nextSecond.Equal(base.Add(time.Second)) {
		t.Fatalf("new WA second must reset tie-break, got %s", nextSecond)
	}
}

func TestReserveDeepHistorySyncRejectsBusyWithoutQueueing(t *testing.T) {
	w := &waInstance{}
	w.historyRequestMu.Lock()
	_, err := w.ReserveDeepHistorySync("6282261008855")
	w.historyRequestMu.Unlock()
	if !errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("busy reservation error = %v, want %v", err, ErrHistorySyncBusy)
	}
}

func TestReserveDeepHistorySyncReleasesSlotWhenDisconnected(t *testing.T) {
	w := &waInstance{}
	if _, err := w.ReserveDeepHistorySync("6282261008855"); err == nil || errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("first disconnected reservation error = %v", err)
	}
	if _, err := w.ReserveDeepHistorySync("6282261008855"); err == nil || errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("slot leaked after disconnected reservation, error = %v", err)
	}
}

func TestRequestRecentChatCatchUpRejectsBusyWithoutQueueing(t *testing.T) {
	w := &waInstance{}
	w.historyRequestMu.Lock()
	err := w.RequestRecentChatCatchUp("6282261008855", 100, time.Now())
	w.historyRequestMu.Unlock()
	if !errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("auto catch-up error = %v, want %v", err, ErrHistorySyncBusy)
	}
}

func TestHistorySyncFailureMessageCollapsesDeviceTimeouts(t *testing.T) {
	err := errors.Join(
		fmt.Errorf("%w (batas 12s)", errHistoryDeviceNoResponse),
		fmt.Errorf("%w (full history, batas 25s)", errHistoryDeviceNoResponse),
		fmt.Errorf("%w (full history, batas 25s)", errHistoryDeviceNoResponse),
	)
	message := historySyncFailureMessage(err)
	if strings.Count(message, "25s") != 0 || strings.Count(message, "12s") != 0 {
		t.Fatalf("pesan operator tidak boleh memuat timeout fallback berulang: %q", message)
	}
	if !strings.Contains(message, "HP utama belum mengirim paket riwayat") {
		t.Fatalf("pesan harus memberi tindakan yang jelas: %q", message)
	}
}

func TestOnDemandHistorySyncWithoutConversationsAcknowledgesRequest(t *testing.T) {
	previousHistoryHandler := onHistorySync
	defer func() { onHistorySync = previousHistoryHandler }()
	onHistorySync = func(_ uint, _ []HistoricalMessage) (int, int, error) { return 0, 0, nil }

	w := &waInstance{historyWaiters: make(map[string][]chan struct{})}
	waiter := w.addHistoryWaiter("120363425256238999@g.us")
	syncType := waHistorySync.HistorySync_ON_DEMAND
	w.processHistorySync(&events.HistorySync{Data: &waHistorySync.HistorySync{
		SyncType: &syncType,
		Progress: proto.Uint32(100),
	}})

	select {
	case <-waiter:
	default:
		t.Fatal("respons ON_DEMAND kosong harus menyelesaikan waiter manual")
	}
}

func TestCatchUpHistoryRequestUsesEventResponseMode(t *testing.T) {
	request := buildCatchUpHistoryRequest(types.MessageInfo{
		MessageSource: types.MessageSource{Chat: types.NewJID("120363425256238999", types.GroupServer)},
		ID:            "anchor",
		Timestamp:     time.Unix(1785568200, 0),
	}, 100)
	onDemand := request.GetProtocolMessage().GetPeerDataOperationRequestMessage().GetHistorySyncOnDemandRequest()
	if onDemand == nil {
		t.Fatal("request ON_DEMAND tidak terbentuk")
	}
	if onDemand.SupportInlineResponse != nil {
		t.Fatal("inline response tidak boleh diaktifkan karena waiter memakai event HistorySync")
	}
	if onDemand.GetChatJID() != "120363425256238999@g.us" {
		t.Fatalf("JID grup berubah: %q", onDemand.GetChatJID())
	}
}

func TestHistorySyncFailureMessageDeduplicatesOtherErrors(t *testing.T) {
	err := errors.Join(errors.New("gagal A"), errors.New("gagal A"), errors.New("gagal B"))
	message := historySyncFailureMessage(err)
	if strings.Count(message, "gagal A") != 1 || strings.Count(message, "gagal B") != 1 {
		t.Fatalf("error unik harus tampil sekali: %q", message)
	}
	alreadyFriendly := "Sinkronisasi belum lengkap karena WhatsApp di HP tidak merespons."
	if got := historySyncFailureMessage(errors.New(alreadyFriendly)); got != alreadyFriendly {
		t.Fatalf("pesan ramah tidak boleh diberi prefix kedua: %q", got)
	}
}

func TestInteractiveSendTimeoutIsBoundedAndRecognizable(t *testing.T) {
	if interactiveWASendTimeout > 20*time.Second {
		t.Fatalf("interactive send timeout terlalu panjang: %s", interactiveWASendTimeout)
	}
	err := normalizeWASendError(context.DeadlineExceeded, interactiveWASendTimeout)
	if !errors.Is(err, ErrWASendTimeout) {
		t.Fatalf("timeout error = %v, want wrapped %v", err, ErrWASendTimeout)
	}
	if !strings.Contains(err.Error(), "pesan tidak terkirim ganda") {
		t.Fatalf("timeout harus mengingatkan pemeriksaan duplikasi: %v", err)
	}
}

func TestNewestHistoryMessagesKeepsNewestWithinBound(t *testing.T) {
	message := func(timestamp, order uint64) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{
			Message:    &waWeb.WebMessageInfo{MessageTimestamp: proto.Uint64(timestamp)},
			MsgOrderID: proto.Uint64(order),
		}
	}
	messages := []*waHistorySync.HistorySyncMsg{
		message(10, 1), message(30, 1), message(20, 2), message(20, 1),
	}
	got := newestHistoryMessages(messages, 3)
	if len(got) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3", len(got))
	}
	if got[0].GetMessage().GetMessageTimestamp() != 30 ||
		got[1].GetMsgOrderID() != 2 || got[2].GetMsgOrderID() != 1 {
		t.Fatalf("urutan pesan terbaru tidak sesuai: %+v", got)
	}
	// Helper tidak boleh mengubah urutan slice protobuf milik event asli.
	if messages[0].GetMessage().GetMessageTimestamp() != 10 {
		t.Fatal("slice HistorySync asli berubah")
	}
}

func TestFormatGroupInboxTextIncludesParticipant(t *testing.T) {
	if got := FormatGroupInboxText("Update selesai", "Budi", "628123"); got != "Budi: Update selesai" {
		t.Fatalf("label nama grup = %q", got)
	}
	if got := FormatGroupInboxText("", "", "08123"); got != "+628123: Pesan grup" {
		t.Fatalf("fallback nomor grup = %q", got)
	}
}

func TestProcessHistorySyncImportsGroupIntoCanonicalThread(t *testing.T) {
	previousHistoryHandler := onHistorySync
	previousStateHandler := onHistoryChatState
	previousRevokeHandler := onMessageRevoke
	defer func() {
		onHistorySync = previousHistoryHandler
		onHistoryChatState = previousStateHandler
		onMessageRevoke = previousRevokeHandler
	}()

	var captured []HistoricalMessage
	var states []HistoryChatState
	onHistorySync = func(_ uint, messages []HistoricalMessage) (int, int, error) {
		captured = append(captured, append([]HistoricalMessage(nil), messages...)...)
		return len(messages), 0, nil
	}
	onHistoryChatState = func(_ uint, incoming []HistoryChatState) {
		states = append(states, incoming...)
	}
	onMessageRevoke = nil

	groupJID := "120363123456789012@g.us"
	webMessage := &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			RemoteJID: proto.String(groupJID),
			FromMe:    proto.Bool(false),
			ID:        proto.String("group-history-1"),
		},
		Message:          &waProto.Message{Conversation: proto.String("Internet sudah normal")},
		MessageTimestamp: proto.Uint64(1785568200),
		Participant:      proto.String("6281220990678@s.whatsapp.net"),
		PushName:         proto.String("Budi"),
	}
	syncType := waHistorySync.HistorySync_RECENT
	w := &waInstance{
		agentID:        7,
		client:         &whatsmeow.Client{},
		historyWaiters: make(map[string][]chan struct{}),
	}
	w.processHistorySync(&events.HistorySync{Data: &waHistorySync.HistorySync{
		SyncType: &syncType,
		Progress: proto.Uint32(100),
		Conversations: []*waHistorySync.Conversation{{
			ID:               proto.String(groupJID),
			Messages:         []*waHistorySync.HistorySyncMsg{{Message: webMessage}},
			UnreadCount:      proto.Uint32(1),
			LastMsgTimestamp: proto.Uint64(1785568200),
		}},
	}})

	if len(captured) != 1 {
		t.Fatalf("pesan grup terimpor = %d, mau 1", len(captured))
	}
	if captured[0].Sender != groupJID {
		t.Fatalf("sender grup berubah menjadi %q", captured[0].Sender)
	}
	if captured[0].Text != "Budi: Internet sudah normal" {
		t.Fatalf("teks grup = %q", captured[0].Text)
	}
	if len(states) != 1 || states[0].Sender != groupJID || states[0].UnreadCount != 1 {
		t.Fatalf("state grup tidak sesuai: %+v", states)
	}
}

func TestOwnGroupMessageUsesGroupHandler(t *testing.T) {
	previousGroupHandler := onGroupMessage
	previousOwnHandler := onOwnMessage
	defer func() {
		onGroupMessage = previousGroupHandler
		onOwnMessage = previousOwnHandler
	}()

	groupEvents := make(chan GroupMessageMeta, 1)
	personalEvents := make(chan struct{}, 1)
	onGroupMessage = func(_ uint, message GroupMessageMeta) { groupEvents <- message }
	onOwnMessage = func(_ uint, _ types.JID, _ IncomingMessage) { personalEvents <- struct{}{} }

	groupJID := types.NewJID("120363123456789012", types.GroupServer)
	w := &waInstance{agentID: 7}
	w.handleEvent(&events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat: groupJID, Sender: types.NewJID("6281220990678", types.DefaultUserServer),
				IsGroup: true, IsFromMe: true,
			},
			ID: "group-own-1", Timestamp: time.Now(),
		},
		Message: &waProto.Message{Conversation: proto.String("Balasan dari HP")},
	})

	select {
	case message := <-groupEvents:
		if !message.FromMe || message.GroupJID != groupJID.String() || message.Text != "Balasan dari HP" {
			t.Fatalf("event grup sendiri tidak sesuai: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("pesan grup sendiri tidak diteruskan")
	}
	select {
	case <-personalEvents:
		t.Fatal("pesan grup sendiri salah masuk handler personal")
	default:
	}
}
