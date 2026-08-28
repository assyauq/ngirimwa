package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kirimwa/backend/database"
	"kirimwa/backend/models"
	"kirimwa/backend/services"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm/clause"
)

var historicalMediaLocks sync.Map
var profilePictureCache sync.Map

type profilePictureCacheEntry struct {
	URL       string
	ExpiresAt time.Time
}

// TestChat menjalankan AI agent tanpa WhatsApp (simulator "coba chat" di dashboard).
// Pipeline diselaraskan dengan production: routing produk/form, shipping, dan kebijakan eskalasi.
func TestChat(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Message string `json:"message"`
		History []struct {
			Role string `json:"role"` // "user" | "bot"
			Text string `json:"text"`
		} `json:"history"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "Pesan kosong"})
		return
	}
	// Simulator multi-turn: pakai riwayat dari frontend (tanpa menyentuh ChatHistory asli/analytics).
	var history []models.ChatHistory
	hist := req.History
	if len(hist) > 40 {
		hist = hist[len(hist)-40:]
	}
	for _, h := range hist {
		switch h.Role {
		case "user":
			history = append(history, models.ChatHistory{Message: h.Text})
		case "bot":
			history = append(history, models.ChatHistory{Reply: h.Text})
		}
	}
	// Samakan pemotongan konteks dengan production (berdasarkan budget rune, bukan angka pesan tetap).
	newestFirst := make([]models.ChatHistory, len(history))
	for i := range history {
		newestFirst[len(history)-1-i] = history[i]
	}
	history = historyWithinContextBudget(newestFirst, recentContextRuneBudget)

	var agent models.Agent
	database.DB.First(&agent, id)
	prompt := agent.SystemPrompt
	if prompt == "" {
		prompt = "Kamu adalah asisten AI yang ramah. Jawab dalam bahasa Indonesia."
	}
	tone := agent.Tone
	if tone == "" {
		tone = "ramah"
	}
	// Memory per-kontak tidak dipakai di simulator (tidak ada nomor pengirim nyata).
	start := time.Now()
	enhancedPrompt := prompt
	routingText := req.Message
	for i := len(history) - 1; i >= 0 && i >= len(history)-4; i-- {
		routingText += "\n" + history[i].Message + "\n" + history[i].Reply
	}
	if !isGenericGreetingMessage(req.Message) {
		if productRouting := productCheckoutRoutingPrompt(id, testAITurnSender, routingText); productRouting != "" {
			enhancedPrompt += "\n\n" + productRouting
		}
		if formRouting := aiFormRoutingPrompt(id, testAITurnSender); formRouting != "" {
			enhancedPrompt += "\n\n" + formRouting
		}
	}
	shippingCtx := maybeBuildShippingContext(agent, req.Message, history)
	usedShippingTool := strings.Contains(shippingCtx, "ONGKIR_")
	turnError := shippingTurnError(shippingCtx)
	if shippingCtx != "" {
		enhancedPrompt += "\n\n" + shippingCtx
	}
	chatResult, err := services.ChatWithKnowledge(id, enhancedPrompt, tone, req.Message, history)
	reply := chatResult.Reply
	escalate := chatResult.Escalate
	model := chatResult.Model
	knowledgeCount := chatResult.Trace.KnowledgeUsedCount
	trace := chatResult.Trace
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()
		if turnError != "" {
			turnError += "; "
		}
		turnError += "ai: " + err.Error()
		logAITurn(id, testAITurnSender, req.Message, "", model, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	reply, escalate, turnError = applyEscalationPolicy(id, enhancedPrompt, tone, req.Message, history, reply, escalate, turnError)
	if escalate {
		reply = humanFacingHoldReply
	}
	// Samakan production: jalankan directive form/checkout + free-collection di simulator.
	if productStart, ok := handleProductCheckoutDirective(id, testAITurnSender, reply); ok {
		reply = productStart.reply
	} else if formStart, ok := handleAIFormDirective(id, testAITurnSender, reply); ok {
		reply = formStart.reply
	} else if productStart, ok := startProductFromFreeCollection(id, testAITurnSender, req.Message, routingText, reply); ok {
		reply = productStart.reply
	} else if formStart, ok := startAIFormFromFreeCollection(id, testAITurnSender, req.Message, routingText, reply); ok {
		reply = formStart.reply
	}
	latencyMs := time.Since(start).Milliseconds()
	logAITurn(id, testAITurnSender, req.Message, reply, model, knowledgeCount, usedShippingTool, escalate, turnError, latencyMs, trace)

	reply = services.LinkifyWhatsApp(reply, agent.Number) // nomor WA jadi tautan klik (kecuali nomor sendiri)

	var imageURL string
	if chatResult.AttachmentPath != "" {
		token := issueMediaToken(currentTenantID(c), id)
		if token != "" {
			if chatResult.AttachmentSrc == "product" && chatResult.AttachmentID > 0 {
				imageURL = fmt.Sprintf("/api/agents/%d/products/%d/image?token=%s", id, chatResult.AttachmentID, token)
			} else if chatResult.AttachmentSrc == "knowledge" && chatResult.AttachmentID > 0 {
				imageURL = fmt.Sprintf("/api/agents/%d/knowledge/%d/image?token=%s", id, chatResult.AttachmentID, token)
			}
		}
	}

	c.JSON(200, gin.H{
		"reply": reply, "escalate": escalate, "model": model,
		"image_url":          imageURL,
		"knowledge_count":    knowledgeCount,
		"retrieval_mode":     trace.RetrievalMode,
		"retrieval_query":    trace.RetrievalQuery,
		"top_similarity":     trace.TopSimilarity,
		"answer_overlap":     trace.AnswerOverlap,
		"product_ids":        trace.ProductIDs,
		"knowledge_ids":      trace.KnowledgeIDs,
		"grounding_retried":  trace.GroundingRetried,
		"grounding_fallback": trace.GroundingFallback,
		"response_policy":    trace.ResponsePolicy,
		"response_retried":   trace.ResponseRetried,
		"response_chars":     trace.ResponseChars,
	})
}

// AgentAnalytics mengembalikan ringkasan + tren 7 hari untuk satu agent.
func AgentAnalytics(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var totalIn, aiReplies, humanReplies, contacts, openHandoffs int64
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND message <> ''", id).Count(&totalIn)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND reply <> '' AND from_human = ?", id, false).Count(&aiReplies)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND from_human = ?", id, true).Count(&humanReplies)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ?", id).Distinct("sender").Count(&contacts)
	database.DB.Model(&models.Handoff{}).Where("agent_id = ?", id).Count(&openHandoffs)

	pct := 0
	if totalIn > 0 {
		pct = int(aiReplies * 100 / totalIn)
	}

	// Tren pesan masuk 7 hari terakhir.
	type dayRow struct {
		Day string
		Cnt int
	}
	var rows []dayRow
	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	database.DB.Model(&models.ChatHistory{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') as day, COUNT(*) as cnt").
		Where("agent_id = ? AND message <> '' AND created_at >= ?", id, since+" 00:00:00").
		Group("day").Scan(&rows)
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Day] = r.Cnt
	}
	trend := make([]gin.H, 0, 7)
	for i := 6; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		trend = append(trend, gin.H{"day": d, "count": counts[d]})
	}

	c.JSON(200, gin.H{
		"total_incoming": totalIn,
		"ai_replies":     aiReplies,
		"human_replies":  humanReplies,
		"contacts":       contacts,
		"open_handoffs":  openHandoffs,
		"ai_handled_pct": pct,
		"trend":          trend,
	})
}

// InboxContacts = daftar kontak (diurutkan dari yang terbaru) + penanda butuh manusia.
// Urutan memakai GREATEST(max created_at, last_msg_at) agar selaras WhatsApp Web:
// last_msg_at diisi dari LastMsgTimestamp WA / timestamp pesan (bukan read-receipt).
func InboxContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	db := database.DB.WithContext(c.Request.Context())

	type contactRow struct {
		Sender  string    `json:"sender"`
		LastAt  time.Time `json:"last_at"`
		LastMsg string    `json:"last_msg"`
	}
	var rows []contactRow
	// Ambil thread dari state chat yang sudah dibackfill dan selalu dimajukan saat
	// pesan masuk/keluar. State tetap menampilkan chat meski preview lokal tertunda.
	type activityRow struct {
		Sender string
		LastAt time.Time
	}
	var activities []activityRow
	// InboxReadState sudah dibackfill saat startup dan dimajukan atomik setiap
	// pesan. Menggunakannya menghindari GROUP BY seluruh chat_histories setiap
	// kali UI melakukan refresh daftar Inbox.
	// Read state dari HistorySync bisa berisi chat yang tidak punya satu pun
	// pesan tersimpan lokal (nomor hantu/arsip HP). Thread seperti itu hanya
	// menampilkan "+nomor" tanpa nama & preview, jadi disembunyikan kecuali
	// punya chat lokal atau kontak bernama. Group tetap masuk lewat CachedGroups.
	db.Model(&models.InboxReadState{}).
		Select("sender, last_msg_at AS last_at").
		Where("agent_id = ? AND sender <> '' AND last_msg_at IS NOT NULL", id).
		Where(`(
			EXISTS (SELECT 1 FROM chat_histories ch
				WHERE ch.agent_id = inbox_read_states.agent_id AND ch.sender = inbox_read_states.sender)
			OR EXISTS (SELECT 1 FROM contacts c
				WHERE c.agent_id = inbox_read_states.agent_id AND c.number = inbox_read_states.sender
					AND TRIM(COALESCE(c.name, '')) <> '')
		)`).
		Order("last_msg_at DESC, sender ASC").
		Scan(&activities)

	// Preview = tepat satu pesan lokal terbaru untuk SETIAP sender. Window per
	// sender mencegah satu thread panjang menghabiskan LIMIT global dan membuat
	// preview thread lain kosong.
	type rawContactRow struct {
		Sender    string
		CreatedAt time.Time
		Message   string
		Reply     string
		MediaType string
		Revoked   bool
	}
	sendersForPreview := make([]string, 0, len(activities))
	activityAt := make(map[string]time.Time, len(activities))
	localMaxBySender := make(map[string]time.Time, len(activities))
	for _, a := range activities {
		sendersForPreview = append(sendersForPreview, a.Sender)
		activityAt[a.Sender] = a.LastAt
	}
	previewBySender := make(map[string]string, len(activities))
	if len(sendersForPreview) > 0 {
		var rawRows []rawContactRow
		db.Raw(`
			SELECT ch.sender, ch.created_at, ch.message, ch.reply, ch.media_type, ch.revoked
			FROM chat_histories ch
			INNER JOIN (
				SELECT sender, MAX(created_at) AS latest_at
				FROM chat_histories
				WHERE agent_id = ? AND sender IN ?
				GROUP BY sender
			) latest
				ON latest.sender = ch.sender AND latest.latest_at = ch.created_at
			WHERE ch.agent_id = ?
			ORDER BY ch.sender ASC, ch.created_at DESC, ch.id DESC
		`, id, sendersForPreview, id).Scan(&rawRows)
		for _, raw := range rawRows {
			if _, exists := previewBySender[raw.Sender]; exists {
				continue
			}
			msg := "Pesan ini dihapus"
			if !raw.Revoked {
				msg = strings.TrimSpace(raw.Message)
				if msg == "" {
					msg = strings.TrimSpace(raw.Reply)
				}
				if msg == "" && raw.MediaType != "" {
					msg = "[" + raw.MediaType + "]"
				}
			}
			previewBySender[raw.Sender] = msg
			localMaxBySender[raw.Sender] = raw.CreatedAt
		}
	}
	// Read-only: jangan pernah "menyembuhkan" last_msg_at saat GET. Gap tetap
	// terlihat sampai pesan yang benar-benar ber-ID WhatsApp berhasil tersimpan.
	staleMap := make(map[string]bool, len(activities))
	for sender, waOrLocalLast := range activityAt {
		localLast, hasLocal := localMaxBySender[sender]
		if !hasLocal || waOrLocalLast.After(localLast) {
			staleMap[sender] = true
		}
	}
	rows = make([]contactRow, 0, len(activities))
	for _, a := range activities {
		rows = append(rows, contactRow{
			Sender:  a.Sender,
			LastAt:  a.LastAt,
			LastMsg: previewBySender[a.Sender],
		})
	}

	knownThreads := make(map[string]bool, len(rows))
	for _, row := range rows {
		knownThreads[row.Sender] = true
	}
	for _, group := range services.WA(id).CachedGroups() {
		if !knownThreads[group.JID] {
			rows = append(rows, contactRow{Sender: group.JID, LastMsg: "Grup WhatsApp"})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].LastAt.Equal(rows[j].LastAt) {
			return rows[i].Sender < rows[j].Sender
		}
		return rows[i].LastAt.After(rows[j].LastAt)
	})

	senders := make([]string, 0, len(rows))
	for _, r := range rows {
		senders = append(senders, r.Sender)
	}

	needsHuman := map[string]bool{}
	if len(senders) > 0 {
		var hs []models.Handoff
		db.Select("sender").Where("agent_id = ? AND sender IN ?", id, senders).Find(&hs)
		for _, h := range hs {
			needsHuman[h.Sender] = true
		}
	}

	names := map[string]string{}
	groupNames := map[string]string{}
	pauses := map[string]*time.Time{}
	if len(senders) > 0 {
		var cs []models.Contact
		db.Select(
			"number", "name", "manual_pause_until",
		).
			Where("agent_id = ? AND number IN ?", id, senders).Find(&cs)
		now := time.Now()
		for i := range cs {
			if cs[i].Name != "" {
				names[cs[i].Number] = cs[i].Name
			}
			if cs[i].ManualPauseUntil != nil && cs[i].ManualPauseUntil.After(now) {
				pauses[cs[i].Number] = cs[i].ManualPauseUntil
			}
		}
		var groupConfigs []models.GroupGuardConfig
		db.Select("group_jid", "group_name").
			Where("agent_id = ? AND group_jid IN ?", id, senders).Find(&groupConfigs)
		for _, group := range groupConfigs {
			if strings.TrimSpace(group.GroupName) != "" {
				groupNames[group.GroupJID] = group.GroupName
			}
		}
	}

	whatsappUnread := map[string]int{}
	whatsappSynced := map[string]bool{}
	contactLabels := map[string][]gin.H{}
	if len(senders) > 0 {
		var states []models.InboxReadState
		db.Where("agent_id = ? AND sender IN ?", id, senders).Find(&states)
		for _, state := range states {
			whatsappUnread[state.Sender] = state.WhatsAppUnreadCount
			whatsappSynced[state.Sender] = state.WhatsAppSynced
		}
		type inboxLabelRow struct {
			Sender  string
			LabelID string
			Name    string
			Color   int
		}
		var labelRows []inboxLabelRow
		db.Table("chat_labels AS cl").
			Select("cl.sender, cl.label_id, l.name, l.color").
			Joins("JOIN labels AS l ON l.agent_id = cl.agent_id AND l.label_id = cl.label_id").
			Where("cl.agent_id = ? AND cl.sender IN ?", id, senders).
			Order("l.name ASC").Scan(&labelRows)
		for _, label := range labelRows {
			contactLabels[label.Sender] = append(contactLabels[label.Sender], gin.H{
				"label_id": label.LabelID, "name": label.Name, "color": label.Color,
			})
		}
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		isGroup := services.IsGroupJID(r.Sender)
		if isGroup && names[r.Sender] == "" {
			if groupNames[r.Sender] != "" {
				names[r.Sender] = groupNames[r.Sender]
			} else if cachedName := services.WA(id).GroupName(r.Sender); cachedName != "" {
				names[r.Sender] = cachedName
			} else {
				names[r.Sender] = "Grup WhatsApp"
			}
		}
		msg := strings.TrimSpace(r.LastMsg)
		// Normalisasi whitespace preview agar list tidak "melebar".
		msg = strings.Join(strings.Fields(msg), " ")
		if len([]rune(msg)) > 64 {
			msg = string([]rune(msg)[:64]) + "…"
		}
		out = append(out, gin.H{
			"sender": r.Sender, "last_at": r.LastAt, "last_msg": msg,
			"preview_stale": staleMap[r.Sender],
			"is_group":      isGroup,
			"labels":        contactLabels[r.Sender],
			"needs_human":   needsHuman[r.Sender], "manual_pause_until": pauses[r.Sender],
			"name":                    names[r.Sender],
			"unread_count": func() int {
				if whatsappSynced[r.Sender] {
					return whatsappUnread[r.Sender]
				}
				return 0
			}(),
		})
	}
	c.JSON(200, gin.H{"data": out})
}

// MarkInboxConversationRead memajukan posisi baca lokal secara instan agar UI
// Inbox tidak lemot saat operator pilih chat. Sinkronisasi receipt ke WhatsApp
// dijalankan di background (best-effort) supaya API WA tidak memblokir klik.
func MarkInboxConversationRead(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Sender string `json:"sender"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Sender) == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}
	sender := strings.TrimSpace(req.Sender)
	var latest models.ChatHistory
	if err := database.DB.Where("agent_id = ? AND sender = ?", id, sender).
		Order("created_at DESC, id DESC").First(&latest).Error; err != nil {
		if services.IsGroupJID(sender) {
			c.JSON(200, gin.H{"ok": true, "sender": sender, "whatsapp_synced": true, "already_read": true})
			return
		}
		c.JSON(404, gin.H{"error": "Percakapan tidak ditemukan"})
		return
	}
	state := models.InboxReadState{AgentID: id, Sender: sender}
	if err := ensureInboxReadState(id, sender); err != nil {
		c.JSON(500, gin.H{"error": "Gagal membaca status percakapan"})
		return
	}
	if err := database.DB.Where("agent_id = ? AND sender = ?", id, sender).First(&state).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membaca status percakapan"})
		return
	}

	// Sudah terbaca sampai pesan terbaru → tidak perlu hit WA lagi.
	alreadyRead := state.LastReadAt != nil &&
		!state.LastReadAt.Before(latest.CreatedAt) &&
		state.WhatsAppUnreadCount == 0 &&
		state.WhatsAppSynced
	if alreadyRead {
		c.JSON(200, gin.H{"ok": true, "sender": sender, "whatsapp_synced": true, "already_read": true})
		return
	}

	// Ambil hanya ID pesan masuk yang belum dibaca (cap 40 agar ringan).
	var messageIDs []string
	unreadQuery := database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND TRIM(COALESCE(message, '')) <> '' AND wa_msg_id <> ''", id, sender)
	if state.LastReadAt != nil {
		unreadQuery = unreadQuery.Where("created_at > ?", *state.LastReadAt)
	}
	if err := unreadQuery.Order("created_at DESC, id DESC").Limit(40).Pluck("wa_msg_id", &messageIDs).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membaca status pesan"})
		return
	}
	// Balik ke urutan kronologis untuk receipt.
	for left, right := 0, len(messageIDs)-1; left < right; left, right = left+1, right-1 {
		messageIDs[left], messageIDs[right] = messageIDs[right], messageIDs[left]
	}

	// Update lokal dulu supaya badge unread hilang tanpa menunggu WA.
	changed, err := advanceInboxWAState(id, sender, latest.CreatedAt, map[string]any{
		"last_read_at": latest.CreatedAt, "whats_app_unread_count": 0,
	})
	if err != nil {
		c.JSON(500, gin.H{"error": "Gagal menandai chat sudah dibaca"})
		return
	}
	if changed {
		publishInboxEvent(id, sender, "read")
	}
	logCSActivity(c, id, sender, "read", "Membuka percakapan")

	// Sinkron ke perangkat WhatsApp di background — jangan blokir response.
	if len(messageIDs) > 0 {
		go func(agentID uint, phone string, ids []string) {
			_ = services.WA(agentID).MarkConversationRead(phone, ids)
		}(id, sender, append([]string(nil), messageIDs...))
	}
	c.JSON(200, gin.H{"ok": true, "sender": sender, "whatsapp_synced": true, "pending_wa_sync": len(messageIDs) > 0})
}

