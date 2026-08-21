package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"wa-assistant/backend/config"
	"wa-assistant/backend/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var DB *gorm.DB

func fatalDatabaseStartup(message string, err error) {
	detail := fmt.Sprintf("%s: %v", message, err)
	_ = os.MkdirAll(".tmp", 0o755)
	_ = os.WriteFile(
		".tmp/backend-startup-error.log",
		[]byte(time.Now().Format(time.RFC3339)+" "+detail+"\n"),
		0o600,
	)
	log.Fatal(detail)
}

func Init() {
	host := config.Env("DB_HOST", "localhost")
	port := config.Env("DB_PORT", "3306")
	user := config.Env("DB_USER", "root")
	pass := config.Env("DB_PASS", "")
	name := config.Env("DB_NAME", "wa_assistant")
	// Validasi nama DB (hanya huruf/angka/underscore) sebelum dipakai di query CREATE DATABASE.
	if !validDBName(name) {
		log.Printf("DB_NAME tidak valid (%q) — pakai default 'wa_assistant'", name)
		name = "wa_assistant"
	}

	// Buat database-nya kalau belum ada (connect tanpa nama DB dulu).
	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=True&loc=Local", user, pass, host, port)
	if rootDB, err := gorm.Open(mysql.Open(rootDSN), &gorm.Config{}); err == nil {
		rootDB.Exec("CREATE DATABASE IF NOT EXISTS `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
		if sqlDB, e := rootDB.DB(); e == nil {
			sqlDB.Close()
		}
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, pass, host, port, name)
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		fatalDatabaseStartup("DB error (MySQL)", err)
	}

	// Batasi connection pool agar lonjakan traffic tidak menghabiskan koneksi MySQL
	// (penting di VPS yang dipakai bersama situs lain). Semua bisa diatur via env.
	if sqlDB, e := DB.DB(); e == nil {
		sqlDB.SetMaxOpenConns(config.EnvInt("DB_MAX_OPEN_CONNS", 25))
		sqlDB.SetMaxIdleConns(config.EnvInt("DB_MAX_IDLE_CONNS", 5))
		sqlDB.SetConnMaxLifetime(time.Duration(config.EnvInt("DB_CONN_MAX_LIFETIME_MIN", 30)) * time.Minute)
	}

	if err := preflightCanonicalChatSchema(); err != nil {
		fatalDatabaseStartup("Database tidak aman sebelum migrasi Inbox", err)
	}
	if err := DB.AutoMigrate(
		&models.User{}, &models.UserAgentAssignment{}, &models.CSActivityLog{}, &models.LoginThrottle{}, &models.Agent{}, &models.ChatHistory{}, &models.InboxReadState{}, &models.Setting{},
		&models.AITurn{},
		&models.Knowledge{}, &models.Handoff{}, &models.Contact{}, &models.ConversationMemory{},
		&models.CrawlJob{}, &models.CrawlPage{},
		&models.Tenant{},
		&models.Broadcast{}, &models.BroadcastRecipient{}, &models.OptOut{}, &models.ContactConsent{},
		&models.ScheduledMessage{}, &models.ScheduledStatus{}, &models.Label{}, &models.ChatLabel{}, &models.AutoReply{},
		&models.Flow{}, &models.FlowSession{}, &models.OTPCode{},
		&models.Template{},
		&models.FollowUp{}, &models.FollowUpStep{}, &models.FollowUpEnrollment{},
		&models.Product{}, &models.ProductCheckoutSession{}, &models.ProductOrder{},
		&models.AIForm{}, &models.AIFormSession{}, &models.AIFormSubmission{},
		&models.AppSetting{},
		&models.ClosingForm{}, &models.ClosingRecord{},
		&models.ShippingCity{},
		&models.GroupGuardConfig{}, &models.GroupModerationLog{},
		&models.MetaConversionEvent{},
	); err != nil {
		fatalDatabaseStartup("Migrasi database gagal", err)
	}

	backfillKnowledgeCharCount()
	backfillHistoricalDeliveryStatus()
	backfillInboxLastMsgAt()
	normalizeSenderFields()
	if err := ensureCanonicalChatMessageIDs(); err != nil {
		fatalDatabaseStartup("Database tidak aman untuk sinkronisasi Inbox", err)
	}
	_ = os.Remove(".tmp/backend-startup-error.log")
	recoverStuckCrawlJobs()
	seedSuperAdmin()
	seedDefaultTenant()

	log.Println("Database ready")
}

// backfillInboxLastMsgAt mengisi last_msg_at dari MAX(created_at) chat_histories
// agar daftar Inbox terurut wajar sebelum event WA berikutnya memajukan nilainya.
func backfillInboxLastMsgAt() {
	type row struct {
		AgentID uint
		Sender  string
		LastAt  time.Time
	}
	var rows []row
	if err := DB.Raw(`
		SELECT agent_id, sender, MAX(created_at) AS last_at
		FROM chat_histories
		WHERE sender <> ''
		GROUP BY agent_id, sender
	`).Scan(&rows).Error; err != nil {
		log.Printf("Backfill last_msg_at gagal: %v", err)
		return
	}
	filled := 0
	for _, r := range rows {
		if r.LastAt.IsZero() {
			continue
		}
		state := models.InboxReadState{AgentID: r.AgentID, Sender: r.Sender}
		if err := DB.Where("agent_id = ? AND sender = ?", r.AgentID, r.Sender).
			FirstOrCreate(&state).Error; err != nil {
			continue
		}
		if state.LastMsgAt != nil && !state.LastMsgAt.Before(r.LastAt) {
			continue
		}
		if err := DB.Model(&models.InboxReadState{}).
			Where("agent_id = ? AND sender = ?", r.AgentID, r.Sender).
			Update("last_msg_at", r.LastAt).Error; err == nil {
			filled++
		}
	}
	if filled > 0 {
		log.Printf("Backfill last_msg_at untuk %d percakapan", filled)
	}
}

