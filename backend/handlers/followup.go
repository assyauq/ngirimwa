package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"kirimwa/backend/database"
	"kirimwa/backend/models"
	"kirimwa/backend/services"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
	"gorm.io/gorm"
)

type followUpStepReq struct {
	DelayHours    int    `json:"delay_hours"`
	Message       string `json:"message"`
	AiGenerated   bool   `json:"ai_generated"`
	AiInstruction string `json:"ai_instruction"`
}

// stepsWithCounts merangkai respons follow-up lengkap dengan langkah & ringkasan pendaftaran.
func followUpResponse(fu models.FollowUp) gin.H {
	var steps []models.FollowUpStep
	database.DB.Where("follow_up_id = ?", fu.ID).Order("step_order asc, id asc").Find(&steps)
	var active, completed, stopped, due int64
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "active").Count(&active)
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "completed").Count(&completed)
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "stopped").Count(&stopped)

	var activeEnrollments []models.FollowUpEnrollment
	database.DB.Select("enrolled_at", "next_step").
		Where("follow_up_id = ? AND status = ?", fu.ID, "active").Find(&activeEnrollments)
	var nextSendAt, lastSentAt *time.Time
	now := time.Now()
	for _, enrollment := range activeEnrollments {
		if enrollment.NextStep < len(steps) {
			dueAt := enrollment.EnrolledAt.Add(time.Duration(steps[enrollment.NextStep].DelayHours) * time.Hour)
			if !dueAt.After(now) {
				due++
			}
			if nextSendAt == nil || dueAt.Before(*nextSendAt) {
				value := dueAt
				nextSendAt = &value
			}
		}
	}
	var latest models.FollowUpEnrollment
	if database.DB.Select("last_sent_at").Where("follow_up_id = ? AND last_sent_at IS NOT NULL", fu.ID).
		Order("last_sent_at desc").First(&latest).Error == nil {
		lastSentAt = latest.LastSentAt
	}
	return gin.H{
		"id": fu.ID, "name": fu.Name, "enabled": fu.Enabled, "stop_on_reply": fu.StopOnReply,
		"steps": steps, "next_send_at": nextSendAt, "last_sent_at": lastSentAt,
		"counts": gin.H{"active": active, "completed": completed, "stopped": stopped, "due": due},
	}
}

func ListFollowUps(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fus []models.FollowUp
	database.DB.Where("agent_id = ?", id).Order("id desc").Find(&fus)
	out := make([]gin.H, 0, len(fus))
	for _, fu := range fus {
		out = append(out, followUpResponse(fu))
	}
	c.JSON(200, gin.H{"data": out})
}

func normalizeFollowUpSteps(steps []followUpStepReq) ([]followUpStepReq, error) {
	normalized := make([]followUpStepReq, 0, len(steps))
	previousDelay := -1
	for _, raw := range steps {
		step := raw
		step.Message = strings.TrimSpace(step.Message)
		step.AiInstruction = strings.TrimSpace(step.AiInstruction)
		if step.AiGenerated {
			if step.AiInstruction == "" {
				step.AiInstruction = step.Message
			}
			step.Message = step.AiInstruction
		} else {
			step.AiInstruction = ""
		}
		if step.Message == "" {
			continue
		}
		if step.DelayHours < 0 {
			return nil, fmt.Errorf("jeda tidak boleh negatif")
		}
		if previousDelay > step.DelayHours {
			return nil, fmt.Errorf("waktu langkah harus berurutan dari paling awal")
		}
		previousDelay = step.DelayHours
		normalized = append(normalized, step)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("minimal satu langkah dengan pesan")
	}
	return normalized, nil
}

