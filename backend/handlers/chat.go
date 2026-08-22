package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

func ChatHistory(c *gin.Context) {
	var chats []models.ChatHistory
	database.DB.Where("agent_id = ?", currentAgentID(c)).Order("created_at desc").Limit(50).Find(&chats)
	c.JSON(200, gin.H{"data": chats})
}

// Settings = persona & tone milik agent (back-compat: tanpa :id pakai agent default 1).
func GetSettings(c *gin.Context) {
	var a models.Agent
	database.DB.First(&a, currentAgentID(c))
	c.JSON(200, gin.H{"data": gin.H{"system_prompt": a.SystemPrompt, "tone": a.Tone, "ai_model": "deepseek-v4-pro"}})
}

func UpdateSettings(c *gin.Context) {
	var req struct {
		SystemPrompt string `json:"system_prompt"`
		Tone         string `json:"tone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	var a models.Agent
	if database.DB.First(&a, currentAgentID(c)).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	a.SystemPrompt = req.SystemPrompt
	if req.Tone != "" {
		a.Tone = req.Tone
	}
	database.DB.Save(&a)
	c.JSON(200, gin.H{"data": gin.H{"system_prompt": a.SystemPrompt, "tone": a.Tone}})
}

func attachKnowledgeImageURLs(c *gin.Context, agentID uint, kb []models.Knowledge) {
	token := issueMediaToken(currentTenantID(c), agentID)
	if token == "" {
		return
	}
	for i := range kb {
		if kb[i].ImagePath != "" {
			kb[i].ImageURL = fmt.Sprintf("/api/agents/%d/knowledge/%d/image?token=%s", agentID, kb[i].ID, token)
		}
	}
}

func ServeKnowledgeImage(c *gin.Context) {
	tid, tokenAgentID, ok := tenantFromToken(c.Query("token"))
	if !ok {
		if user, exists := c.Get("user"); exists {
			if u, valid := user.(models.User); valid {
				if u.TenantID != nil {
					tid = *u.TenantID
				}
				ok = true
			}
		}
	}
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
	var k models.Knowledge
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("kid"), agentID).First(&k).Error != nil || k.ImagePath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(k.ImagePath)
}

func saveKnowledgeImage(c *gin.Context, agentID uint, k *models.Knowledge) error {
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		return nil
	}
	defer file.Close()
	data, readErr := io.ReadAll(file)
	if readErr != nil {
		return fmt.Errorf("Gagal membaca gambar")
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return fmt.Errorf("File harus berupa gambar (jpg, png, webp)")
	}
	if k.ImagePath != "" {
		_ = os.Remove(k.ImagePath)
	}
	filename := fmt.Sprintf("knowledge_%d_%d%s", agentID, time.Now().UnixNano(), filepath.Ext(header.Filename))
	path := filepath.Join("data", "knowledge", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("Gagal membuat direktori gambar")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("Gagal menyimpan gambar")
	}
	k.ImagePath = path
	k.ImageMime = mime
	return nil
}

func ListKnowledge(c *gin.Context) {
	aid := currentAgentID(c)
	var kb []models.Knowledge
	database.DB.Where("agent_id = ?", aid).Order("created_at desc").Find(&kb)
	attachKnowledgeImageURLs(c, aid, kb)
	c.JSON(200, gin.H{"data": kb})
}

func CreateKnowledge(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var question, answer, tags string
	contentType := c.ContentType()
	if strings.Contains(contentType, "multipart/form-data") {
		question = strings.TrimSpace(c.PostForm("question"))
		answer = strings.TrimSpace(c.PostForm("answer"))
		tags = strings.TrimSpace(c.PostForm("tags"))
	} else {
		var req struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
			Tags     string `json:"tags"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		question = strings.TrimSpace(req.Question)
		answer = strings.TrimSpace(req.Answer)
		tags = strings.TrimSpace(req.Tags)
	}

	if question == "" || answer == "" {
		c.JSON(400, gin.H{"error": "Pertanyaan & jawaban wajib diisi"})
		return
	}
	writer := newKnowledgeUpserter(aid)
	merged := writer.findDuplicate(question, answer) != nil
	k, _, err := writer.save(question, answer, tags, "manual", "")
	if err != nil {
		c.JSON(500, gin.H{"error": "FAQ belum bisa disimpan"})
		return
	}

	// Handle optional image upload
	if strings.Contains(contentType, "multipart/form-data") {
		if err := saveKnowledgeImage(c, aid, &k); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if k.ImagePath != "" {
			database.DB.Model(&models.Knowledge{}).Where("id = ?", k.ID).Updates(map[string]any{
				"image_path": k.ImagePath,
				"image_mime": k.ImageMime,
			})
			services.IndexKnowledge(&k)
		}
	}

	items := []models.Knowledge{k}
	attachKnowledgeImageURLs(c, aid, items)
	status := 201
	if merged {
		status = 200
	}
	c.JSON(status, gin.H{"data": items[0], "merged": merged})
}