// backfillHistoricalDeliveryStatus memanfaatkan bukti lokal yang masih tersedia:
// jika pelanggan mengirim pesan lagi SETELAH pesan keluar, percakapan pasti sudah
// dibuka kembali. Receipt lama tidak dapat diminta ulang dari WhatsApp, jadi status
// ini dibedakan sebagai read_inferred sampai receipt read asli datang.
func backfillHistoricalDeliveryStatus() {
	type latestIncoming struct {
		AgentID uint
		Sender  string
		LastAt  time.Time
	}
	var rows []latestIncoming
	if err := DB.Model(&models.ChatHistory{}).
		Select("agent_id, sender, MAX(created_at) AS last_at").
		Where("TRIM(COALESCE(message, '')) <> ''").
		Group("agent_id, sender").
		Scan(&rows).Error; err != nil {
		log.Printf("Backfill status pesan lama gagal membaca bukti interaksi: %v", err)
		return
	}

	var updated int64
	for _, row := range rows {
		result := DB.Model(&models.ChatHistory{}).
			Where(
				"agent_id = ? AND sender = ? AND created_at < ? AND wa_msg_id <> '' AND TRIM(COALESCE(reply, '')) <> '' AND delivery_status IN ?",
				row.AgentID, row.Sender, row.LastAt, []string{"", "sent", "delivered"},
			).
			Update("delivery_status", "read_inferred")
		if result.Error == nil {
			updated += result.RowsAffected
		}
	}
	if updated > 0 {
		log.Printf("Backfill status terbaca untuk %d pesan lama berdasarkan balasan pelanggan sesudahnya", updated)
	}
}

// normalizePhoneLocal menormalkan nomor telepon: hanya digit, 08→628, 8→628.
// Duplikasi dari services.NormalizePhone untuk menghindari circular import.
func normalizePhoneLocal(s string) string {
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
	case strings.HasPrefix(d, "8"):
		return "62" + d
	default:
		return d
	}
}

// normalizedSenderFieldValue hanya mengubah JID personal legacy menjadi nomor.
// JID grup adalah identitas thread kanonik dan tidak boleh kehilangan @g.us.
func normalizedSenderFieldValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(strings.ToLower(value), "@g.us") {
		return "", false
	}
	clean := strings.SplitN(value, "@", 2)[0]
	if clean == value || clean == "" {
		return "", false
	}
	normalized := normalizePhoneLocal(clean)
	return normalized, normalized != "" && normalized != value
}

// normalizeSenderFields memperbaiki sender personal yang mungkin tersimpan
// dengan format JID (@s.whatsapp.net) atau nomor yang belum ternormalisasi.
// JID grup sengaja dipertahankan utuh.
func normalizeSenderFields() {
	type senderPair struct{ Old, New string }
	var pairs []senderPair

	// Kumpulkan semua sender unik dari chat_histories + inbox_read_states +
	// handoffs + conversation_memories + ai_turns + contacts (number).
	seen := make(map[string]struct{})
	for _, table := range []string{
		"chat_histories", "inbox_read_states", "handoffs",
		"conversation_memories", "ai_turns", "follow_ups",
	} {
		var values []string
		DB.Table(table).Distinct("sender").Where("sender LIKE ?", "%@%").Pluck("sender", &values)
		for _, v := range values {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			if normalized, ok := normalizedSenderFieldValue(v); ok {
				pairs = append(pairs, senderPair{Old: v, New: normalized})
			}
		}
	}

	// Contacts pakai kolom 'number' bukan 'sender'.
	var contactNumbers []string
	DB.Table("contacts").Distinct("number").Where("number LIKE ?", "%@%").Pluck("number", &contactNumbers)
	for _, v := range contactNumbers {
		if normalized, ok := normalizedSenderFieldValue(v); ok {
			pairs = append(pairs, senderPair{Old: v, New: normalized})
		}
	}

	if len(pairs) == 0 {
		return
	}

	// Update semua tabel terkait.
	tables := map[string]string{
		"chat_histories":        "sender",
		"inbox_read_states":     "sender",
		"handoffs":              "sender",
		"conversation_memories": "sender",
		"ai_turns":              "sender",
		"follow_ups":            "sender",
		"contacts":              "number",
	}
	var total int64
	for _, pair := range pairs {
		for table, col := range tables {
			result := DB.Table(table).Where(col+" = ?", pair.Old).Update(col, pair.New)
			if result.Error == nil {
				total += result.RowsAffected
			}
		}
	}
	if total > 0 {
		log.Printf("Normalisasi sender: %d baris diperbaiki di %d sender unik", total, len(pairs))
	}
}

func newerTimePointer(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil || left.After(*right) {
		return left
	}
	return right
}