// DeleteInboxConversation menghapus riwayat chat satu kontak dari inbox agent.
// Menghapus: chat_histories, handoff, conversation_memory, ai_turns untuk sender itu.
// Tidak menghapus data CRM (contacts) — hanya menghilangkan thread di Inbox.
func DeleteInboxConversation(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimSpace(c.Query("sender"))
	if sender == "" {
		sender = strings.TrimSpace(c.Param("sender"))
	}
	if sender == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}

	// Ambil path media dulu (opsional hapus file di disk).
	var mediaPaths []string
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND media_path != '' AND media_path IS NOT NULL", id, sender).
		Pluck("media_path", &mediaPaths)

	tx := database.DB.Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal memulai penghapusan chat"})
		return
	}
	res := tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.ChatHistory{})
	if res.Error != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"error": "Gagal menghapus riwayat chat"})
		return
	}
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.Handoff{}).Error
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.ConversationMemory{}).Error
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.AITurn{}).Error
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.InboxReadState{}).Error
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyelesaikan penghapusan chat"})
		return
	}

	// Best-effort hapus file media di disk (abaikan error).
	seen := map[string]bool{}
	for _, p := range mediaPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		_ = os.Remove(p)
	}
	publishInboxEvent(id, sender, "delete")

	c.JSON(200, gin.H{
		"message":       "Chat dihapus dari inbox",
		"sender":        sender,
		"deleted_chats": res.RowsAffected,
		"deleted_media": len(seen),
	})
}

