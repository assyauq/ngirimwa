package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"kirimwa/backend/database"
	"kirimwa/backend/models"
	"kirimwa/backend/services"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	unreadBootstrapLimit         = 12
	unreadBootstrapProbeWait     = 6 * time.Second
	unreadBootstrapMaxFailures   = 2
	unreadBootstrapReadyWait     = 30 * time.Second
	automaticCatchUpLimit        = 6
	automaticCatchUpMessageCount = 100
	automaticCatchUpBetweenChats = 750 * time.Millisecond
)

// reconcileUnreadAfterConnect melengkapi status percakapan lama yang tidak
// tercakup oleh History Sync awal. Permintaan dikirim satu per satu dan berhenti
// setelah beberapa kegagalan beruntun agar tidak membebani HP utama.
func reconcileUnreadAfterConnect(agentID uint) {
	connectedAt := time.Now()
	// History Sync awal biasanya datang beberapa detik setelah Connected. Beri
	// kesempatan proses bawaan selesai sebelum mencari percakapan yang tertinggal.
	deadline := time.Now().Add(unreadBootstrapReadyWait)
	time.Sleep(5 * time.Second)
	for time.Now().Before(deadline) {
		if !services.WA(agentID).IsConnected() {
			return
		}
		if services.WA(agentID).HistorySyncStatus().State == "syncing" {
			time.Sleep(2 * time.Second)
			continue
		}
		var historyCount int64
		database.DB.Model(&models.ChatHistory{}).Where("agent_id = ?", agentID).Count(&historyCount)
		if historyCount > 0 {
			break
		}
		time.Sleep(5 * time.Second)
	}
	// Tunggu sebentar untuk chunk terakhir yang sudah diterima tetapi masih
	// menyelesaikan transaksi database.
	time.Sleep(2 * time.Second)

	type senderRow struct {
		Sender string
	}
	var candidates []senderRow
	if err := database.DB.Raw(`
		SELECT ch.sender
		FROM chat_histories ch
		LEFT JOIN inbox_read_states rs
			ON rs.agent_id = ch.agent_id AND rs.sender = ch.sender
		WHERE ch.agent_id = ? AND ch.sender <> '' AND ch.wa_msg_id <> ''
			AND ch.sender NOT LIKE '%@g.us'
			AND (
				rs.id IS NULL
				OR COALESCE(rs.whats_app_synced, 0) = 0
				OR rs.updated_at < ?
			)
		GROUP BY ch.sender
		ORDER BY MAX(ch.created_at) DESC
		LIMIT ?
	`, agentID, connectedAt, unreadBootstrapLimit).Scan(&candidates).Error; err != nil {
		log.Printf("WA agent %d: gagal menyiapkan bootstrap status unread: %v", agentID, err)
		return
	}
	if len(candidates) == 0 {
		log.Printf("WA agent %d: status unread awal sudah lengkap", agentID)
		return
	}

	synced := 0
	consecutiveFailures := 0
	for _, candidate := range candidates {
		if !services.WA(agentID).IsConnected() {
			break
		}
		// State mungkin sudah masuk dari event lain setelah daftar kandidat dibuat.
		var state models.InboxReadState
		if database.DB.Where("agent_id = ? AND sender = ?", agentID, candidate.Sender).First(&state).Error == nil &&
			state.WhatsAppSynced && !state.UpdatedAt.Before(connectedAt) {
			continue
		}
		var anchor models.ChatHistory
		if err := database.DB.Where("agent_id = ? AND sender = ? AND wa_msg_id <> ''", agentID, candidate.Sender).
			Order("created_at DESC, id DESC").First(&anchor).Error; err != nil {
			continue
		}
		info := types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID(candidate.Sender, types.DefaultUserServer),
				IsFromMe: anchor.Message == "" && anchor.Reply != "",
			},
			ID: types.MessageID(anchor.WAMsgID), Timestamp: anchor.CreatedAt,
		}
		if err := services.WA(agentID).SyncConversationUnread(info, candidate.Sender, unreadBootstrapProbeWait); err != nil {
			consecutiveFailures++
			log.Printf("WA agent %d: bootstrap unread %s gagal: %v", agentID, candidate.Sender, err)
			if consecutiveFailures >= unreadBootstrapMaxFailures {
				break
			}
			continue
		}
		synced++
		consecutiveFailures = 0
		time.Sleep(350 * time.Millisecond)
	}
	log.Printf("WA agent %d: bootstrap status unread selesai (%d/%d percakapan)", agentID, synced, len(candidates))
}