// MergeLegacyGroupThread menggabungkan thread grup lama yang kehilangan suffix
// @g.us ke JID grup kanonik. Fungsi ini idempotent dan dipanggil setelah daftar
// grup resmi diterima dari WhatsApp, sehingga angka panjang tidak ditebak sebagai
// grup tanpa bukti dari akun yang tertaut.
func MergeLegacyGroupThread(agentID uint, legacySender, canonicalJID string) (int64, error) {
	legacySender = strings.TrimSpace(legacySender)
	canonicalJID = strings.TrimSpace(canonicalJID)
	if agentID == 0 || legacySender == "" || canonicalJID == "" || legacySender == canonicalJID {
		return 0, nil
	}
	if !strings.HasSuffix(strings.ToLower(canonicalJID), "@g.us") {
		return 0, fmt.Errorf("JID grup kanonik tidak valid: %q", canonicalJID)
	}
	groupUser := strings.TrimSuffix(strings.ToLower(canonicalJID), "@g.us")
	if normalizePhoneLocal(groupUser) != legacySender {
		return 0, fmt.Errorf("alias %q tidak cocok dengan grup %q", legacySender, canonicalJID)
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	rollback := func(err error) (int64, error) {
		tx.Rollback()
		return 0, err
	}
	var affected int64

	var legacyState models.InboxReadState
	legacyErr := tx.Where("agent_id = ? AND sender = ?", agentID, legacySender).First(&legacyState).Error
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return rollback(legacyErr)
	}
	if legacyErr == nil {
		var canonicalState models.InboxReadState
		canonicalErr := tx.Where("agent_id = ? AND sender = ?", agentID, canonicalJID).First(&canonicalState).Error
		switch {
		case errors.Is(canonicalErr, gorm.ErrRecordNotFound):
			result := tx.Model(&models.InboxReadState{}).
				Where("id = ?", legacyState.ID).
				Update("sender", canonicalJID)
			if result.Error != nil {
				return rollback(result.Error)
			}
			affected += result.RowsAffected
		case canonicalErr != nil:
			return rollback(canonicalErr)
		default:
			unread := canonicalState.WhatsAppUnreadCount
			if legacyState.WhatsAppSynced && (!canonicalState.WhatsAppSynced || legacyState.UpdatedAt.After(canonicalState.UpdatedAt)) {
				unread = legacyState.WhatsAppUnreadCount
			}
			updates := map[string]interface{}{
				"last_read_at":           newerTimePointer(canonicalState.LastReadAt, legacyState.LastReadAt),
				"last_msg_at":            newerTimePointer(canonicalState.LastMsgAt, legacyState.LastMsgAt),
				"whats_app_state_at":     newerTimePointer(canonicalState.WhatsAppStateAt, legacyState.WhatsAppStateAt),
				"whats_app_synced":       canonicalState.WhatsAppSynced || legacyState.WhatsAppSynced,
				"whats_app_unread_count": unread,
				"updated_at":             time.Now(),
			}
			result := tx.Model(&models.InboxReadState{}).Where("id = ?", canonicalState.ID).Updates(updates)
			if result.Error != nil {
				return rollback(result.Error)
			}
			affected += result.RowsAffected
			result = tx.Delete(&models.InboxReadState{}, legacyState.ID)
			if result.Error != nil {
				return rollback(result.Error)
			}
			affected += result.RowsAffected
		}
	}

	for _, table := range []string{"chat_histories", "handoffs", "ai_turns", "cs_activity_logs", "closing_records"} {
		result := tx.Table(table).
			Where("agent_id = ? AND sender = ?", agentID, legacySender).
			Update("sender", canonicalJID)
		if result.Error != nil {
			return rollback(result.Error)
		}
		affected += result.RowsAffected
	}

	var legacyLabels []models.ChatLabel
	if err := tx.Where("agent_id = ? AND sender = ?", agentID, legacySender).Find(&legacyLabels).Error; err != nil {
		return rollback(err)
	}
	for _, label := range legacyLabels {
		label.ID = 0
		label.Sender = canonicalJID
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&label).Error; err != nil {
			return rollback(err)
		}
	}
	if len(legacyLabels) > 0 {
		result := tx.Where("agent_id = ? AND sender = ?", agentID, legacySender).Delete(&models.ChatLabel{})
		if result.Error != nil {
			return rollback(result.Error)
		}
		affected += result.RowsAffected
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return affected, nil
}

// ensureCanonicalChatMessageIDs menjadikan (agent_id, wa_msg_id) identitas
// kanonik tanpa melarang banyak baris legacy yang wa_msg_id-nya kosong.
//
// MySQL tidak memiliki partial unique index. Karena itu index memakai generated
// column NULLIF(wa_msg_id, ”): nilai kosong menjadi NULL (boleh berulang),
// sedangkan ID WhatsApp non-kosong wajib unik per agent.
type canonicalGeneratedColumnInfo struct {
	DataType             string `gorm:"column:data_type"`
	MaxLength            int64  `gorm:"column:max_length"`
	CollationName        string `gorm:"column:collation_name"`
	Extra                string `gorm:"column:extra"`
	GenerationExpression string `gorm:"column:generation_expression"`
}

type canonicalWAMessageIDColumnInfo struct {
	DataType      string `gorm:"column:data_type"`
	MaxLength     int64  `gorm:"column:max_length"`
	CollationName string `gorm:"column:collation_name"`
}

