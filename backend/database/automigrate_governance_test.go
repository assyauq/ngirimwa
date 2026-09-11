package database

import (
	"os"
	"strings"
	"testing"

	"kirimwa/backend/config"
)

// TEST-A & TEST-B: Verifikasi governance environment switch AUTO_MIGRATE.
func TestAutoMigrateSwitchGovernance(t *testing.T) {
	orig := os.Getenv("AUTO_MIGRATE")
	defer func() {
		if orig != "" {
			os.Setenv("AUTO_MIGRATE", orig)
		} else {
			os.Unsetenv("AUTO_MIGRATE")
		}
	}()

	// TEST-A: AUTO_MIGRATE=false -> migration path disabled (production-safe default)
	os.Unsetenv("AUTO_MIGRATE")
	if config.AutoMigrateEnabled() {
		t.Fatal("TEST-A FAILED: AUTO_MIGRATE harus default ke false saat tidak diset")
	}

	os.Setenv("AUTO_MIGRATE", "false")
	if config.AutoMigrateEnabled() {
		t.Fatal("TEST-A FAILED: AUTO_MIGRATE=false harus mengembalikan false")
	}

	os.Setenv("AUTO_MIGRATE", "0")
	if config.AutoMigrateEnabled() {
		t.Fatal("TEST-A FAILED: AUTO_MIGRATE=0 harus mengembalikan false")
	}

	// TEST-B: AUTO_MIGRATE=true -> migration path enabled
	os.Setenv("AUTO_MIGRATE", "true")
	if !config.AutoMigrateEnabled() {
		t.Fatal("TEST-B FAILED: AUTO_MIGRATE=true harus mengembalikan true")
	}

	os.Setenv("AUTO_MIGRATE", "1")
	if !config.AutoMigrateEnabled() {
		t.Fatal("TEST-B FAILED: AUTO_MIGRATE=1 harus mengembalikan true")
	}
}

// TEST-C: AUTO_MIGRATE=false -> Preflight lolos jika seluruh tabel dan skema wajib ada.
func TestPreflightPassesWhenRequiredSchemaExists(t *testing.T) {
	// Pastikan seluruh tabel wajib dikenali dan tidak ada missing jika tabel ada
	missing := CheckMissingTables(RequiredCurrentTables, RequiredCurrentTables)
	if len(missing) > 0 {
		t.Fatalf("TEST-C FAILED: seluruh tabel wajib ada tetapi dilaporkan missing: %v", missing)
	}

	// Verifikasi validator skema chat kanonik secara read-only
	waIDInfo := canonicalWAMessageIDColumnInfo{
		DataType:      "varchar",
		MaxLength:     64,
		CollationName: "ascii_bin",
	}
	keyInfo := canonicalGeneratedColumnInfo{
		DataType:             "varchar",
		MaxLength:            64,
		CollationName:        "ascii_bin",
		Extra:                "STORED GENERATED",
		GenerationExpression: "nullif(trim(`wa_msg_id`),_ascii'')",
	}
	indexParts := []canonicalIndexPart{
		{Seq: 1, ColumnName: "agent_id", NonUnique: 0},
		{Seq: 2, ColumnName: "wa_msg_key", NonUnique: 0},
	}

	err := ValidateCanonicalChatSchemaReadonly(true, waIDInfo, true, keyInfo, true, indexParts)
	if err != nil {
		t.Fatalf("TEST-C FAILED: skema chat kanonik valid harus lolos preflight: %v", err)
	}
}