func saveSteps(tx *gorm.DB, followUpID uint, steps []followUpStepReq) error {
	if err := tx.Where("follow_up_id = ?", followUpID).Delete(&models.FollowUpStep{}).Error; err != nil {
		return err
	}
	for i, s := range steps {
		if err := tx.Create(&models.FollowUpStep{
			FollowUpID: followUpID, StepOrder: i, DelayHours: s.DelayHours,
			Message: s.Message, AiGenerated: s.AiGenerated, AiInstruction: s.AiInstruction,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func CreateFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)
	var req struct {
		Name        string            `json:"name"`
		StopOnReply *bool             `json:"stop_on_reply"`
		Steps       []followUpStepReq `json:"steps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(400, gin.H{"error": "Nama urutan wajib diisi"})
		return
	}
	steps, err := normalizeFollowUpSteps(req.Steps)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	stop := true
	if req.StopOnReply != nil {
		stop = *req.StopOnReply
	}
	fu := models.FollowUp{TenantID: tid, AgentID: id, Name: req.Name, Enabled: true, StopOnReply: stop}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&fu).Error; err != nil {
			return err
		}
		return saveSteps(tx, fu.ID, steps)
	}); err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat follow-up"})
		return
	}
	c.JSON(201, gin.H{"data": followUpResponse(fu)})
}

func UpdateFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	var req struct {
		Name        *string            `json:"name"`
		Enabled     *bool              `json:"enabled"`
		StopOnReply *bool              `json:"stop_on_reply"`
		Steps       *[]followUpStepReq `json:"steps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(400, gin.H{"error": "Nama urutan wajib diisi"})
			return
		}
		fu.Name = name
	}
	if req.Enabled != nil {
		fu.Enabled = *req.Enabled
	}
	if req.StopOnReply != nil {
		fu.StopOnReply = *req.StopOnReply
	}
	var steps []followUpStepReq
	if req.Steps != nil {
		var err error
		steps, err = normalizeFollowUpSteps(*req.Steps)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&fu).Error; err != nil {
			return err
		}
		if req.Steps != nil {
			return saveSteps(tx, fu.ID, steps)
		}
		return nil
	}); err != nil {
		c.JSON(500, gin.H{"error": "Follow-up belum bisa diperbarui"})
		return
	}
	c.JSON(200, gin.H{"data": followUpResponse(fu)})
}

func DeleteFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	if !fu.Enabled {
		c.JSON(409, gin.H{"error": "Aktifkan urutan sebelum menambahkan kontak"})
		return
	}
	_ = database.DB.Where("follow_up_id = ?", fu.ID).Delete(&models.FollowUpStep{}).Error
	_ = database.DB.Where("follow_up_id = ?", fu.ID).Delete(&models.FollowUpEnrollment{}).Error
	_ = database.DB.Delete(&fu).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}