func OnWAHistoryChatState(agentID uint, states []services.HistoryChatState) {
	for _, state := range states {
		state.Sender = services.NormalizeInboxSender(state.Sender)
		if state.Sender == "" {
			continue
		}
		if !state.Timestamp.IsZero() && state.Timestamp.Year() < 2020 {
			state.Timestamp = time.Time{}
		}
		var current models.InboxReadState
		if database.DB.Where("agent_id = ? AND sender = ?", agentID, state.Sender).First(&current).Error == nil &&
			current.WhatsAppSynced && current.WhatsAppStateAt != nil && !state.Timestamp.IsZero() &&
			state.Timestamp.Before(*current.WhatsAppStateAt) {
			// Chunk on-demand/late dapat membawa snapshot lebih tua. Mengizinkannya
			// menurunkan tip akan memindahkan chat baru ke bawah daftar.
			continue
		}
		count := state.UnreadCount
		if state.MarkedUnread && count == 0 {
			count = 1
		}
		updates := map[string]interface{}{"whats_app_unread_count": count, "whats_app_synced": true, "updated_at": time.Now()}
		if !state.Timestamp.IsZero() {
			updates["whats_app_state_at"] = state.Timestamp
			updates["last_msg_at"] = gorm.Expr(
				"CASE WHEN last_msg_at IS NULL OR last_msg_at < ? THEN ? ELSE last_msg_at END",
				state.Timestamp, state.Timestamp,
			)
		}
		// WhatsApp mengirim jumlah unread, bukan ID batas baca. Cari pesan tepat
		// sebelum sejumlah pesan unread agar counter lokal menghasilkan nilai sama.
		var boundary models.ChatHistory
		query := database.DB.Where("agent_id = ? AND sender = ? AND TRIM(COALESCE(message, '')) <> ''", agentID, state.Sender).
			Order("created_at DESC, id DESC")
		if count > 0 {
			query = query.Offset(count)
		}
		if query.First(&boundary).Error == nil {
			updates["last_read_at"] = boundary.CreatedAt
		} else if count == 0 && !state.Timestamp.IsZero() {
			updates["last_read_at"] = state.Timestamp
		}
		if err := ensureInboxReadState(agentID, state.Sender); err != nil {
			log.Printf("Gagal memastikan state Inbox (agent %d, %s): %v", agentID, state.Sender, err)
			continue
		}
		updateQuery := database.DB.Model(&models.InboxReadState{}).
			Where("agent_id = ? AND sender = ?", agentID, state.Sender)
		if !state.Timestamp.IsZero() {
			// Guard di SQL menutup race antara dua HistorySync yang selesai paralel.
			updateQuery = updateQuery.Where(
				"whats_app_state_at IS NULL OR whats_app_state_at <= ?",
				state.Timestamp,
			)
		} else {
			// Snapshot tanpa LastMsgTimestamp tidak punya urutan kausal. Boleh
			// menginisialisasi state kosong, tetapi tidak boleh menimpa live/read
			// state yang sudah pernah diterima.
			updateQuery = updateQuery.Where("whats_app_synced = ?", false)
		}
		result := updateQuery.Updates(updates)
		if result.Error == nil && result.RowsAffected > 0 {
			publishInboxEvent(agentID, state.Sender, "state")
		}
	}
}

// reconcileStaleInboxAfterConnect menarik ulang chat yang last_msg_at-nya lebih
// baru dari pesan lokal agar daftar Inbox + preview menyusul WhatsApp Web
// setelah reconnect.
// Dipanggil SETELAH reconcileUnreadAfterConnect selesai (serial) agar tidak
// berebut historyStatus dengan bootstrap unread.
func reconcileStaleInboxAfterConnect(agentID uint) {
	if !services.WA(agentID).IsConnected() {
		return
	}
	// Jeda singkat agar chunk HistorySync RECENT terakhir sempat di-commit.
	time.Sleep(3 * time.Second)
	if !services.WA(agentID).IsConnected() {
		return
	}
	type staleRow struct {
		Sender    string
		LastMsgAt time.Time
		LocalMax  time.Time
	}
	var rows []staleRow
	// Prioritas: gap last_msg_at vs max(created_at), lalu top aktif.
	if err := database.DB.Raw(`
		SELECT rs.sender AS sender,
			rs.last_msg_at AS last_msg_at,
			COALESCE(MAX(ch.created_at), CAST('1970-01-01 00:00:00' AS DATETIME)) AS local_max
		FROM inbox_read_states rs
		LEFT JOIN chat_histories ch
			ON ch.agent_id = rs.agent_id AND ch.sender = rs.sender
		WHERE rs.agent_id = ?
			AND rs.sender <> ''
			AND rs.sender REGEXP '^[0-9]{8,15}$'
			AND rs.last_msg_at IS NOT NULL
			AND rs.last_msg_at > '1971-01-01'
		GROUP BY rs.sender, rs.last_msg_at
		HAVING rs.last_msg_at > DATE_ADD(COALESCE(MAX(ch.created_at), CAST('1970-01-01 00:00:00' AS DATETIME)), INTERVAL 2 MINUTE)
		ORDER BY rs.last_msg_at DESC
		LIMIT ?
	`, agentID, automaticCatchUpLimit).Scan(&rows).Error; err != nil {
		log.Printf("WA agent %d: query catch-up stale gagal: %v", agentID, err)
	}

	log.Printf("WA agent %d: catch-up Inbox untuk %d percakapan", agentID, len(rows))
	for i, row := range rows {
		if !services.WA(agentID).IsConnected() {
			break
		}
		// Rekonsiliasi otomatis hanya mengambil jendela terbaru dan tidak boleh
		// mengantre di belakang sinkronisasi manual/deep milik operator.
		if i > 0 {
			time.Sleep(automaticCatchUpBetweenChats)
		}
		if err := services.WA(agentID).RequestRecentChatCatchUp(
			row.Sender,
			automaticCatchUpMessageCount,
			row.LastMsgAt,
		); err != nil {
			if errors.Is(err, services.ErrHistorySyncBusy) {
				log.Printf("WA agent %d: catch-up otomatis ditunda karena sinkronisasi lain aktif", agentID)
				break
			}
			log.Printf("WA agent %d: catch-up %s: %v", agentID, row.Sender, err)
			continue
		}
		log.Printf("WA agent %d: catch-up #%d %s (wa=%s local=%s)",
			agentID, i+1, row.Sender, row.LastMsgAt.Format(time.RFC3339), row.LocalMax.Format(time.RFC3339))
	}
}