const (
	defaultInboxConversationLimit = 100
	maxInboxConversationLimit     = 200
)

func inboxConversationLimit(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return defaultInboxConversationLimit
	}
	if limit > maxInboxConversationLimit {
		return maxInboxConversationLimit
	}
	return limit
}

func parseInboxConversationCursor(rawAt, rawID string) (time.Time, uint, error) {
	rawAt = strings.TrimSpace(rawAt)
	rawID = strings.TrimSpace(rawID)
	if rawAt == "" && rawID == "" {
		return time.Time{}, 0, nil
	}
	if rawAt == "" || rawID == "" {
		return time.Time{}, 0, fmt.Errorf("cursor percakapan tidak lengkap")
	}
	beforeAt, err := time.Parse(time.RFC3339Nano, rawAt)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("waktu cursor percakapan tidak valid")
	}
	beforeID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || beforeID == 0 {
		return time.Time{}, 0, fmt.Errorf("ID cursor percakapan tidak valid")
	}
	return beforeAt, uint(beforeID), nil
}

// InboxConversation memuat jendela pesan terbaru. Riwayat lama tetap tersedia
// melalui cursor before_at + before_id agar setiap halaman lama hanya diunduh
// sekali dan pesan terbaru tetap dapat disegarkan secara terpisah.
func InboxConversation(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	sender := c.Query("sender")
	if sender == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}
	sender = strings.TrimPrefix(strings.TrimSpace(sender), "+")
	limit := inboxConversationLimit(c.Query("limit"))
	beforeAt, beforeID, cursorErr := parseInboxConversationCursor(
		c.Query("before_at"),
		c.Query("before_id"),
	)
	if cursorErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": cursorErr.Error()})
		return
	}
	// Jangan load blob media_metadata ke memori. Metadata historis dapat berukuran
	// besar dan sebelumnya tetap terbaca karena SELECT chat_histories.*.
	var msgs []models.ChatHistory
	query := db.Model(&models.ChatHistory{}).
		Omit("MediaMetadata").
		Where("agent_id = ? AND sender = ?", id, sender)
	if beforeID > 0 {
		query = query.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			beforeAt, beforeAt, beforeID,
		)
	}
	result := query.
		Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&msgs)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat percakapan"})
		return
	}
	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}
	var nextBeforeAt *time.Time
	var nextBeforeID uint
	if hasMore && len(msgs) > 0 {
		oldest := msgs[len(msgs)-1]
		cursorAt := oldest.CreatedAt
		nextBeforeAt = &cursorAt
		nextBeforeID = oldest.ID
	}

	// Ambil hanya ID yang punya metadata, tanpa pernah memindahkan isi blob.
	messageIDs := make([]uint, 0, len(msgs))
	for i := range msgs {
		messageIDs = append(messageIDs, msgs[i].ID)
	}
	metadataAvailable := make(map[uint]struct{}, len(messageIDs))
	if len(messageIDs) > 0 {
		var metadataIDs []uint
		db.Model(&models.ChatHistory{}).
			Where("id IN ? AND media_metadata IS NOT NULL AND LENGTH(media_metadata) > 0", messageIDs).
			Pluck("id", &metadataIDs)
		for _, messageID := range metadataIDs {
			metadataAvailable[messageID] = struct{}{}
		}
	}

	for i := range msgs {
		msgs[i].MediaMetadata = nil
		msgs[i].MediaAvailable = strings.TrimSpace(msgs[i].MediaPath) != ""
		_, hasMediaMetadata := metadataAvailable[msgs[i].ID]
		msgs[i].MediaDownloadable = msgs[i].MediaAvailable || hasMediaMetadata
	}
	for left, right := 0, len(msgs)-1; left < right; left, right = left+1, right-1 {
		msgs[left], msgs[right] = msgs[right], msgs[left]
	}
	// Lengkapi reply_text dari pesan yang di-quote (wa_msg_id) supaya UI quote + scroll
	// tetap jalan untuk data lama yang tidak menyimpan preview.
	enrichConversationReplyPreviews(msgs)
	var h int64
	db.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", id, sender).Count(&h)
	var contact models.Contact
	db.Select("manual_pause_until").Where("agent_id = ? AND number = ?", id, sender).First(&contact)
	var pauseUntil *time.Time
	if contact.ManualPauseUntil != nil && contact.ManualPauseUntil.After(time.Now()) {
		pauseUntil = contact.ManualPauseUntil
	}
	c.JSON(200, gin.H{
		"data": msgs, "sender": sender, "has_more": hasMore, "loaded_count": len(msgs),
		"next_before_at": nextBeforeAt, "next_before_id": nextBeforeID,
		"needs_human": h > 0, "manual_pause_until": pauseUntil,
		"media_token": issueMediaToken(currentTenantID(c), id),
	})
}