func preflightCanonicalChatSchema() error {
	var tableCount int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = 'chat_histories'
	`).Scan(&tableCount).Error; err != nil {
		return fmt.Errorf("gagal memeriksa tabel chat_histories: %w", err)
	}
	if tableCount == 0 {
		return nil
	}

	keyExists, keyInfo, err := readCanonicalGeneratedColumn()
	if err != nil {
		return err
	}
	if keyExists && !canonicalGeneratedColumnValid(
		keyInfo.DataType,
		keyInfo.MaxLength,
		keyInfo.CollationName,
		keyInfo.Extra,
		keyInfo.GenerationExpression,
	) && !canonicalGeneratedColumnLegacyCollationOnly(
		keyInfo.DataType,
		keyInfo.MaxLength,
		keyInfo.CollationName,
		keyInfo.Extra,
		keyInfo.GenerationExpression,
	) {
		return invalidCanonicalGeneratedColumnError(keyInfo)
	}
	indexExists, indexParts, err := readCanonicalMessageIndex()
	if err != nil {
		return err
	}
	if indexExists && !canonicalMessageIndexValid(indexParts) {
		return fmt.Errorf("unique index pesan tidak valid sebelum migrasi: bagian=%v", indexParts)
	}

	waIDExists, waIDInfo, err := readCanonicalWAMessageIDColumn()
	if err != nil {
		return err
	}
	if waIDExists && !canonicalWAMessageIDColumnValid(waIDInfo) {
		return validateWAMessageIDsForASCII()
	}
	return nil
}

func ensureCanonicalChatMessageIDs() error {
	keyExistsBeforeMigration, keyInfoBeforeMigration, err := readCanonicalGeneratedColumn()
	if err != nil {
		return err
	}
	if keyExistsBeforeMigration && !canonicalGeneratedColumnValid(
		keyInfoBeforeMigration.DataType,
		keyInfoBeforeMigration.MaxLength,
		keyInfoBeforeMigration.CollationName,
		keyInfoBeforeMigration.Extra,
		keyInfoBeforeMigration.GenerationExpression,
	) && !canonicalGeneratedColumnLegacyCollationOnly(
		keyInfoBeforeMigration.DataType,
		keyInfoBeforeMigration.MaxLength,
		keyInfoBeforeMigration.CollationName,
		keyInfoBeforeMigration.Extra,
		keyInfoBeforeMigration.GenerationExpression,
	) {
		return invalidCanonicalGeneratedColumnError(keyInfoBeforeMigration)
	}
	indexExistsBeforeMigration, indexPartsBeforeMigration, err := readCanonicalMessageIndex()
	if err != nil {
		return err
	}
	if indexExistsBeforeMigration && !canonicalMessageIndexValid(indexPartsBeforeMigration) {
		return fmt.Errorf("unique index pesan tidak valid sebelum normalisasi: bagian=%v", indexPartsBeforeMigration)
	}

	// ID stanza WhatsApp bersifat case-sensitive. Kolasi default database biasanya
	// case-insensitive, yang dapat menganggap "AbC" sama dengan "aBc".
	waIDExists, waIDInfo, err := readCanonicalWAMessageIDColumn()
	if err != nil {
		return err
	}
	if !waIDExists {
		return fmt.Errorf("kolom wa_msg_id tidak ditemukan setelah migrasi")
	}
	if !canonicalWAMessageIDColumnValid(waIDInfo) {
		if err := validateWAMessageIDsForASCII(); err != nil {
			return err
		}
		if err := DB.Exec(`
			ALTER TABLE chat_histories
			MODIFY COLUMN wa_msg_id VARCHAR(64)
			CHARACTER SET ascii COLLATE ascii_bin NULL
		`).Error; err != nil {
			return fmt.Errorf("gagal memasang kolasi biner wa_msg_id: %w", err)
		}
	}

	type duplicateKey struct {
		AgentID uint
		WAMsgID string
		Count   int64
	}
	var duplicates []duplicateKey
	if err := DB.Model(&models.ChatHistory{}).
		Select("agent_id, BINARY TRIM(wa_msg_id) AS wa_msg_id, COUNT(*) AS count").
		Where("TRIM(COALESCE(wa_msg_id, '')) <> ''").
		Group("agent_id, BINARY TRIM(wa_msg_id)").
		Having("COUNT(*) > 1").
		Scan(&duplicates).Error; err != nil {
		return fmt.Errorf("gagal mengaudit duplikat wa_msg_id: %w", err)
	}

	removed := int64(0)
	for _, duplicate := range duplicates {
		var rows []models.ChatHistory
		if err := DB.Where("agent_id = ? AND BINARY TRIM(wa_msg_id) = BINARY ?", duplicate.AgentID, duplicate.WAMsgID).
			Order("id ASC").Find(&rows).Error; err != nil {
			return fmt.Errorf("gagal membaca duplikat wa_msg_id agent %d: %w", duplicate.AgentID, err)
		}
		if len(rows) < 2 {
			return fmt.Errorf("hasil audit duplikat wa_msg_id agent %d berubah saat migrasi", duplicate.AgentID)
		}
		keeper := rows[0]
		updates := mergedCanonicalChatFields(keeper, rows[1:])
		updates["wa_msg_id"] = duplicate.WAMsgID
		tx := DB.Begin()
		if tx.Error != nil {
			return fmt.Errorf("gagal memulai merge duplikat wa_msg_id: %w", tx.Error)
		}
		ids := make([]uint, 0, len(rows)-1)
		for _, row := range rows[1:] {
			ids = append(ids, row.ID)
		}
		result := tx.Where("id IN ?", ids).Delete(&models.ChatHistory{})
		if result.Error != nil {
			tx.Rollback()
			return fmt.Errorf("gagal menghapus duplikat wa_msg_id: %w", result.Error)
		}
		if err := tx.Model(&models.ChatHistory{}).Where("id = ?", keeper.ID).Updates(updates).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("gagal menggabungkan data duplikat wa_msg_id: %w", err)
		}
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("gagal menyimpan merge duplikat wa_msg_id: %w", err)
		}
		removed += result.RowsAffected
	}
	if removed > 0 {
		log.Printf("Deduplikasi identitas pesan WhatsApp: %d baris duplikat digabung", removed)
	}
	if result := DB.Exec(`
		UPDATE chat_histories
		SET wa_msg_id = TRIM(wa_msg_id)
		WHERE wa_msg_id IS NOT NULL
			AND BINARY wa_msg_id <> BINARY TRIM(wa_msg_id)
	`); result.Error != nil {
		return fmt.Errorf("gagal menormalkan wa_msg_id lama: %w", result.Error)
	}

	keyExists, generatedInfo, err := readCanonicalGeneratedColumn()
	if err != nil {
		return err
	}
	if !keyExists {
		if err := DB.Exec(`
			ALTER TABLE chat_histories
			ADD COLUMN wa_msg_key VARCHAR(64)
			CHARACTER SET ascii COLLATE ascii_bin
			GENERATED ALWAYS AS (NULLIF(TRIM(wa_msg_id), '')) STORED
		`).Error; err != nil {
			return fmt.Errorf("gagal memasang generated key pesan: %w", err)
		}
		keyExists, generatedInfo, err = readCanonicalGeneratedColumn()
		if err != nil {
			return err
		}
		if !keyExists {
			return fmt.Errorf("generated key pesan tidak ditemukan setelah migrasi")
		}
	}
	// Build lama sempat membuat generated column kanonik dengan kolasi default
	// database. Hanya signature legacy yang dikenal itu yang boleh diperbaiki
	// otomatis; bentuk lain dihentikan agar kolom manual tidak ditimpa diam-diam.
	if !canonicalGeneratedColumnValid(
		generatedInfo.DataType,
		generatedInfo.MaxLength,
		generatedInfo.CollationName,
		generatedInfo.Extra,
		generatedInfo.GenerationExpression,
	) {
		if !canonicalGeneratedColumnLegacyCollationOnly(
			generatedInfo.DataType,
			generatedInfo.MaxLength,
			generatedInfo.CollationName,
			generatedInfo.Extra,
			generatedInfo.GenerationExpression,
		) {
			return invalidCanonicalGeneratedColumnError(generatedInfo)
		}
		log.Printf("Memperbaiki kolasi generated key pesan lama dari %s ke ascii_bin", generatedInfo.CollationName)
		if err := DB.Exec(`
			ALTER TABLE chat_histories
			MODIFY COLUMN wa_msg_key VARCHAR(64)
			CHARACTER SET ascii COLLATE ascii_bin
			GENERATED ALWAYS AS (NULLIF(TRIM(wa_msg_id), '')) STORED
		`).Error; err != nil {
			return fmt.Errorf("gagal memperbaiki generated key pesan lama: %w", err)
		}
		keyExists, generatedInfo, err = readCanonicalGeneratedColumn()
		if err != nil {
			return err
		}
		if !keyExists {
			return fmt.Errorf("generated key pesan hilang setelah perbaikan")
		}
	}
	if !canonicalGeneratedColumnValid(
		generatedInfo.DataType,
		generatedInfo.MaxLength,
		generatedInfo.CollationName,
		generatedInfo.Extra,
		generatedInfo.GenerationExpression,
	) {
		return invalidCanonicalGeneratedColumnError(generatedInfo)
	}
	indexExists, indexParts, err := readCanonicalMessageIndex()
	if err != nil {
		return err
	}
	if indexExists && !canonicalMessageIndexValid(indexParts) {
		return fmt.Errorf("unique index pesan berubah saat migrasi: bagian=%v", indexParts)
	}
	if !indexExists {
		if err := DB.Exec(`
			CREATE UNIQUE INDEX uidx_chat_agent_wa_key
			ON chat_histories (agent_id, wa_msg_key)
		`).Error; err != nil {
			return fmt.Errorf("gagal memasang unique index pesan: %w", err)
		}
	}
	// Verifikasi ulang hasil DDL. Startup tidak boleh lanjut dalam mode diam-diam
	// case-insensitive/non-unique karena itu dapat menelan pesan ID berbeda.
	waIDExists, waIDInfo, err = readCanonicalWAMessageIDColumn()
	if err != nil {
		return err
	}
	if !waIDExists || !canonicalWAMessageIDColumnValid(waIDInfo) {
		return fmt.Errorf(
			"kolom wa_msg_id tidak valid (exists=%t type=%s length=%d collation=%s)",
			waIDExists,
			waIDInfo.DataType,
			waIDInfo.MaxLength,
			waIDInfo.CollationName,
		)
	}
	indexExists, indexParts, err = readCanonicalMessageIndex()
	if err != nil {
		return err
	}
	if !indexExists || !canonicalMessageIndexValid(indexParts) {
		return fmt.Errorf("unique index pesan tidak valid: bagian=%v", indexParts)
	}
	return nil
}

type canonicalIndexPart struct {
	Seq          int    `gorm:"column:seq"`
	ColumnName   string `gorm:"column:column_name"`
	PrefixLength int64  `gorm:"column:prefix_length"`
	NonUnique    int    `gorm:"column:non_unique"`
}

func canonicalMessageIndexValid(parts []canonicalIndexPart) bool {
	return len(parts) == 2 &&
		parts[0].NonUnique == 0 && parts[0].Seq == 1 && parts[0].ColumnName == "agent_id" && parts[0].PrefixLength == 0 &&
		parts[1].NonUnique == 0 && parts[1].Seq == 2 && parts[1].ColumnName == "wa_msg_key" && parts[1].PrefixLength == 0
}

func readCanonicalMessageIndex() (bool, []canonicalIndexPart, error) {
	var parts []canonicalIndexPart
	if err := DB.Raw(`
		SELECT
			SEQ_IN_INDEX AS seq,
			COLUMN_NAME AS column_name,
			COALESCE(SUB_PART, 0) AS prefix_length,
			NON_UNIQUE AS non_unique
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = 'chat_histories'
			AND INDEX_NAME = 'uidx_chat_agent_wa_key'
		ORDER BY SEQ_IN_INDEX
	`).Scan(&parts).Error; err != nil {
		return false, nil, fmt.Errorf("gagal memeriksa unique index pesan: %w", err)
	}
	return len(parts) > 0, parts, nil
}

func readCanonicalWAMessageIDColumn() (bool, canonicalWAMessageIDColumnInfo, error) {
	var rows []canonicalWAMessageIDColumnInfo
	if err := DB.Raw(`
		SELECT
			COALESCE(DATA_TYPE, '') AS data_type,
			COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS max_length,
			COALESCE(COLLATION_NAME, '') AS collation_name
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = 'chat_histories'
			AND COLUMN_NAME = 'wa_msg_id'
	`).Scan(&rows).Error; err != nil {
		return false, canonicalWAMessageIDColumnInfo{}, fmt.Errorf("gagal memeriksa kolom wa_msg_id: %w", err)
	}
	if len(rows) == 0 {
		return false, canonicalWAMessageIDColumnInfo{}, nil
	}
	return true, rows[0], nil
}

func canonicalWAMessageIDColumnValid(info canonicalWAMessageIDColumnInfo) bool {
	return strings.EqualFold(strings.TrimSpace(info.DataType), "varchar") &&
		info.MaxLength == 64 &&
		strings.EqualFold(strings.TrimSpace(info.CollationName), "ascii_bin")
}

func readCanonicalGeneratedColumn() (bool, canonicalGeneratedColumnInfo, error) {
	var rows []canonicalGeneratedColumnInfo
	if err := DB.Raw(`
		SELECT
			COALESCE(DATA_TYPE, '') AS data_type,
			COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS max_length,
			COALESCE(COLLATION_NAME, '') AS collation_name,
			COALESCE(EXTRA, '') AS extra,
			COALESCE(GENERATION_EXPRESSION, '') AS generation_expression
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
			AND TABLE_NAME = 'chat_histories'
			AND COLUMN_NAME = 'wa_msg_key'
	`).Scan(&rows).Error; err != nil {
		return false, canonicalGeneratedColumnInfo{}, fmt.Errorf("gagal memeriksa generated key pesan: %w", err)
	}
	if len(rows) == 0 {
		return false, canonicalGeneratedColumnInfo{}, nil
	}
	return true, rows[0], nil
}

func invalidCanonicalGeneratedColumnError(info canonicalGeneratedColumnInfo) error {
	return fmt.Errorf(
		"generated key pesan tidak valid (type=%s length=%d collation=%s extra=%s expression=%s)",
		info.DataType,
		info.MaxLength,
		info.CollationName,
		info.Extra,
		info.GenerationExpression,
	)
}

func canonicalGeneratedColumnValid(dataType string, maxLength int64, collation, extra, expression string) bool {
	normalizedExpression := normalizeGeneratedExpression(expression)
	return strings.EqualFold(strings.TrimSpace(dataType), "varchar") &&
		maxLength == 64 &&
		strings.EqualFold(strings.TrimSpace(collation), "ascii_bin") &&
		strings.EqualFold(strings.TrimSpace(extra), "stored generated") &&
		canonicalNullIfEmptyExpression(normalizedExpression)
}

func canonicalGeneratedColumnLegacyCollationOnly(dataType string, maxLength int64, collation, extra, expression string) bool {
	return strings.EqualFold(strings.TrimSpace(dataType), "varchar") &&
		maxLength == 64 &&
		!strings.EqualFold(strings.TrimSpace(collation), "ascii_bin") &&
		strings.EqualFold(strings.TrimSpace(extra), "stored generated") &&
		canonicalNullIfEmptyExpression(normalizeGeneratedExpression(expression))
}

func normalizeGeneratedExpression(expression string) string {
	normalized := strings.ReplaceAll(expression, "\\'", "'")
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(normalized, "`", ""))), "")
}