func UpdateKnowledge(c *gin.Context) {
	aid := currentAgentID(c)
	var k models.Knowledge
	if database.DB.Where("agent_id = ?", aid).First(&k, c.Param("kid")).Error != nil {
		c.JSON(404, gin.H{"error": "Not found"})
		return
	}

	var question, answer, tags string
	var active *bool
	var priority *int
	removeImage := false

	contentType := c.ContentType()
	if strings.Contains(contentType, "multipart/form-data") {
		question = strings.TrimSpace(c.PostForm("question"))
		answer = strings.TrimSpace(c.PostForm("answer"))
		tags = strings.TrimSpace(c.PostForm("tags"))
		if actStr := c.PostForm("active"); actStr != "" {
			b := actStr == "true" || actStr == "1"
			active = &b
		}
		if prioStr := c.PostForm("priority"); prioStr != "" {
			if p, err := strconv.Atoi(prioStr); err == nil {
				priority = &p
			}
		}
		if rmStr := c.PostForm("remove_image"); rmStr == "true" || rmStr == "1" {
			removeImage = true
		}
	} else {
		var req struct {
			Question    string `json:"question"`
			Answer      string `json:"answer"`
			Tags        string `json:"tags"`
			Active      *bool  `json:"active"`
			Priority    *int   `json:"priority"`
			RemoveImage bool   `json:"remove_image"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		question = strings.TrimSpace(req.Question)
		answer = strings.TrimSpace(req.Answer)
		tags = strings.TrimSpace(req.Tags)
		active = req.Active
		priority = req.Priority
		removeImage = req.RemoveImage
	}

	if question == "" || answer == "" {
		c.JSON(400, gin.H{"error": "Pertanyaan dan jawaban wajib diisi"})
		return
	}

	var siblings []models.Knowledge
	database.DB.Where("agent_id = ? AND id <> ?", k.AgentID, k.ID).Find(&siblings)
	for _, sibling := range siblings {
		if knowledgeLooksDuplicate(sibling.Question, sibling.Answer, question, answer) {
			c.JSON(409, gin.H{"error": "FAQ serupa sudah ada. Edit FAQ yang sudah ada agar pengetahuan tidak duplikat."})
			return
		}
	}

	k.Question = question
	k.Answer = answer
	k.Tags = normalizeKnowledgeTags(tags, "manual")
	k.Source = "manual"
	if active != nil {
		k.Active = *active
	}
	if priority != nil {
		if *priority < -100 || *priority > 100 {
			c.JSON(400, gin.H{"error": "Prioritas harus antara -100 sampai 100"})
			return
		}
		k.Priority = *priority
	}
	now := time.Now()
	k.VerifiedAt = &now

	if removeImage {
		if k.ImagePath != "" {
			_ = os.Remove(k.ImagePath)
			k.ImagePath = ""
			k.ImageMime = ""
		}
	}

	if strings.Contains(contentType, "multipart/form-data") {
		if err := saveKnowledgeImage(c, aid, &k); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	}

	if err := database.DB.Save(&k).Error; err != nil {
		c.JSON(500, gin.H{"error": "FAQ belum bisa diperbarui"})
		return
	}
	if k.Active {
		services.IndexKnowledge(&k)
	} else {
		services.InvalidateKB(k.AgentID)
	}

	items := []models.Knowledge{k}
	attachKnowledgeImageURLs(c, aid, items)
	c.JSON(200, gin.H{"data": items[0]})
}

func DeleteKnowledge(c *gin.Context) {
	aid := currentAgentID(c)
	var k models.Knowledge
	if database.DB.Where("agent_id = ? AND id = ?", aid, c.Param("kid")).First(&k).Error == nil {
		if k.ImagePath != "" {
			_ = os.Remove(k.ImagePath)
		}
	}
	result := database.DB.Where("agent_id = ?", aid).Delete(&models.Knowledge{}, c.Param("kid"))
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "FAQ belum bisa dihapus"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "FAQ tidak ditemukan"})
		return
	}
	services.InvalidateKB(aid)
	c.JSON(200, gin.H{"message": "FAQ dihapus", "deleted": 1})
}

func DeleteAllKnowledge(c *gin.Context) {
	aid := currentAgentID(c)
	var items []models.Knowledge
	database.DB.Where("agent_id = ? AND image_path <> ''", aid).Find(&items)
	for _, item := range items {
		if item.ImagePath != "" {
			_ = os.Remove(item.ImagePath)
		}
	}

	result := database.DB.Where("agent_id = ?", aid).Delete(&models.Knowledge{})
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "Pengetahuan belum bisa dihapus"})
		return
	}
	services.InvalidateKB(aid)
	c.JSON(200, gin.H{"message": "Pengetahuan dihapus", "count": result.RowsAffected})
}