// enrichConversationReplyPreviews mengisi ReplyText kosong dari pesan target di batch
// yang sama (cocok exact/suffix pada wa_msg_id). Tidak menulis ke database.
func enrichConversationReplyPreviews(msgs []models.ChatHistory) {
	if len(msgs) == 0 {
		return
	}
	byWA := make(map[string]*models.ChatHistory, len(msgs))
	for i := range msgs {
		wa := strings.TrimSpace(msgs[i].WAMsgID)
		if wa == "" {
			continue
		}
		byWA[wa] = &msgs[i]
		byWA[strings.ToLower(wa)] = &msgs[i]
	}
	previewOf := func(m *models.ChatHistory) string {
		if m == nil {
			return ""
		}
		if t := strings.TrimSpace(m.Message); t != "" {
			return truncateChatPreview(t, 160)
		}
		if t := strings.TrimSpace(m.Reply); t != "" {
			return truncateChatPreview(t, 160)
		}
		switch m.MediaType {
		case "image", "sticker":
			return "📷 Foto"
		case "video":
			return "🎥 Video"
		case "audio":
			return "🎵 Audio"
		case "document":
			if m.FileName != "" {
				return "📄 " + m.FileName
			}
			return "📄 Dokumen"
		case "location":
			return "📍 Lokasi"
		}
		return ""
	}
	findTarget := func(replyTo string) *models.ChatHistory {
		replyTo = strings.TrimSpace(replyTo)
		if replyTo == "" {
			return nil
		}
		if hit := byWA[replyTo]; hit != nil {
			return hit
		}
		if hit := byWA[strings.ToLower(replyTo)]; hit != nil {
			return hit
		}
		if len(replyTo) < 8 {
			return nil
		}
		for wa, m := range byWA {
			if strings.EqualFold(wa, replyTo) || strings.HasSuffix(wa, replyTo) || strings.HasSuffix(replyTo, wa) {
				return m
			}
		}
		return nil
	}
	for i := range msgs {
		rt := strings.TrimSpace(msgs[i].ReplyTo)
		if rt == "" {
			continue
		}
		target := findTarget(rt)
		if target == nil {
			// Fallback: pesan media terdekat sebelum pesan ini (data lama tanpa match ID).
			for j := i - 1; j >= 0 && j >= i-12; j-- {
				if strings.TrimSpace(msgs[j].MediaType) != "" {
					target = &msgs[j]
					break
				}
			}
		}
		if target == nil {
			continue
		}
		if strings.TrimSpace(msgs[i].ReplyText) == "" {
			if prev := previewOf(target); prev != "" {
				msgs[i].ReplyText = prev
			}
		}
		// Pastikan reply_to mengarah ke id yang benar-benar ada di batch (untuk scroll FE).
		if wa := strings.TrimSpace(target.WAMsgID); wa != "" {
			msgs[i].ReplyTo = wa
		} else {
			// Tanpa wa_msg_id: pakai id lokal bertanda "local:" agar FE bisa scroll.
			msgs[i].ReplyTo = fmt.Sprintf("local:%d", target.ID)
		}
	}
}