// EnrollFollowUp mendaftarkan kontak ke sebuah urutan. Lewati nomor yang sudah opt-out
// atau sudah aktif di urutan ini. Kontak yang dulu pernah ikut & sudah selesai bisa diikutkan lagi.
func EnrollFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	var req struct {
		Recipients []scheduleRecipient `json:"recipients"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Recipients) == 0 {
		c.JSON(400, gin.H{"error": "Penerima wajib diisi"})
		return
	}
	optedOut := optedOutSet(id)
	now := time.Now()
	var added, skipped, invalid, duplicate, optedOutCount, alreadyActive, failed int
	seen := map[string]bool{}
	for _, r := range req.Recipients {
		num := services.NormalizePhone(r.Number)
		if num == "" {
			invalid++
			skipped++
			continue
		}
		if seen[num] {
			duplicate++
			skipped++
			continue
		}
		seen[num] = true
		if optedOut[num] {
			optedOutCount++
			skipped++
			continue
		}
		// Sudah aktif di urutan ini? lewati.
		var existing models.FollowUpEnrollment
		if database.DB.Where("follow_up_id = ? AND number = ?", fu.ID, num).First(&existing).Error == nil {
			if existing.Status == "active" {
				alreadyActive++
				skipped++
				continue
			}
			// daftar ulang: reset enrollment lama.
			updates := map[string]any{
				"enrolled_at": now, "next_step": 0,
				"status": "active", "stopped_reason": "", "last_sent_at": nil,
			}
			if strings.TrimSpace(r.Name) != "" {
				updates["name"] = strings.TrimSpace(r.Name)
			}
			if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
				failed++
				skipped++
				continue
			}
			added++
			continue
		}
		if err := database.DB.Create(&models.FollowUpEnrollment{
			FollowUpID: fu.ID, Number: num, TenantID: tid, AgentID: id,
			Name: strings.TrimSpace(r.Name), EnrolledAt: now, NextStep: 0, Status: "active",
		}).Error; err != nil {
			failed++
			skipped++
			continue
		}
		added++
	}
	c.JSON(200, gin.H{
		"added": added, "skipped": skipped,
		"details": gin.H{
			"invalid": invalid, "duplicate": duplicate, "opted_out": optedOutCount,
			"already_active": alreadyActive, "failed": failed,
		},
	})
}

// ---- Worker ----

var followUpSweeping sync.Mutex

// processDueFollowUps mengirim langkah follow-up yang jatuh tempo. Dipanggil tiap menit
// dari scheduler. Dijaga mutex agar tidak ada dua sweep berbarengan (cegah dobel kirim).
func processDueFollowUps() {
	if !followUpSweeping.TryLock() {
		return
	}
	defer followUpSweeping.Unlock()

	var enrolls []models.FollowUpEnrollment
	database.DB.Where("status = ?", "active").Order("id asc").Find(&enrolls)

	const maxPerSweep = 40
	sent := 0
	for _, e := range enrolls {
		if sent >= maxPerSweep {
			break
		}
		var fu models.FollowUp
		if database.DB.First(&fu, e.FollowUpID).Error != nil || !fu.Enabled {
			continue
		}
		var steps []models.FollowUpStep
		database.DB.Where("follow_up_id = ?", fu.ID).Order("step_order asc, id asc").Find(&steps)
		if e.NextStep >= len(steps) {
			database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", e.ID).Update("status", "completed")
			continue
		}
		step := steps[e.NextStep]
		if time.Now().Before(e.EnrolledAt.Add(time.Duration(step.DelayHours) * time.Hour)) {
			continue // belum waktunya
		}
		// Opt-out -> stop.
		if followUpOptedOut(e.AgentID, e.Number) {
			stopEnrollment(e.ID, "opt-out")
			continue
		}
		// Kontak sudah membalas setelah didaftarkan -> stop (kalau diaktifkan).
		if fu.StopOnReply && repliedSince(e.AgentID, e.Number, e.EnrolledAt) {
			stopEnrollment(e.ID, "dibalas")
			continue
		}
		if !services.WA(e.AgentID).IsConnected() {
			continue // tunda, coba menit berikutnya
		}

		msg := personalize(spinText(step.Message), e.Name)

		// AI-generated: gunakan instruksi sebagai prompt, personalisasi dengan konteks.
		if step.AiGenerated {
			if aiMsg, ok := generateAIFollowUpMsg(e.AgentID, e.Number, e.Name, step); ok {
				msg = aiMsg
			} else {
				// Jangan pernah mengirim instruksi internal AI sebagai pesan literal.
				msg = fallbackAIFollowUpMessage(e.Name)
			}
		}
		msg = strings.TrimSpace(msg)
		if msg == "" {
			log.Printf("FollowUp %d langkah %d dilewati: pesan kosong", fu.ID, e.NextStep)
			continue
		}

		if err := services.WA(e.AgentID).SendText(e.Number, msg); err != nil {
			continue // gagal kirim -> jangan maju, coba lagi nanti
		}
		logTurn(e.AgentID, e.Number, "", msg, true, "", "")
		sent++

		now := time.Now()
		nextStep := e.NextStep + 1
		status := "active"
		if nextStep >= len(steps) {
			status = "completed"
		}
		if err := database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", e.ID).
			Updates(map[string]any{"next_step": nextStep, "status": status, "last_sent_at": &now}).Error; err != nil {
			log.Printf("FollowUp gagal memperbarui enrollment %d setelah terkirim: %v", e.ID, err)
		}

		// Jeda kecil antar kirim agar lembut (anti-banned).
		time.Sleep(6 * time.Second)
	}
}

func followUpOptedOut(agentID uint, number string) bool {
	var n int64
	database.DB.Model(&models.OptOut{}).Where("agent_id = ? AND sender = ?", agentID, number).Count(&n)
	return n > 0
}

// repliedSince = true bila ada pesan MASUK dari kontak setelah waktu tertentu.
func repliedSince(agentID uint, number string, since time.Time) bool {
	var n int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND (message <> '' OR media_type <> '') AND created_at > ?", agentID, number, since).
		Count(&n)
	return n > 0
}

// stopActiveFollowUps menghentikan enrollment segera ketika pesan masuk diterima,
// sehingga status dashboard akurat tanpa menunggu langkah berikutnya jatuh tempo.
func stopActiveFollowUps(agentID uint, number, reason string) {
	number = services.NormalizePhone(number)
	if agentID == 0 || number == "" {
		return
	}
	followUps := database.DB.Model(&models.FollowUp{}).Select("id").Where("agent_id = ?", agentID)
	if reason != "opt-out" {
		followUps = followUps.Where("stop_on_reply = ?", true)
	}
	if err := database.DB.Model(&models.FollowUpEnrollment{}).
		Where("agent_id = ? AND number = ? AND status = ? AND follow_up_id IN (?)", agentID, number, "active", followUps).
		Updates(map[string]any{"status": "stopped", "stopped_reason": reason}).Error; err != nil {
		log.Printf("FollowUp gagal menghentikan enrollment %s agent %d: %v", number, agentID, err)
	}
}

func stopEnrollment(enrollID uint, reason string) {
	database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", enrollID).
		Updates(map[string]any{"status": "stopped", "stopped_reason": reason})
}

func fallbackAIFollowUpMessage(name string) string {
	greeting := "Halo Kak"
	if cleanName := strings.TrimSpace(name); cleanName != "" {
		greeting = "Halo " + cleanName
	}
	return greeting + ", kami ingin menindaklanjuti percakapan sebelumnya. Apakah masih ada yang bisa kami bantu?"
}

// generateAIFollowUpMsg membuat pesan follow-up personal pakai AI.
// Menggabungkan instruksi user, riwayat chat, dan persona agent.
func generateAIFollowUpMsg(agentID uint, number, name string, step models.FollowUpStep) (string, bool) {
	instruction := strings.TrimSpace(step.AiInstruction)
	if instruction == "" {
		instruction = strings.TrimSpace(step.Message)
	}
	if instruction == "" {
		return "", false
	}

	// Ambil riwayat chat terakhir (maks 5 pesan masuk terakhir).
	var history []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND message <> ''", agentID, number).
		Order("created_at desc").Limit(5).Find(&history)
	// Balik urutan jadi kronologis.
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	// Ambil persona agent untuk konteks bisnis.
	var agent models.Agent
	database.DB.Select("system_prompt, name").First(&agent, agentID)

	var sb strings.Builder
	sb.WriteString("Kamu adalah asisten WhatsApp bisnis yang bertugas mengirim pesan follow-up ke pelanggan. ")
	sb.WriteString("Tulis pesan WhatsApp yang personal, ramah, dan natural — seperti manusia, bukan template. ")
	sb.WriteString("Sapa dengan nama bila wajar. Ringkas, 1-3 kalimat. JANGAN mengarang detail spesifik (harga, promo, stok) yang tidak disebut di instruksi.\n")
	sb.WriteString("PENTING: Output HANYA teks pesan WhatsApp yang akan dikirim. JANGAN sertakan catatan, penjelasan, analisis, tanda kutip, prefix \"Pesan:\", asterisk, atau format markdown apapun. Langsung teks mentah saja.\n")
	if strings.TrimSpace(agent.SystemPrompt) != "" {
		sb.WriteString("\nKONTEKS BISNIS:\n" + strings.TrimSpace(agent.SystemPrompt) + "\n")
	}
	sb.WriteString("\nPENERIMA: " + name)
	if number != "" {
		sb.WriteString(" (" + number + ")")
	}
	sb.WriteString("\n\nINSTRUKSI FOLLOW-UP:\n" + instruction)

	if len(history) > 0 {
		sb.WriteString("\n\nRIWAYAT CHAT TERAKHIR:\n")
		for _, h := range history {
			if h.Message != "" {
				sb.WriteString("- Pelanggan: " + h.Message + "\n")
			}
			if h.Reply != "" {
				sb.WriteString("- CS: " + h.Reply + "\n")
			}
		}
	}

	prompt := sb.String()
	log.Printf("FollowUp AI: generating for %s (agent=%d)", number, agentID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := services.CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}, 500, 0.7)
	if err != nil {
		log.Printf("FollowUp AI: gagal generate untuk %s: %v", number, err)
		return "", false
	}
	if len(resp.Choices) == 0 {
		return "", false
	}
	reply := strings.TrimSpace(resp.Choices[0].Message.Content)
	if reply == "" {
		return "", false
	}
	// Sanitasi: hapus prefix umum & bocoran internal AI.
	reply = strings.TrimPrefix(reply, "Pesan:")
	reply = strings.TrimPrefix(reply, "\"")
	reply = strings.TrimSuffix(reply, "\"")
	// Potong di section "Catatan:" atau "*Catatan:" kalau AI masih bandel.
	if idx := strings.Index(reply, "\nCatatan:"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	if idx := strings.Index(reply, "\n*Catatan:"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	if idx := strings.Index(reply, "\n**Catatan:**"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", false
	}
	log.Printf("FollowUp AI: generated untuk %s (%d chars)", number, len(reply))
	return reply, true
}