func OnWAWhatsAppReadState(agentID uint, sender string, read bool, timestamp time.Time) {
	sender = services.NormalizeInboxSender(sender)
	if sender == "" {
		return
	}
	timestamp = inboxWAEventTime(timestamp)
	var count any = gorm.Expr("CASE WHEN whats_app_unread_count > 0 THEN whats_app_unread_count ELSE 1 END")
	if read {
		count = 0
	}
	updates := map[string]interface{}{"whats_app_unread_count": count}
	if read {
		var latest models.ChatHistory
		if database.DB.Where(
			"agent_id = ? AND sender = ? AND TRIM(COALESCE(message, '')) <> '' AND created_at <= ?",
			agentID, sender, timestamp,
		).
			Order("created_at DESC, id DESC").First(&latest).Error == nil {
			updates["last_read_at"] = latest.CreatedAt
		} else {
			updates["last_read_at"] = timestamp
		}
	}
	if changed, err := advanceInboxWAState(agentID, sender, timestamp, updates); err == nil && changed {
		publishInboxEvent(agentID, sender, "read")
	}
}

// OnWAHistorySync menyimpan history tanpa melewati pipeline balasan realtime.
// Query deduplikasi dilakukan sekali per batch agar backfill ribuan pesan tetap cepat.
// nonEmptyMessageLines memecah teks multi-baris (hasil debounce) jadi baris non-kosong.
func nonEmptyMessageLines(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// extractMergedLine memisahkan baris target dari teks hasil merge debounce ("a\nb").
// Mengembalikan sisa teks (baris lain digabung ulang) bila target adalah salah satu baris.
func extractMergedLine(merged, target string) (remaining string, ok bool) {
	merged = strings.TrimSpace(merged)
	target = strings.TrimSpace(target)
	if merged == "" || target == "" || !strings.Contains(merged, "\n") {
		return "", false
	}
	if merged == target {
		return "", false
	}
	lines := nonEmptyMessageLines(merged)
	kept := make([]string, 0, len(lines))
	found := false
	for _, line := range lines {
		if !found && line == target {
			found = true
			continue
		}
		kept = append(kept, line)
	}
	if !found || len(kept) == 0 {
		return "", false
	}
	return strings.Join(kept, "\n"), true
}

// splitMergedRemainder menulis sisa baris merge sebagai pesan customer terpisah (tanpa wa_msg_id).
// History sync berikutnya akan mengisi wa_msg_id/timestamp lewat repairMergedCustomerLine.
func splitMergedRemainder(agentID uint, old models.ChatHistory, remaining string) {
	base := old.CreatedAt
	if base.IsZero() {
		base = time.Now()
	}
	i := 0
	for _, line := range strings.Split(remaining, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i++
		row := models.ChatHistory{
			AgentID: agentID, Sender: old.Sender, Message: line, FromHuman: false,
			DeliveryStatus: "sent", CreatedAt: base.Add(time.Duration(i) * time.Millisecond),
		}
		if err := database.DB.Create(&row).Error; err != nil {
			log.Printf("[history-sync] gagal lepas sisa merge agent=%d: %v", agentID, err)
		}
	}
}

// repairMergedCustomerLine melepaskan satu pesan history dari baris Inbox yang digabung
// debounce (hanya punya satu wa_msg_id). Juga menambal baris sisa split yang belum ber-ID.
// Mengembalikan true bila berhasil memperbaiki.
func repairMergedCustomerLine(agentID uint, msg services.HistoricalMessage) bool {
	text := strings.TrimSpace(msg.Text)
	if text == "" || msg.WAMsgID == "" || msg.Sender == "" || msg.Timestamp.IsZero() {
		return false
	}
	// Cari hanya row legacy pada jendela debounce yang sama. Tanpa batas waktu,
	// pesan valid berulang ("Siap", "Terima kasih") dapat dicocokkan ke hari lain.
	windowStart := msg.Timestamp.Add(-10 * time.Second)
	windowEnd := msg.Timestamp.Add(10 * time.Second)
	var candidates []models.ChatHistory
	if err := database.DB.
		Where("agent_id = ? AND sender = ? AND from_human = ?", agentID, msg.Sender, false).
		Where("(reply = '' OR reply IS NULL)").
		Where("message <> ''").
		Where("created_at BETWEEN ? AND ?", windowStart, windowEnd).
		Order("id desc").Limit(40).
		Find(&candidates).Error; err != nil || len(candidates) == 0 {
		return false
	}
	createdAt := msg.Timestamp
	for _, old := range candidates {
		oldMsg := strings.TrimSpace(old.Message)
		// Kasus 1: baris sisa split (teks tunggal sama, belum ada wa_msg_id).
		if oldMsg == text && strings.TrimSpace(old.WAMsgID) == "" {
			if createdAt.IsZero() {
				createdAt = old.CreatedAt
			}
			updates := map[string]interface{}{"wa_msg_id": msg.WAMsgID, "created_at": createdAt}
			if msg.ReplyTo != "" {
				updates["reply_to"] = msg.ReplyTo
			}
			if msg.ReplyText != "" {
				updates["reply_text"] = msg.ReplyText
			}
			if err := database.DB.Model(&models.ChatHistory{}).Where("id = ?", old.ID).Updates(updates).Error; err != nil {
				return false
			}
			log.Printf("[history-sync] isi wa_msg_id sisa merge agent=%d sender=%s wa=%s", agentID, msg.Sender, msg.WAMsgID)
			return true
		}
		// Kasus 2: masih digabung "a\nb" — lepas baris target.
		remaining, ok := extractMergedLine(oldMsg, text)
		if !ok {
			continue
		}
		if createdAt.IsZero() {
			createdAt = old.CreatedAt
		}
		tx := database.DB.Begin()
		if tx.Error != nil {
			return false
		}
		if err := tx.Model(&models.ChatHistory{}).Where("id = ?", old.ID).
			Update("message", remaining).Error; err != nil {
			tx.Rollback()
			return false
		}
		row := models.ChatHistory{
			AgentID: agentID, Sender: msg.Sender, Message: text, FromHuman: false,
			MediaType: msg.MediaType, FileName: msg.FileName, Mimetype: msg.Mimetype,
			WAMsgID: msg.WAMsgID, ReplyTo: msg.ReplyTo, ReplyText: msg.ReplyText,
			DeliveryStatus: "sent", CreatedAt: createdAt,
		}
		if len(msg.MediaMetadata) > 0 {
			row.MediaMetadata = msg.MediaMetadata
			row.MediaFetchStatus = "pending"
		}
		if err := tx.Create(&row).Error; err != nil {
			tx.Rollback()
			return false
		}
		if err := tx.Commit().Error; err != nil {
			return false
		}
		log.Printf("[history-sync] lepas pesan merge debounce agent=%d sender=%s wa=%s", agentID, msg.Sender, msg.WAMsgID)
		return true
	}
	return false
}

// repairLegacyOutgoingMessage mengadopsi row balasan lama yang dibuat sebelum
// sender menyimpan stanza ID. Pencocokan dibatasi sender, teks, arah, dan ±10
// detik agar dua balasan valid dengan isi sama pada waktu berbeda tetap terpisah.
func repairLegacyOutgoingMessage(agentID uint, msg services.HistoricalMessage) bool {
	text := strings.TrimSpace(msg.Text)
	if !msg.FromMe || text == "" || msg.WAMsgID == "" || msg.Sender == "" || msg.Timestamp.IsZero() {
		return false
	}
	var legacy models.ChatHistory
	if err := database.DB.
		Where("agent_id = ? AND sender = ? AND TRIM(COALESCE(reply, '')) = ?", agentID, msg.Sender, text).
		Where("TRIM(COALESCE(message, '')) = ''").
		Where("(wa_msg_id = '' OR wa_msg_id IS NULL)").
		Where("created_at BETWEEN ? AND ?", msg.Timestamp.Add(-10*time.Second), msg.Timestamp.Add(10*time.Second)).
		Order("created_at ASC, id ASC").
		First(&legacy).Error; err != nil {
		return false
	}
	updates := map[string]interface{}{
		"wa_msg_id": msg.WAMsgID, "created_at": msg.Timestamp,
		"delivery_status": "sent",
	}
	if strings.TrimSpace(msg.ReplyTo) != "" {
		updates["reply_to"] = strings.TrimSpace(msg.ReplyTo)
	}
	if strings.TrimSpace(msg.ReplyText) != "" {
		updates["reply_text"] = strings.TrimSpace(msg.ReplyText)
	}
	if strings.TrimSpace(msg.MediaType) != "" {
		updates["media_type"] = strings.TrimSpace(msg.MediaType)
	}
	if strings.TrimSpace(msg.FileName) != "" {
		updates["file_name"] = strings.TrimSpace(msg.FileName)
	}
	if strings.TrimSpace(msg.Mimetype) != "" {
		updates["mimetype"] = strings.TrimSpace(msg.Mimetype)
	}
	if len(msg.MediaMetadata) > 0 {
		updates["media_metadata"] = msg.MediaMetadata
		updates["media_fetch_status"] = gorm.Expr(
			"CASE WHEN TRIM(COALESCE(media_path, '')) = '' THEN 'pending' ELSE media_fetch_status END",
		)
	}
	result := database.DB.Model(&models.ChatHistory{}).
		Where("id = ? AND (wa_msg_id = '' OR wa_msg_id IS NULL)", legacy.ID).
		Updates(updates)
	if result.Error != nil || result.RowsAffected != 1 {
		return false
	}
	return true
}

func OnWAHistorySync(agentID uint, messages []services.HistoricalMessage) (int, int, error) {
	if len(messages) == 0 {
		return 0, 0, nil
	}
	// Satu chunk dapat berisi ratusan update untuk sender yang sama. Kirim satu
	// invalidasi per sender setelah batch selesai agar browser tidak refetch pada
	// setiap pesan history.
	changedSenders := make(map[string]time.Time)
	markChanged := func(sender string, ts time.Time) {
		sender = strings.TrimSpace(sender)
		if sender == "" {
			return
		}
		current, exists := changedSenders[sender]
		if !exists || (!ts.IsZero() && ts.After(current)) {
			changedSenders[sender] = ts
		}
	}
	defer func() {
		for sender, ts := range changedSenders {
			if !ts.IsZero() {
				// touchInboxLastMsg juga menerbitkan tepat satu event realtime.
				touchInboxLastMsg(agentID, sender, ts)
				continue
			}
			publishInboxEvent(agentID, sender, "history")
		}
	}()

	ids := make([]string, 0, len(messages))
	seenIDs := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		if msg.WAMsgID == "" {
			continue
		}
		if _, seen := seenIDs[msg.WAMsgID]; !seen {
			seenIDs[msg.WAMsgID] = struct{}{}
			ids = append(ids, msg.WAMsgID)
		}
	}
	var existingRows []models.ChatHistory
	if len(ids) > 0 {
		if err := database.DB.
			Select(
				"id", "wa_msg_id", "sender", "message", "reply", "created_at",
				"reply_to", "reply_text", "media_type", "file_name", "mimetype",
				"media_metadata",
			).
			Where("agent_id = ? AND wa_msg_id IN ?", agentID, ids).
			Find(&existingRows).Error; err != nil {
			return 0, 0, err
		}
	}
	existing := make(map[string]models.ChatHistory, len(existingRows))
	for _, row := range existingRows {
		existing[row.WAMsgID] = row
	}

	contactsByNumber := make(map[string]models.Contact)
	rows := make([]models.ChatHistory, 0, len(messages))
	skipped := 0
	for _, msg := range messages {
		msg.Sender = services.NormalizeInboxSender(msg.Sender)
		msg.Text = strings.TrimSpace(msg.Text)
		if msg.Sender == "" || msg.WAMsgID == "" || msg.Text == "" {
			skipped++
			continue
		}
		if old, found := existing[msg.WAMsgID]; found {
			// ID yang baru ditemukan dua kali dalam chunk yang sama belum memiliki
			// row database. Baris pertamanya sudah berada di batch insert.
			if old.ID == 0 {
				skipped++
				continue
			}
			updates := map[string]interface{}{}
			// SELALU percayai timestamp WA untuk baris yang sudah ada — koreksi stub
			// yang digeser ke "Hari ini" (CreatedAt = time.Now / CS−1s).
			if !msg.Timestamp.IsZero() && msg.Timestamp.Year() >= 2020 {
				baseSecond := msg.Timestamp.Truncate(time.Second)
				if old.CreatedAt.Before(baseSecond) || !old.CreatedAt.Before(baseSecond.Add(time.Second)) {
					// CASE dievaluasi di UPDATE yang sama: bila live delivery menulis
					// tie-break ms tepat setelah HistorySync mulai, nilainya tidak
					// dapat ditimpa kembali oleh timestamp detik mentah.
					updates["created_at"] = gorm.Expr(
						"CASE WHEN created_at >= ? AND created_at < ? THEN created_at ELSE ? END",
						baseSecond, baseSecond.Add(time.Second), msg.Timestamp,
					)
				}
			}
			// Isi body bila stub masih placeholder.
			if !msg.FromMe && strings.TrimSpace(msg.Text) != "" {
				cur := strings.TrimSpace(old.Message)
				if cur == "" || strings.Contains(cur, "Pesan dikutip") || strings.Contains(cur, "mengambil dari HP") {
					updates["message"] = msg.Text
					updates["from_human"] = false
					updates["reply"] = ""
				}
			} else if msg.FromMe && strings.TrimSpace(msg.Text) != "" {
				cur := strings.TrimSpace(old.Reply)
				if cur == "" || strings.Contains(cur, "Pesan dikutip") || strings.Contains(cur, "mengambil dari HP") {
					updates["reply"] = msg.Text
					updates["from_human"] = true
					updates["message"] = ""
				}
			}
			// Bila baris lama hasil merge debounce ("msg1\nmsg2"), baris ini (pemilik wa_msg_id)
			// di-set ke teks tunggal; sisa baris dilepas sebagai pesan customer terpisah.
			if !msg.FromMe {
				if remaining, ok := extractMergedLine(old.Message, msg.Text); ok {
					updates["message"] = msg.Text
					splitMergedRemainder(agentID, old, remaining)
				}
			}
			// Lengkapi quote reply (stub teks-only / data lama tanpa ContextInfo).
			if replyTo := strings.TrimSpace(msg.ReplyTo); replyTo != "" && old.ReplyTo != replyTo {
				updates["reply_to"] = replyTo
			}
			if replyText := strings.TrimSpace(msg.ReplyText); replyText != "" && old.ReplyText != replyText {
				updates["reply_text"] = replyText
			}
			if mediaType := strings.TrimSpace(msg.MediaType); mediaType != "" && old.MediaType != mediaType {
				updates["media_type"] = mediaType
			}
			if fileName := strings.TrimSpace(msg.FileName); fileName != "" && old.FileName != fileName {
				updates["file_name"] = fileName
			}
			if mimetype := strings.TrimSpace(msg.Mimetype); mimetype != "" && old.Mimetype != mimetype {
				updates["mimetype"] = mimetype
			}
			if len(msg.MediaMetadata) > 0 && len(old.MediaMetadata) == 0 {
				updates["media_metadata"] = msg.MediaMetadata
				updates["media_fetch_status"] = gorm.Expr(
					"CASE WHEN TRIM(COALESCE(media_path, '')) = '' THEN 'pending' ELSE media_fetch_status END",
				)
			}
			if len(updates) > 0 {
				if err := database.DB.Model(&models.ChatHistory{}).
					Where("agent_id = ? AND wa_msg_id = ?", agentID, msg.WAMsgID).
					Updates(updates).Error; err == nil {
					if value, ok := updates["message"].(string); ok {
						old.Message = value
					}
					if value, ok := updates["reply"].(string); ok {
						old.Reply = value
					}
					if value, ok := updates["reply_to"].(string); ok {
						old.ReplyTo = value
					}
					if value, ok := updates["reply_text"].(string); ok {
						old.ReplyText = value
					}
					if value, ok := updates["media_type"].(string); ok {
						old.MediaType = value
					}
					if value, ok := updates["file_name"].(string); ok {
						old.FileName = value
					}
					if value, ok := updates["mimetype"].(string); ok {
						old.Mimetype = value
					}
					if _, ok := updates["media_metadata"]; ok {
						old.MediaMetadata = msg.MediaMetadata
					}
					if _, ok := updates["created_at"]; ok {
						old.CreatedAt = msg.Timestamp
					}
					existing[msg.WAMsgID] = old
					markChanged(msg.Sender, msg.Timestamp)
				}
			}
			skipped++
			continue
		}
		// Adopsi row legacy tanpa ID dalam jendela waktu sempit. Ini mencegah
		// history sync menggandakan balasan bot/CS yang sudah ada secara lokal.
		if msg.FromMe {
			if repaired := repairLegacyOutgoingMessage(agentID, msg); repaired {
				existing[msg.WAMsgID] = models.ChatHistory{WAMsgID: msg.WAMsgID}
				markChanged(msg.Sender, msg.Timestamp)
				skipped++
				continue
			}
		} else {
			// Pesan customer yang kehilangan wa_msg_id karena merge debounce:
			// lepas dari baris gabungan.
			if repaired := repairMergedCustomerLine(agentID, msg); repaired {
				existing[msg.WAMsgID] = models.ChatHistory{WAMsgID: msg.WAMsgID}
				markChanged(msg.Sender, msg.Timestamp)
				skipped++
				continue
			}
		}
		// Lindungi juga dari ID ganda di batch HistorySync yang sama.
		existing[msg.WAMsgID] = models.ChatHistory{WAMsgID: msg.WAMsgID}
		// Tanpa timestamp resmi, menaruh pesan history di "hari ini" akan membuat
		// timeline tampak sinkron padahal tanggalnya rekaan. Tunggu salinan WA yang
		// lengkap daripada menyajikan waktu yang menipu.
		if msg.Timestamp.IsZero() || msg.Timestamp.Year() < 2020 {
			skipped++
			continue
		}
		createdAt := msg.Timestamp
		row := models.ChatHistory{
			AgentID: agentID, Sender: msg.Sender, FromHuman: msg.FromMe,
			MediaType: msg.MediaType, FileName: msg.FileName, Mimetype: msg.Mimetype,
			WAMsgID: msg.WAMsgID, ReplyTo: msg.ReplyTo, ReplyText: msg.ReplyText,
			DeliveryStatus: "sent", CreatedAt: createdAt,
		}
		if len(msg.MediaMetadata) > 0 {
			row.MediaMetadata = msg.MediaMetadata
			row.MediaFetchStatus = "pending"
		}
		if msg.FromMe {
			row.Reply = msg.Text
		} else {
			row.Message = msg.Text
		}
		rows = append(rows, row)
		if !services.IsGroupJID(msg.Sender) {
			contact := models.Contact{
				AgentID: agentID, Number: msg.Sender, Name: strings.TrimSpace(msg.PushName),
				LeadStage: leadStageNew, LeadStageSource: "system", LeadStageReason: "Kontak dari sinkronisasi riwayat WhatsApp",
			}
			if old, ok := contactsByNumber[msg.Sender]; !ok || (old.Name == "" && contact.Name != "") {
				contactsByNumber[msg.Sender] = contact
			}
		}
	}
	if len(rows) == 0 {
		return 0, skipped, nil
	}

	// Menulis sesuai waktu membuat ID lokal tetap lebih masuk akal untuk query lain,
	// walaupun sumber kebenaran urutan Inbox tetap CreatedAt.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.Before(rows[j].CreatedAt) })
	contacts := make([]models.Contact, 0, len(contactsByNumber))
	for _, contact := range contactsByNumber {
		contacts = append(contacts, contact)
	}
	tx := database.DB.Begin()
	if tx.Error != nil {
		return 0, skipped, tx.Error
	}
	if len(contacts) > 0 {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&contacts, 250).Error; err != nil {
			tx.Rollback()
			return 0, skipped, err
		}
	}
	createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&rows, 250)
	if createResult.Error != nil {
		tx.Rollback()
		return 0, skipped, createResult.Error
	}
	if err := tx.Commit().Error; err != nil {
		return 0, skipped, err
	}
	inserted := int(createResult.RowsAffected)
	if inserted < len(rows) {
		skipped += len(rows) - inserted
	}
	// Majukan last_msg_at per sender dari timestamp WA agar daftar chat ikut terurut.
	latestBySender := make(map[string]time.Time, len(contactsByNumber))
	for _, row := range rows {
		if t, ok := latestBySender[row.Sender]; !ok || row.CreatedAt.After(t) {
			latestBySender[row.Sender] = row.CreatedAt
		}
	}
	for sender, ts := range latestBySender {
		markChanged(sender, ts)
	}
	return inserted, skipped, nil
}