func truncateChatPreview(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if max <= 0 || len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

// ReanalyzeInboxImage menjalankan ulang vision pada media yang sudah tersimpan.
// Instruksi CS hanya memengaruhi analisis ini dan tidak dikirim sebagai pesan pelanggan.
func ReanalyzeInboxImage(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format instruksi tidak valid"})
		return
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if len([]rune(req.Instruction)) > 800 {
		c.JSON(400, gin.H{"error": "Instruksi maksimal 800 karakter"})
		return
	}
	var row models.ChatHistory
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), id).First(&row).Error != nil {
		c.JSON(404, gin.H{"error": "Pesan gambar tidak ditemukan"})
		return
	}
	if row.MediaPath == "" || (row.MediaType != "image" && row.MediaType != "sticker") {
		c.JSON(400, gin.H{"error": "Pesan ini bukan gambar yang dapat dianalisis"})
		return
	}
	data, err := os.ReadFile(row.MediaPath)
	if err != nil {
		c.JSON(404, gin.H{"error": "File gambar sudah tidak tersedia"})
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil {
		c.JSON(404, gin.H{"error": "Nomor tidak ditemukan"})
		return
	}
	prompt := strings.TrimSpace(agent.SystemPrompt)
	if prompt == "" {
		prompt = "Kamu adalah asisten AI bisnis yang ramah dan akurat."
	}
	tone := agent.Tone
	if tone == "" {
		tone = "ramah"
	}
	var history []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND id < ?", id, row.Sender, row.ID).Order("id desc").Limit(12).Find(&history)
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	result, err := services.AnalyzeCustomerImage(id, prompt, tone, row.Message, req.Instruction, row.Mimetype, data, history)
	if err != nil {
		failureText := "Analisis ulang gagal: " + err.Error()
		database.DB.Model(&row).Updates(map[string]any{
			"image_analysis_status": "failed", "image_analysis": failureText,
			"image_analysis_model": "", "image_analysis_confidence": 0,
			"image_analysis_answer": "", "image_analysis_product_id": 0,
			"image_analysis_needs_human": true,
		})
		publishInboxEvent(id, row.Sender, "analysis")
		name := contactNames(id)[row.Sender]
		dispatchStoredImageAnalysisWebhook(id, row, name, "failed", services.VisionAnalysisResult{Analysis: failureText}, true)
		c.JSON(502, gin.H{"error": "Analisis ulang gagal: " + err.Error()})
		return
	}
	needsHuman := result.NeedsHuman || result.Confidence < 0.55
	database.DB.Model(&row).Updates(map[string]any{
		"image_analysis": result.Analysis, "image_analysis_status": "completed",
		"image_analysis_model": result.Model, "image_analysis_confidence": result.Confidence,
		"image_analysis_answer": result.Answer, "image_analysis_product_id": result.ProductID,
		"image_analysis_needs_human": needsHuman,
	})
	publishInboxEvent(id, row.Sender, "analysis")
	if needsHuman {
		_ = database.DB.Where(models.Handoff{AgentID: id, Sender: row.Sender}).
			Assign(models.Handoff{LastMsg: row.Message}).FirstOrCreate(&models.Handoff{}).Error
	}
	name := contactNames(id)[row.Sender]
	dispatchStoredImageAnalysisWebhook(id, row, name, "completed", result, needsHuman)
	c.JSON(200, gin.H{"data": gin.H{
		"image_analysis": result.Analysis, "image_analysis_status": "completed",
		"image_analysis_model": result.Model, "image_analysis_confidence": result.Confidence,
		"image_analysis_answer": result.Answer, "image_analysis_product_id": result.ProductID,
		"image_analysis_needs_human": needsHuman,
	}})
}