func canonicalNullIfEmptyExpression(expression string) bool {
	const prefix = "nullif(trim(wa_msg_id),"
	if !strings.HasPrefix(expression, prefix) || !strings.HasSuffix(expression, ")") {
		return false
	}
	emptyArg := strings.TrimSuffix(strings.TrimPrefix(expression, prefix), ")")
	if emptyArg == "''" {
		return true
	}
	if !strings.HasPrefix(emptyArg, "_") || !strings.HasSuffix(emptyArg, "''") {
		return false
	}
	charset := strings.TrimSuffix(strings.TrimPrefix(emptyArg, "_"), "''")
	switch charset {
	case "ascii", "utf8", "utf8mb3", "utf8mb4":
		return true
	default:
		return false
	}
}

func validateWAMessageIDsForASCII() error {
	rows, err := DB.Model(&models.ChatHistory{}).
		Select("id, wa_msg_id").
		Where("TRIM(COALESCE(wa_msg_id, '')) <> ''").
		Rows()
	if err != nil {
		return fmt.Errorf("gagal mengaudit wa_msg_id sebelum migrasi: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rowID uint
		var waMsgID string
		if err := rows.Scan(&rowID, &waMsgID); err != nil {
			return fmt.Errorf("gagal membaca wa_msg_id saat audit: %w", err)
		}
		if len(waMsgID) > 64 {
			return fmt.Errorf("wa_msg_id baris %d melebihi 64 byte; migrasi dihentikan agar ID tidak terpotong", rowID)
		}
		if !waMessageIDIsASCII(waMsgID) {
			return fmt.Errorf("wa_msg_id baris %d mengandung karakter non-ASCII; migrasi dihentikan agar ID tidak berubah", rowID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("gagal menyelesaikan audit wa_msg_id: %w", err)
	}
	return nil
}

func waMessageIDIsASCII(value string) bool {
	for _, b := range []byte(value) {
		if b > 0x7f {
			return false
		}
	}
	return true
}

func mergedCanonicalChatFields(keeper models.ChatHistory, duplicates []models.ChatHistory) map[string]interface{} {
	updates := make(map[string]interface{})
	bestCreatedAt := keeper.CreatedAt
	for _, row := range duplicates {
		if strings.TrimSpace(keeper.Message) == "" && strings.TrimSpace(keeper.Reply) == "" {
			if strings.TrimSpace(row.Message) != "" || strings.TrimSpace(row.Reply) != "" {
				keeper.Message, keeper.Reply, keeper.FromHuman = row.Message, row.Reply, row.FromHuman
				updates["message"], updates["reply"], updates["from_human"] = row.Message, row.Reply, row.FromHuman
			}
		}
		if keeper.MediaType == "" && row.MediaType != "" {
			keeper.MediaType = row.MediaType
			updates["media_type"] = row.MediaType
		}
		if keeper.MediaPath == "" && row.MediaPath != "" {
			keeper.MediaPath = row.MediaPath
			updates["media_path"] = row.MediaPath
		}
		if len(keeper.MediaMetadata) == 0 && len(row.MediaMetadata) > 0 {
			keeper.MediaMetadata = row.MediaMetadata
			updates["media_metadata"] = row.MediaMetadata
		}
		if keeper.MediaFetchStatus == "" && row.MediaFetchStatus != "" {
			keeper.MediaFetchStatus = row.MediaFetchStatus
			updates["media_fetch_status"] = row.MediaFetchStatus
		}
		if keeper.FileName == "" && row.FileName != "" {
			keeper.FileName = row.FileName
			updates["file_name"] = row.FileName
		}
		if keeper.Mimetype == "" && row.Mimetype != "" {
			keeper.Mimetype = row.Mimetype
			updates["mimetype"] = row.Mimetype
		}
		if keeper.ReplyTo == "" && row.ReplyTo != "" {
			keeper.ReplyTo = row.ReplyTo
			updates["reply_to"] = row.ReplyTo
		}
		if keeper.ReplyText == "" && row.ReplyText != "" {
			keeper.ReplyText = row.ReplyText
			updates["reply_text"] = row.ReplyText
		}
		if row.Revoked && !keeper.Revoked {
			keeper.Revoked = true
			updates["revoked"] = true
		}
		if deliveryRank(row.DeliveryStatus) > deliveryRank(keeper.DeliveryStatus) {
			keeper.DeliveryStatus = row.DeliveryStatus
			updates["delivery_status"] = row.DeliveryStatus
		}
		if !row.CreatedAt.IsZero() && (bestCreatedAt.IsZero() || row.CreatedAt.Before(bestCreatedAt)) {
			bestCreatedAt = row.CreatedAt
		}
	}
	if !bestCreatedAt.IsZero() && !bestCreatedAt.Equal(keeper.CreatedAt) {
		updates["created_at"] = bestCreatedAt
	}
	return updates
}

func deliveryRank(status string) int {
	switch strings.TrimSpace(status) {
	case "played":
		return 6
	case "read":
		return 5
	case "read_inferred":
		return 4
	case "delivered":
		return 3
	case "sent":
		return 2
	case "pending_retry":
		return 1
	default:
		return 0
	}
}

// validDBName memastikan nama database hanya berisi huruf/angka/underscore
// (mencegah injeksi pada query CREATE DATABASE yang merangkai nama).
func validDBName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// backfillKnowledgeCharCount mengisi char_count knowledge lama (kolom baru) = panjang Answer,
// agar perhitungan kuota karakter akurat. DB-agnostic, hanya menyentuh baris yang masih 0.
func backfillKnowledgeCharCount() {
	var rows []models.Knowledge
	DB.Where("char_count = 0 AND answer <> ''").Find(&rows)
	for i := range rows {
		DB.Model(&models.Knowledge{}).Where("id = ?", rows[i].ID).
			Update("char_count", len([]rune(rows[i].Answer)))
	}
	if len(rows) > 0 {
		log.Printf("Backfill char_count untuk %d knowledge lama", len(rows))
	}
}

// recoverStuckCrawlJobs membereskan job yang menggantung saat server restart di tengah proses.
// Goroutine crawl/training mati saat restart, jadi statusnya tak akan pernah berubah sendiri:
// crawl yang belum kelar -> failed; training yang belum kelar -> done (halaman yang sudah jadi tetap aman).
func recoverStuckCrawlJobs() {
	c := DB.Model(&models.CrawlJob{}).Where("status IN ?", []string{"pending", "crawling"}).
		Updates(map[string]any{"status": "failed", "error": "terhenti karena server restart"})
	t := DB.Model(&models.CrawlJob{}).Where("status IN ?", []string{"training", "stopping"}).
		Update("status", "done")
	// Halaman yang sempat berstatus "training" saat restart -> kembalikan ke "crawled" agar bisa dilatih ulang.
	DB.Model(&models.CrawlPage{}).Where("status = ?", "training").Update("status", "crawled")
	if c.RowsAffected > 0 || t.RowsAffected > 0 {
		log.Printf("Recover crawl job menggantung: %d crawl -> failed, %d training -> done", c.RowsAffected, t.RowsAffected)
	}
}

// seedSuperAdmin memastikan ada satu operator platform (login ke /admin).
func seedSuperAdmin() {
	var n int64
	DB.Model(&models.User{}).Where("is_super_admin = ?", true).Count(&n)
	if n > 0 {
		syncSuperAdminPassword() // jaga password/username super-admin sinkron dengan env
		return
	}
	username := config.Env("SUPERADMIN_USERNAME", "superadmin")
	pw := os.Getenv("SUPERADMIN_PASSWORD")
	if pw == "" {
		log.Println("Seeder: SUPERADMIN_PASSWORD tidak diset — skip superadmin")
		return
	}
	// Tolak password lemah agar tidak ada instalasi produksi dengan kredensial mudah ditebak.
	if len(pw) < 12 {
		log.Println("Seeder: SUPERADMIN_PASSWORD terlalu pendek (min 12 karakter) — superadmin TIDAK dibuat")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	DB.Create(&models.User{
		Name: "Super Admin", Username: username, Email: "super@wa-assistant.local",
		Password: string(hash), IsSuperAdmin: true, Role: "admin",
	})
	log.Printf("Seeder: super admin '%s' dibuat", username)
}

// syncSuperAdminPassword memperbarui kredensial super-admin agar cocok dengan env
// SUPERADMIN_PASSWORD / SUPERADMIN_USERNAME (kalau diisi). Cara aman ganti password
// super-admin TANPA lewat chat: set SUPERADMIN_PASSWORD di .env lalu restart service.
// Kalau env kosong, kredensial yang ada dibiarkan apa adanya.
func syncSuperAdminPassword() {
	pw := os.Getenv("SUPERADMIN_PASSWORD")
	if pw == "" {
		return
	}
	username := config.Env("SUPERADMIN_USERNAME", "superadmin")
	var u models.User
	if DB.Where("is_super_admin = ?", true).First(&u).Error != nil {
		return
	}
	// Sudah cocok → cukup pastikan username sinkron.
	// Password TIDAK di-overwrite — user bisa ganti dari dashboard.
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)) == nil {
		if u.Username != username {
			DB.Model(&u).Update("username", username)
		}
		return
	}
	// Password di DB beda dengan .env → user sudah ganti via dashboard. Hormati perubahan user.
	log.Printf("Super admin '%s': password di DB berbeda dari .env (user sudah ganti via dashboard)", username)
}

// seedDefaultTenant memastikan tenant ID=1 selalu ada + punya minimal 1 agent.
func seedDefaultTenant() {
	// Pastikan tenant ID 1 ada.
	var t models.Tenant
	if DB.First(&t, uint(1)).Error != nil {
		t = models.Tenant{Name: "Default"}
		t.ID = 1
		DB.Create(&t)
		log.Printf("Seeder: tenant default (id=1) dibuat")
	}

	// Pastikan tenant 1 punya minimal 1 agent.
	var agentCount int64
	DB.Model(&models.Agent{}).Where("tenant_id = 1").Count(&agentCount)
	if agentCount == 0 {
		def := models.Agent{TenantID: 1, Name: "CS Utama", Tone: "ramah"}
		DB.Create(&def)
		DB.Model(&models.Knowledge{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id", def.ID)
		DB.Model(&models.ChatHistory{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id", def.ID)
		log.Printf("Seeder: agent default 'CS Utama' dibuat untuk tenant 1")
	}

	// Pindahkan data yatim ke tenant 1.
	DB.Model(&models.Agent{}).Where("tenant_id = 0 OR tenant_id IS NULL").Update("tenant_id", 1)
	DB.Model(&models.Knowledge{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id", 1)
	DB.Model(&models.ChatHistory{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id", 1)
}