func GetHistorySyncStatus(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	c.JSON(200, gin.H{"data": services.WA(id).HistorySyncStatus()})
}

func RequestHistorySync(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Sender string `json:"sender"`
		Count  int    `json:"count"` // 0 = deep (semua yang HP berbagi); >0 = ukuran halaman opsional
		Deep   *bool  `json:"deep"`  // default true untuk tombol sinkron manual
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Sender) == "" {
		c.JSON(400, gin.H{"error": "Kontak yang akan disinkronkan wajib dipilih"})
		return
	}
	req.Sender = strings.TrimPrefix(strings.TrimSpace(req.Sender), "+")
	if services.IsGroupJID(req.Sender) {
		wa := services.WA(id)
		st, err := wa.ReserveRecentHistorySync(req.Sender)
		if err != nil {
			payload := gin.H{"error": err.Error(), "data": st}
			if errors.Is(err, services.ErrHistorySyncBusy) {
				payload["message"] = "Sinkronisasi lain masih berjalan. Tunggu hingga selesai sebelum mencoba lagi."
			}
			c.JSON(http.StatusConflict, payload)
			return
		}
		agentID := id
		sender := req.Sender
		services.Go("group-history-sync", func() {
			if err := wa.RunReservedRecentHistorySync(sender); err != nil {
				log.Printf("WA agent %d: sinkronisasi grup %s: %v", agentID, sender, err)
			}
			publishInboxEvent(agentID, sender, "history_sync")
		})
		c.JSON(http.StatusAccepted, gin.H{
			"message": "Sinkronisasi hingga 100 pesan grup terbaru dimulai di background.",
			"data":    st,
		})
		return
	}
	deep := true
	if req.Deep != nil {
		deep = *req.Deep
	}
	// Tombol sinkron UI: tarik sedalam data nyata di HP (paginate + full history).
	// Bukan hard-limit "50 pesan" — 50 hanya rekomendasi ukuran 1 halaman protocol.
	// Deep dijalankan async agar HTTP tidak timeout saat banyak halaman.
	if deep || req.Count <= 0 {
		sender := req.Sender
		agentID := id
		wa := services.WA(agentID)
		st, err := wa.ReserveDeepHistorySync(sender)
		if err != nil {
			payload := gin.H{"error": err.Error(), "data": st}
			if errors.Is(err, services.ErrHistorySyncBusy) {
				payload["message"] = "Sinkronisasi lain masih berjalan. Tunggu hingga selesai sebelum mencoba lagi."
			}
			c.JSON(409, payload)
			return
		}
		services.Go("deep-history-sync", func() {
			if err := wa.RunReservedDeepHistorySync(sender); err != nil {
				log.Printf("WA agent %d: deep history %s: %v", agentID, sender, err)
			}
			publishInboxEvent(agentID, sender, "history_sync")
		})
		c.JSON(202, gin.H{
			"message": "Sinkronisasi riwayat lengkap dimulai. Sistem akan menarik sebanyak yang tersedia di HP.",
			"data":    st,
		})
		return
	}
	if req.Count > 500 {
		c.JSON(400, gin.H{"error": "Jumlah per halaman sinkronisasi maksimal 500"})
		return
	}
	// Mode ringan (deep=false): satu putaran catch-up dengan ukuran halaman diminta.
	waLast := services.ChatWATipTime(id, req.Sender)
	var localMax time.Time
	_ = database.DB.Raw(
		`SELECT COALESCE(MAX(created_at), CAST('1970-01-01' AS DATETIME)) FROM chat_histories WHERE agent_id = ? AND sender = ?`,
		id, req.Sender,
	).Scan(&localMax)
	if waLast.IsZero() {
		waLast = localMax
	}

	if err := services.WA(id).RequestChatCatchUp(req.Sender, req.Count, waLast); err != nil {
		// Fallback: tarik halaman lebih lama (media hilang / riwayat ke belakang).
		var anchor models.ChatHistory
		var missingMediaRows []models.ChatHistory
		database.DB.Where(
			"agent_id = ? AND sender = ? AND media_type <> '' AND (media_metadata IS NULL OR length(media_metadata) = 0) AND wa_msg_id <> ''",
			id, req.Sender,
		).Order("created_at DESC, id DESC").Limit(100).Find(&missingMediaRows)
		for _, missingMedia := range missingMediaRows {
			if err2 := database.DB.Where(
				"agent_id = ? AND sender = ? AND wa_msg_id <> '' AND (created_at > ? OR (created_at = ? AND id > ?))",
				id, req.Sender, missingMedia.CreatedAt, missingMedia.CreatedAt, missingMedia.ID,
			).Order("created_at ASC, id ASC").First(&anchor).Error; err2 == nil {
				break
			}
		}
		if anchor.ID == 0 {
			if err2 := database.DB.Where("agent_id = ? AND sender = ? AND wa_msg_id <> ''", id, req.Sender).
				Order("created_at ASC, id ASC").First(&anchor).Error; err2 != nil {
				c.JSON(409, gin.H{"error": fmt.Sprintf("Gagal memulai sinkronisasi: %v", err)})
				return
			}
		}
		chat := types.NewJID(req.Sender, types.DefaultUserServer)
		info := types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, IsFromMe: anchor.Message == "" && anchor.Reply != ""},
			ID:            types.MessageID(anchor.WAMsgID), Timestamp: anchor.CreatedAt,
		}
		if err2 := services.WA(id).RequestHistorySync(info, req.Count, req.Sender); err2 != nil {
			c.JSON(409, gin.H{"error": fmt.Sprintf("Gagal memulai sinkronisasi: %v", err2)})
			return
		}
	}
	publishInboxEvent(id, req.Sender, "history_sync")
	st := services.WA(id).HistorySyncStatus()
	c.JSON(202, gin.H{"message": st.Message, "data": st})
}
