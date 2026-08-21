package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var csUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)

type teamUserRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Active   *bool  `json:"active"`
	AgentIDs []uint `json:"agent_ids"`
}

func validateAssignedAgents(tenantID uint, agentIDs []uint) ([]uint, error) {
	seen := make(map[uint]struct{}, len(agentIDs))
	unique := make([]uint, 0, len(agentIDs))
	for _, id := range agentIDs {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("pilih minimal satu nomor WhatsApp")
	}
	// Ambil hanya agent yang benar-benar ada untuk tenant ini. Relasi CS→agent
	// bisa yatim bila sebuah nomor dihapus; DeleteAgent sudah membersihkannya
	// ke depan, tapi di sini kita tetap buang id tidak valid secara diam-diam
	// (alih-alih menolak total) agar penyimpanan akun CS berhasil dan assignment
	// yatim lama otomatis terkikis saat admin menyimpan.
	var validIDs []uint
	if err := database.DB.Model(&models.Agent{}).
		Where("tenant_id = ? AND id IN ?", tenantID, unique).
		Pluck("id", &validIDs).Error; err != nil {
		return nil, err
	}
	if len(validIDs) == 0 {
		return nil, errors.New("pilih minimal satu nomor WhatsApp")
	}
	sort.Slice(validIDs, func(i, j int) bool { return validIDs[i] < validIDs[j] })
	return validIDs, nil
}

func replaceUserAssignments(tx *gorm.DB, tenantID, userID uint, agentIDs []uint) error {
	if err := tx.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Delete(&models.UserAgentAssignment{}).Error; err != nil {
		return err
	}
	if len(agentIDs) == 0 {
		return nil
	}
	rows := make([]models.UserAgentAssignment, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		rows = append(rows, models.UserAgentAssignment{TenantID: tenantID, UserID: userID, AgentID: agentID})
	}
	return tx.Create(&rows).Error
}

func ListTeamUsers(c *gin.Context) {
	tenantID := currentTenantID(c)
	var users []models.User
	database.DB.Where("tenant_id = ? AND is_super_admin = ?", tenantID, false).
		Order("role asc, name asc, id asc").Find(&users)
	var assignments []models.UserAgentAssignment
	database.DB.Where("tenant_id = ?", tenantID).Find(&assignments)
	byUser := make(map[uint][]uint)
	for _, assignment := range assignments {
		byUser[assignment.UserID] = append(byUser[assignment.UserID], assignment.AgentID)
	}
	out := make([]gin.H, 0, len(users))
	for _, user := range users {
		out = append(out, gin.H{
			"id": user.ID, "name": user.Name, "username": user.Username,
			"role": user.Role, "active": user.Active, "agent_ids": byUser[user.ID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func CreateTeamUser(c *gin.Context) {
	tenantID := currentTenantID(c)
	var req teamUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format data tidak valid"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama CS wajib diisi"})
		return
	}
	if !csUsernamePattern.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username minimal 3 karakter dan hanya boleh berisi huruf, angka, titik, garis bawah, atau strip"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password minimal 8 karakter"})
		return
	}
	agentIDs, err := validateAssignedAgents(tenantID, req.AgentIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var existing int64
	database.DB.Model(&models.User{}).Where("LOWER(username) = LOWER(?)", req.Username).Count(&existing)
	if existing > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Username sudah digunakan"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password belum dapat diamankan"})
		return
	}
	user := models.User{
		Name: req.Name, Username: req.Username, Password: string(hash), Role: "cs",
		TenantID: &tenantID, EmailVerified: true, Active: true,
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return replaceUserAssignments(tx, tenantID, user.ID, agentIDs)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Akun CS belum dapat dibuat"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Akun CS dibuat", "data": gin.H{
		"id": user.ID, "name": user.Name, "username": user.Username,
		"role": user.Role, "active": user.Active, "agent_ids": agentIDs,
	}})
}

func UpdateTeamUser(c *gin.Context) {
	tenantID := currentTenantID(c)
	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ? AND role = ? AND is_super_admin = ?", c.Param("uid"), tenantID, "cs", false).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Akun CS tidak ditemukan"})
		return
	}
	var req teamUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format data tidak valid"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	if req.Name == "" || !csUsernamePattern.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama dan username CS tidak valid"})
		return
	}
	if req.Password != "" && len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password baru minimal 8 karakter"})
		return
	}
	agentIDs, err := validateAssignedAgents(tenantID, req.AgentIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var duplicate int64
	database.DB.Model(&models.User{}).
		Where("LOWER(username) = LOWER(?) AND id <> ?", req.Username, user.ID).Count(&duplicate)
	if duplicate > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Username sudah digunakan"})
		return
	}
	user.Name = req.Name
	user.Username = req.Username
	if req.Active != nil {
		user.Active = *req.Active
	}
	if req.Password != "" {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Password belum dapat diamankan"})
			return
		}
		user.Password = string(hash)
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		return replaceUserAssignments(tx, tenantID, user.ID, agentIDs)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Akun CS belum dapat diperbarui"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Akun CS diperbarui"})
}

func DeleteTeamUser(c *gin.Context) {
	tenantID := currentTenantID(c)
	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ? AND role = ? AND is_super_admin = ?", c.Param("uid"), tenantID, "cs", false).
		First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Akun CS tidak ditemukan"})
		return
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("active", false).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND user_id = ?", tenantID, user.ID).
			Delete(&models.UserAgentAssignment{}).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Akun CS belum dapat dinonaktifkan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Akun CS dinonaktifkan"})
}

