package handlers

import (
	"strings"
	"time"

	"kirimwa/backend/database"
	"kirimwa/backend/models"
	"kirimwa/backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

// GetConversationBrief — GET /agents/:id/conversation/brief?sender=
// Mengembalikan ringkasan operasional untuk CS (cache bila masih segar).
func GetConversationBrief(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimSpace(c.Query("sender"))
	if sender == "" {
		c.JSON(400, gin.H{"error": "Parameter sender wajib"})
		return
	}
	force := c.Query("refresh") == "1" || c.Query("force") == "1"
	brief, err := loadOrBuildBrief(id, sender, force)
	if err != nil {
		c.JSON(502, gin.H{"error": "Ringkasan belum bisa dibuat: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": brief})
}

// RefreshConversationBrief — POST /agents/:id/conversation/brief  body: {"sender":"..."}
func RefreshConversationBrief(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Sender string `json:"sender"`
	}
	_ = c.ShouldBindJSON(&req)
	sender := strings.TrimSpace(req.Sender)
	if sender == "" {
		sender = strings.TrimSpace(c.Query("sender"))
	}
	if sender == "" {
		c.JSON(400, gin.H{"error": "Parameter sender wajib"})
		return
	}
	brief, err := loadOrBuildBrief(id, sender, true)
	if err != nil {
		c.JSON(502, gin.H{"error": "Ringkasan belum bisa dibuat: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": brief})
}

func loadOrBuildBrief(agentID uint, sender string, force bool) (services.ConversationBrief, error) {
	var mem models.ConversationMemory
	_ = database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&mem).Error

	var last models.ChatHistory
	_ = database.DB.Select("id").Where("agent_id = ? AND sender = ?", agentID, sender).Order("id desc").First(&last).Error

	var ho int64
	database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", agentID, sender).Count(&ho)
	needsHuman := ho > 0

	// Cache hit hanya jika algoritma, pesan terakhir, dan status handoff masih sama.
	// Cache lama/stale dibangun ulang secara heuristik (lokal, tanpa API AI), jadi
	// klik chat tetap ringan tetapi tidak menampilkan ringkasan basi berhari-hari.
	if !force {
		if cached, ok := services.DecodeBrief(mem.BriefJSON); ok &&
			cached.Version == services.ConversationBriefVersion &&
			mem.BriefChatID == last.ID &&
			cached.NeedsHuman == needsHuman {
			cached.Stale = false
			return cached, nil
		}
		// Tanpa cache valid: bangun heuristik ringan (tanpa AI) supaya UI langsung
		// mendapat kondisi aktual, termasuk siapa yang sedang ditunggu dan media.
		var recent []models.ChatHistory
		if err := database.DB.Select(
			"id", "message", "reply", "from_human", "media_type", "file_name", "created_at",
		).
			Where("agent_id = ? AND sender = ?", agentID, sender).
			Order("id desc").Limit(60).Find(&recent).Error; err != nil {
			return services.ConversationBrief{}, err
		}
		msgs := make([]models.ChatHistory, 0, len(recent))
		for i := len(recent) - 1; i >= 0; i-- {
			msgs = append(msgs, recent[i])
		}
		brief, err := services.BuildConversationBriefHeuristic(agentID, sender, msgs, mem.Summary, needsHuman)
		if err != nil {
			return brief, err
		}
		// Simpan hasil heuristik agar GET berikutnya instant; AI hanya di refresh manual.
		brief.Stale = false
		if err := storeConversationBrief(agentID, sender, brief); err != nil {
			return brief, err
		}
		return brief, nil
	}

	// Force rebuild (tombol refresh): boleh pakai AI.
	var recent []models.ChatHistory
	if err := database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("id desc").Limit(80).Find(&recent).Error; err != nil {
		return services.ConversationBrief{}, err
	}
	msgs := make([]models.ChatHistory, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		msgs = append(msgs, recent[i])
	}

	brief, err := services.BuildConversationBrief(agentID, sender, msgs, mem.Summary, needsHuman, force)
	if err != nil {
		return brief, err
	}

	brief.Stale = false
	if err := storeConversationBrief(agentID, sender, brief); err != nil {
		return brief, err
	}
	return brief, nil
}

// storeConversationBrief mengubah kolom cache brief saja. Long-term memory dapat
// diperbarui goroutine AI pada waktu bersamaan; upsert selektif ini mencegah
// kedua penulis saling menimpa summary/checkpoint atau brief terbaru.
func storeConversationBrief(agentID uint, sender string, brief services.ConversationBrief) error {
	now := time.Now()
	row := models.ConversationMemory{
		AgentID:     agentID,
		Sender:      sender,
		BriefJSON:   services.EncodeBrief(brief),
		BriefChatID: brief.LastChatID,
		BriefAt:     &now,
	}
	return database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "agent_id"}, {Name: "sender"}},
		DoUpdates: clause.Assignments(map[string]any{
			"brief_json":    row.BriefJSON,
			"brief_chat_id": row.BriefChatID,
			"brief_at":      row.BriefAt,
			"updated_at":    now,
		}),
	}).Create(&row).Error
}