// TEST-D: AUTO_MIGRATE=false -> Preflight gagal secara jelas jika skema wajib belum lengkap.
func TestPreflightFailsClearlyWhenRequiredSchemaMissing(t *testing.T) {
	// 1. Uji tabel missing
	partialTables := []string{"users", "agents", "tenants"}
	missing := CheckMissingTables(partialTables, RequiredCurrentTables)
	if len(missing) == 0 {
		t.Fatal("TEST-D FAILED: CheckMissingTables harus mendeteksi tabel yang hilang")
	}
	// Pastikan chat_histories terdeteksi hilang
	foundChat := false
	for _, m := range missing {
		if m == "chat_histories" {
			foundChat = true
			break
		}
	}
	if !foundChat {
		t.Fatal("TEST-D FAILED: chat_histories harus terdaftar sebagai missing table")
	}

	// 2. Uji jika kolom wa_msg_id hilang
	waIDInfo := canonicalWAMessageIDColumnInfo{DataType: "varchar", MaxLength: 64, CollationName: "ascii_bin"}
	keyInfo := canonicalGeneratedColumnInfo{
		DataType: "varchar", MaxLength: 64, CollationName: "ascii_bin", Extra: "STORED GENERATED",
		GenerationExpression: "nullif(trim(`wa_msg_id`),'')",
	}
	indexParts := []canonicalIndexPart{
		{Seq: 1, ColumnName: "agent_id", NonUnique: 0},
		{Seq: 2, ColumnName: "wa_msg_key", NonUnique: 0},
	}

	if err := ValidateCanonicalChatSchemaReadonly(false, waIDInfo, true, keyInfo, true, indexParts); err == nil {
		t.Fatal("TEST-D FAILED: harus gagal jika wa_msg_id tidak ada")
	}

	// 3. Uji jika wa_msg_key hilang
	if err := ValidateCanonicalChatSchemaReadonly(true, waIDInfo, false, keyInfo, true, indexParts); err == nil {
		t.Fatal("TEST-D FAILED: harus gagal jika wa_msg_key tidak ada")
	}

	// 4. Uji jika index uidx_chat_agent_wa_key hilang
	if err := ValidateCanonicalChatSchemaReadonly(true, waIDInfo, true, keyInfo, false, nil); err == nil {
		t.Fatal("TEST-D FAILED: harus gagal jika unique index tidak ada")
	}
}

// TEST-E & TEST-F: Startup path tidak mengandung mutasi knowledge orphan ke Agent 1.
// NULL agent_id tidak boleh diubah otomatis menjadi Agent 1.
func TestNoUnsafeKnowledgeMutationInDatabaseSource(t *testing.T) {
	content, err := os.ReadFile("database.go")
	if err != nil {
		t.Fatalf("Gagal membaca database.go: %v", err)
	}
	source := string(content)

	// Pastikan tidak ada UPDATE knowledges SET agent_id = 1 / def.ID
	forbiddenPatterns := []string{
		`&models.Knowledge{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id"`,
		`&models.Knowledge{}).Where("agent_id = 0 OR agent_id IS NULL")`,
		`UPDATE knowledges SET agent_id = 1`,
		`UPDATE knowledges SET agent_id`,
	}

	for _, pattern := range forbiddenPatterns {
		if strings.Contains(source, pattern) {
			t.Fatalf("TEST-E / TEST-F FAILED: Ditemukan mutasi tidak aman untuk knowledge di database.go: %q", pattern)
		}
	}
}

// TEST-G: Kompatibilitas tabel runtime saat ini (tidak memuat tabel SaaS fase berikutnya secara prematur).
func TestCurrentRequiredTablesDoNotContainPrematureSaaSTables(t *testing.T) {
	futureSaaSTables := []string{
		"tenant_members",
		"plans",
		"plan_features",
		"subscriptions",
		"usage_counters",
		"audit_logs",
	}

	currentSet := make(map[string]bool)
	for _, tName := range RequiredCurrentTables {
		currentSet[strings.ToLower(tName)] = true
	}

	for _, futureTable := range futureSaaSTables {
		if currentSet[futureTable] {
			t.Fatalf("TEST-G FAILED: tabel masa depan %q tidak boleh ada di RequiredCurrentTables Phase 2B.3.1", futureTable)
		}
	}
}