func accessibleAgents(c *gin.Context) []models.Agent {
	var agents []models.Agent
	query := database.DB.Model(&models.Agent{}).Where("agents.tenant_id = ?", currentTenantID(c))
	if !isTenantAdmin(c) {
		query = query.Joins("JOIN user_agent_assignments uaa ON uaa.agent_id = agents.id").
			Where("uaa.tenant_id = ? AND uaa.user_id = ?", currentTenantID(c), currentUserID(c))
	}
	query.Order("agents.id asc").Find(&agents)
	return agents
}

// AgentUnreadSummary memberi satu pandangan semua nomor tanpa membuka device satu per satu.
func AgentUnreadSummary(c *gin.Context) {
	agents := accessibleAgents(c)
	agentIDs := make([]uint, 0, len(agents))
	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)
	}
	type pair struct {
		AgentID uint
		Sender  string
	}
	whatsappUnread := map[pair]int{}
	whatsappSynced := map[pair]bool{}
	lastActivity := map[uint]time.Time{}
	handoffs := map[uint]int64{}
	if len(agentIDs) > 0 {
		var states []models.InboxReadState
		database.DB.Select("agent_id", "sender", "last_msg_at", "whats_app_unread_count", "whats_app_synced").
			Where("agent_id IN ?", agentIDs).Find(&states)
		for _, state := range states {
			key := pair{AgentID: state.AgentID, Sender: state.Sender}
			whatsappUnread[key] = state.WhatsAppUnreadCount
			whatsappSynced[key] = state.WhatsAppSynced
			if state.LastMsgAt != nil && state.LastMsgAt.After(lastActivity[state.AgentID]) {
				lastActivity[state.AgentID] = *state.LastMsgAt
			}
		}
		type handoffRow struct {
			AgentID uint
			Count   int64
		}
		var handoffRows []handoffRow
		database.DB.Model(&models.Handoff{}).Select("agent_id, COUNT(*) AS count").
			Where("agent_id IN ?", agentIDs).Group("agent_id").Scan(&handoffRows)
		for _, row := range handoffRows {
			handoffs[row.AgentID] = row.Count
		}
	}
	totalUnreadByAgent := map[uint]int{}
	for key := range whatsappUnread {
		if whatsappSynced[key] {
			totalUnreadByAgent[key.AgentID] += whatsappUnread[key]
		}
	}
	out := make([]gin.H, 0, len(agents))
	total := 0
	for _, agent := range agents {
		unread := totalUnreadByAgent[agent.ID]
		total += unread
		out = append(out, gin.H{
			"agent_id": agent.ID, "name": agent.Name, "number": agent.Number,
			"status": services.WA(agent.ID).GetStatus(), "unread_count": unread,
			"handoff_count": handoffs[agent.ID], "last_activity_at": lastActivity[agent.ID],
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out, "total_unread": total})
}

func logCSActivity(c *gin.Context, agentID uint, sender, action, detail string) {
	userID := currentUserID(c)
	if userID == 0 || agentID == 0 {
		return
	}
	detail = strings.TrimSpace(detail)
	if len([]rune(detail)) > 500 {
		detail = string([]rune(detail)[:500])
	}
	_ = database.DB.Create(&models.CSActivityLog{
		TenantID: currentTenantID(c), UserID: userID, AgentID: agentID,
		Sender: services.NormalizePhone(sender), Action: action, Detail: detail,
	}).Error
}

func ListCSActivity(c *gin.Context) {
	tenantID := currentTenantID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := database.DB.Model(&models.CSActivityLog{}).Where("tenant_id = ?", tenantID)
	if agentID := strings.TrimSpace(c.Query("agent_id")); agentID != "" {
		query = query.Where("agent_id = ?", agentID)
	}
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghitung aktivitas CS"})
		return
	}

	var logs []models.CSActivityLog
	if err := query.Order("created_at desc, id desc").
		Offset((page - 1) * limit).Limit(limit).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat aktivitas CS"})
		return
	}

	userIDs := make([]uint, 0, len(logs))
	agentIDs := make([]uint, 0, len(logs))
	for _, log := range logs {
		userIDs = append(userIDs, log.UserID)
		agentIDs = append(agentIDs, log.AgentID)
	}
	var users []models.User
	var agents []models.Agent
	if len(userIDs) > 0 {
		database.DB.Select("id", "name", "username").Where("id IN ?", userIDs).Find(&users)
	}
	if len(agentIDs) > 0 {
		database.DB.Select("id", "name", "number").Where("id IN ?", agentIDs).Find(&agents)
	}
	userNames := make(map[uint]string)
	agentNames := make(map[uint]string)
	for _, user := range users {
		name := user.Name
		if name == "" {
			name = user.Username
		}
		userNames[user.ID] = name
	}
	for _, agent := range agents {
		agentNames[agent.ID] = agent.Name
	}
	out := make([]gin.H, 0, len(logs))
	for _, entry := range logs {
		out = append(out, gin.H{
			"id": entry.ID, "user_id": entry.UserID, "user_name": userNames[entry.UserID],
			"agent_id": entry.AgentID, "agent_name": agentNames[entry.AgentID],
			"sender": entry.Sender, "action": entry.Action, "detail": entry.Detail,
			"created_at": entry.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out, "total": total, "page": page, "limit": limit,
	})
}