// InboxSend mengirim pesan manual dari dashboard ke kontak (ambil alih dari bot).
func InboxSend(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		To        string `json:"to"`
		Message   string `json:"message"`
		ReplyTo   string `json:"reply_to"`
		ReplyText string `json:"reply_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "Tujuan & pesan wajib diisi"})
		return
	}
	// Pause AI dipasang sebelum kirim agar balasan otomatis yang sedang menunggu
	// langsung dibatalkan. Pesan operator sendiri dikirim tanpa artificial delay.
	pauseUntil := pauseAIForManualReply(id, req.To)
	var err error
	var waMsgID string
	if req.ReplyTo != "" && !services.IsGroupJID(req.To) {
		waMsgID, err = services.WA(id).SendImmediateReplyAndGetID(req.To, req.Message, req.ReplyTo)
	} else {
		waMsgID, err = services.WA(id).SendImmediateTextAndGetID(req.To, req.Message)
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, services.ErrWASendTimeout) {
			status = http.StatusGatewayTimeout
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().Truncate(time.Second)
	row := models.ChatHistory{
		AgentID: id, Sender: req.To, Reply: req.Message, FromHuman: true,
		WAMsgID: waMsgID, ReplyTo: req.ReplyTo, ReplyText: req.ReplyText,
		DeliveryStatus: "sent", CreatedAt: now,
	}
	result := database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		// Pesan sudah terkirim ke WhatsApp; jangan membalas 5xx yang dapat membuat
		// operator mengirim ulang. Laporkan kegagalan pencatatan secara eksplisit.
		log.Printf("Pesan WA terkirim tetapi gagal dicatat (agent %d, %s, wa=%s): %v", id, req.To, waMsgID, result.Error)
		logCSActivity(c, id, req.To, "reply", req.Message)
		c.JSON(200, gin.H{"ok": true, "recorded": false, "wa_msg_id": waMsgID, "warning": "Pesan terkirim, tetapi Inbox belum berhasil mencatatnya"})
		return
	}
	// Echo perangkat dapat menang race INSERT setelah WhatsApp mengakui pesan.
	// Ambil row kanoniknya agar frontend tetap menerima ID lokal yang valid.
	if result.RowsAffected == 0 || row.ID == 0 {
		if queryErr := database.DB.Where("agent_id = ? AND wa_msg_id = ?", id, waMsgID).
			First(&row).Error; queryErr != nil {
			log.Printf("Pesan WA terkirim tetapi row kanonik belum ditemukan (agent %d, %s, wa=%s): %v", id, req.To, waMsgID, queryErr)
			logCSActivity(c, id, req.To, "reply", req.Message)
			c.JSON(200, gin.H{"ok": true, "recorded": false, "wa_msg_id": waMsgID, "warning": "Pesan terkirim, tetapi Inbox perlu menyegarkan riwayat"})
			return
		}
	}
	touchInboxLastMsg(id, req.To, now)
	logCSActivity(c, id, req.To, "reply", req.Message)

	c.JSON(200, gin.H{
		"ok": true, "recorded": true, "wa_msg_id": waMsgID,
		"message": row, "manual_pause_until": pauseUntil,
	})
}

// ChatPresence mengirim indikator "mengetik" ke kontak (dipanggil dari Inbox saat CS mengetik).
func ChatPresence(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		To     string `json:"to"`
		Active bool   `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" {
		c.JSON(400, gin.H{"error": "to wajib"})
		return
	}
	debugStartedAt := time.Now()
	err := services.WA(id).TypingContext(c.Request.Context(), req.To, req.Active)
	debugDuration := time.Since(debugStartedAt)
	if err != nil || debugDuration >= 250*time.Millisecond {
		log.Printf("[inbox-debug] typing presence agent=%d active=%t durasi=%s err=%v",
			id, req.Active, debugDuration.Round(time.Millisecond), err)
	}
	c.JSON(200, gin.H{"ok": true})
}

// RevokeMessage menghapus (unsend) pesan yang sudah dikirim.
func RevokeMessage(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	msgID := c.Param("msgId")
	if msgID == "" {
		c.JSON(400, gin.H{"error": "msgId wajib"})
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" {
		c.JSON(400, gin.H{"error": "to wajib"})
		return
	}
	if err := services.WA(id).RevokeMessage(req.To, types.MessageID(msgID)); err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	// Tandai pesan sebagai revoked di DB (tampilkan "Pesan ini dihapus" di Inbox)
	if err := database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND wa_msg_id = ?", id, msgID).
		Update("revoked", true).Error; err == nil {
		inboxEvents.publish(id, req.To, "revoke", msgID)
	}
	c.JSON(200, gin.H{"ok": true})
}

// InboxSendMedia mengirim gambar/file dari dashboard ke kontak (ambil alih dari bot).
func InboxSendMedia(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	to := c.PostForm("to")
	caption := c.PostForm("caption")
	if to == "" {
		c.JSON(400, gin.H{"error": "Nomor tujuan wajib"})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "File wajib diunggah"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "Gagal membaca file"})
		return
	}
	defer f.Close()
	const maxInboxMediaBytes = 64 << 20
	data, readErr := io.ReadAll(io.LimitReader(f, maxInboxMediaBytes+1))
	if readErr != nil {
		c.JSON(400, gin.H{"error": "Gagal membaca media"})
		return
	}
	if len(data) > maxInboxMediaBytes {
		c.JSON(413, gin.H{"error": "Ukuran media maksimal 64 MB"})
		return
	}
	pauseUntil := pauseAIForManualReply(id, to)

	mimetype := fh.Header.Get("Content-Type")
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	var sendErr error
	var waMsgID string
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		waMsgID, sendErr = services.WA(id).SendImageAndGetID(to, caption, mimetype, data)
	case strings.HasPrefix(mimetype, "video/"):
		waMsgID, sendErr = services.WA(id).SendVideoAndGetID(to, caption, mimetype, data)
	default:
		waMsgID, sendErr = services.WA(id).SendDocumentAndGetID(to, fh.Filename, mimetype, caption, data)
	}
	if sendErr != nil {
		status := http.StatusBadGateway
		if errors.Is(sendErr, services.ErrWASendTimeout) {
			status = http.StatusGatewayTimeout
		}
		c.JSON(status, gin.H{"error": sendErr.Error()})
		return
	}

	mediaType := "document"
	if strings.HasPrefix(mimetype, "image/") {
		mediaType = "image"
	} else if strings.HasPrefix(mimetype, "video/") {
		mediaType = "video"
	}
	reply := caption
	if reply == "" {
		reply = mediaPlaceholder(mediaType, fh.Filename)
	}
	now := time.Now()
	row := models.ChatHistory{
		AgentID: id, Sender: to, Reply: reply, FromHuman: true,
		MediaType: mediaType, MediaPath: storeMedia(id, data, mimetype, fh.Filename),
		FileName: fh.Filename, Mimetype: mimetype,
		WAMsgID: waMsgID, DeliveryStatus: "sent", CreatedAt: now,
	}
	result := database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		log.Printf("Media WA terkirim tetapi gagal dicatat (agent %d, %s, wa=%s): %v", id, to, waMsgID, result.Error)
		logCSActivity(c, id, to, "reply_media", reply)
		c.JSON(200, gin.H{"ok": true, "recorded": false, "wa_msg_id": waMsgID, "warning": "Media terkirim, tetapi Inbox belum berhasil mencatatnya"})
		return
	}
	if result.RowsAffected == 0 || row.ID == 0 {
		if queryErr := database.DB.Where("agent_id = ? AND wa_msg_id = ?", id, waMsgID).
			First(&row).Error; queryErr != nil {
			log.Printf("Media WA terkirim tetapi row kanonik belum ditemukan (agent %d, %s, wa=%s): %v", id, to, waMsgID, queryErr)
			logCSActivity(c, id, to, "reply_media", reply)
			c.JSON(200, gin.H{"ok": true, "recorded": false, "wa_msg_id": waMsgID, "warning": "Media terkirim, tetapi Inbox perlu menyegarkan riwayat"})
			return
		}
	}
	row.MediaAvailable = strings.TrimSpace(row.MediaPath) != ""
	row.MediaDownloadable = row.MediaAvailable || len(row.MediaMetadata) > 0
	row.MediaMetadata = nil
	touchInboxLastMsg(id, to, now)
	// Media keluar adalah balasan operator, sama seperti teks manual. Jangan
	// membuat antrean Butuh CS: handoff hanya berasal dari pesan masuk yang
	// benar-benar memerlukan eskalasi. pauseAIForManualReply di atas sudah
	// mencegah AI aktif menyela percakapan selama jeda manual.
	logCSActivity(c, id, to, "reply_media", reply)
	c.JSON(200, gin.H{
		"ok": true, "recorded": true, "wa_msg_id": waMsgID,
		"message": row, "manual_pause_until": pauseUntil,
	})
}

// ServeMedia menyajikan file media sebuah pesan. Auth lewat ?token= (header tak bisa di <img>/<a>).
func ServeMedia(c *gin.Context) {
	tid, tokenAgentID, ok := tenantFromToken(c.Query("token"))
	if !ok {
		c.AbortWithStatus(401)
		return
	}
	agentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	if tokenAgentID > 0 && tokenAgentID != uint(agentID) {
		c.AbortWithStatus(403)
		return
	}
	var agent models.Agent
	if database.DB.Select("id").Where("id = ? AND tenant_id = ?", agentID, tid).First(&agent).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	var ch models.ChatHistory
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), agentID).First(&ch).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	if ch.MediaPath == "" && len(ch.MediaMetadata) > 0 {
		key := strconv.Itoa(agentID) + ":" + c.Param("cid")
		lockValue, _ := historicalMediaLocks.LoadOrStore(key, &sync.Mutex{})
		lock := lockValue.(*sync.Mutex)
		lock.Lock()
		defer func() { lock.Unlock(); historicalMediaLocks.Delete(key) }()

		// Permintaan lain mungkin sudah menyelesaikan unduhan saat kita menunggu lock.
		if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), agentID).First(&ch).Error == nil && ch.MediaPath == "" {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
			defer cancel()
			data, downloadErr := services.WA(uint(agentID)).DownloadHistoricalMedia(ctx, ch.MediaMetadata)
			if downloadErr != nil {
				if err := database.DB.Model(&ch).Update("media_fetch_status", "failed").Error; err == nil {
					publishInboxEvent(uint(agentID), ch.Sender, "media")
				}
				if strings.Contains(strings.ToLower(downloadErr.Error()), "belum terhubung") {
					c.JSON(503, gin.H{"error": "WhatsApp belum terhubung. Coba lagi setelah agent online."})
				} else {
					c.JSON(410, gin.H{"error": "Media lama sudah tidak tersedia di WhatsApp."})
				}
				return
			}
			path := storeMedia(uint(agentID), data, ch.Mimetype, ch.FileName)
			if path == "" {
				c.JSON(500, gin.H{"error": "Media berhasil diambil tetapi gagal disimpan."})
				return
			}
			ch.MediaPath = path
			if err := database.DB.Model(&ch).Updates(map[string]interface{}{"media_path": path, "media_fetch_status": "available"}).Error; err == nil {
				publishInboxEvent(uint(agentID), ch.Sender, "media")
			}
		}
	}
	if ch.MediaPath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(ch.MediaPath)
}

// ServeProfilePicture mem-proxy lokasi foto profil WhatsApp dengan cache ringan.
// Kontak yang menyembunyikan/tidak memiliki foto akan tetap memakai avatar inisial di UI.
func ServeProfilePicture(c *gin.Context) {
	tid, tokenAgentID, ok := tenantFromToken(c.Query("token"))
	if !ok {
		c.AbortWithStatus(401)
		return
	}
	agentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	if tokenAgentID > 0 && tokenAgentID != uint(agentID) {
		c.AbortWithStatus(403)
		return
	}
	var agent models.Agent
	if database.DB.Select("id").Where("id = ? AND tenant_id = ?", agentID, tid).First(&agent).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	sender := strings.TrimPrefix(strings.TrimSpace(c.Query("sender")), "+")
	if sender == "" {
		c.AbortWithStatus(400)
		return
	}
	key := strconv.Itoa(agentID) + ":" + sender
	if cached, found := profilePictureCache.Load(key); found {
		entry := cached.(profilePictureCacheEntry)
		if time.Now().Before(entry.ExpiresAt) {
			if entry.URL == "" {
				c.Header("Cache-Control", "private, max-age=600")
				c.Status(http.StatusNoContent)
			} else {
				c.Header("Cache-Control", "private, max-age=900")
				c.Redirect(http.StatusTemporaryRedirect, entry.URL)
			}
			return
		}
		profilePictureCache.Delete(key)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	url, pictureErr := services.WA(uint(agentID)).ProfilePictureURL(ctx, sender)
	if pictureErr != nil || url == "" {
		if pictureErr != nil && strings.Contains(strings.ToLower(pictureErr.Error()), "belum terhubung") {
			c.AbortWithStatus(503)
			return
		}
		profilePictureCache.Store(key, profilePictureCacheEntry{ExpiresAt: time.Now().Add(10 * time.Minute)})
		c.Header("Cache-Control", "private, max-age=600")
		c.Status(http.StatusNoContent)
		return
	}
	profilePictureCache.Store(key, profilePictureCacheEntry{URL: url, ExpiresAt: time.Now().Add(30 * time.Minute)})
	c.Header("Cache-Control", "private, max-age=900")
	c.Redirect(http.StatusTemporaryRedirect, url)
}
