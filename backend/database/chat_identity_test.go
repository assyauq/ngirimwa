package database

import (
	"testing"
	"time"

	"kirimwa/backend/models"
)

func TestNormalizedSenderFieldValuePreservesGroupThread(t *testing.T) {
	if got, changed := normalizedSenderFieldValue("120363425256238999@g.us"); changed || got != "" {
		t.Fatalf("JID grup tidak boleh dinormalisasi menjadi nomor: got=%q changed=%v", got, changed)
	}
	if got, changed := normalizedSenderFieldValue("6281220990678@s.whatsapp.net"); !changed || got != "6281220990678" {
		t.Fatalf("JID personal legacy harus dinormalisasi: got=%q changed=%v", got, changed)
	}
}

func TestMergedCanonicalChatFieldsPreservesBestDuplicateData(t *testing.T) {
	newer := time.Date(2026, 7, 28, 14, 16, 5, 0, time.UTC)
	older := newer.Add(-5 * time.Second)
	keeper := models.ChatHistory{
		ID: 1, AgentID: 3, WAMsgID: "WA-1", Message: "🌟 Stiker",
		DeliveryStatus: "sent", CreatedAt: newer,
	}
	metadata := []byte{1, 2, 3}
	updates := mergedCanonicalChatFields(keeper, []models.ChatHistory{{
		ID: 2, AgentID: 3, WAMsgID: "WA-1", MediaType: "sticker",
		MediaMetadata: metadata, MediaFetchStatus: "pending",
		DeliveryStatus: "read", CreatedAt: older,
	}})

	if updates["media_type"] != "sticker" {
		t.Fatalf("tipe media duplikat tidak digabung: %#v", updates)
	}
	if got, ok := updates["media_metadata"].([]byte); !ok || len(got) != len(metadata) {
		t.Fatalf("metadata media duplikat tidak dipertahankan: %#v", updates)
	}
	if updates["delivery_status"] != "read" {
		t.Fatalf("status delivery tertinggi tidak dipertahankan: %#v", updates)
	}
	if got, ok := updates["created_at"].(time.Time); !ok || !got.Equal(older) {
		t.Fatalf("timestamp kanonik harus mempertahankan timestamp paling awal: %#v", updates)
	}
}

func TestDeliveryRankNeverDowngradesRead(t *testing.T) {
	if deliveryRank("read") <= deliveryRank("delivered") {
		t.Fatal("read harus lebih tinggi dari delivered")
	}
	if deliveryRank("played") <= deliveryRank("read") {
		t.Fatal("played harus lebih tinggi dari read")
	}
}

func TestCanonicalMessageIndexRequiresExactCompositeColumns(t *testing.T) {
	valid := []canonicalIndexPart{
		{Seq: 1, ColumnName: "agent_id"},
		{Seq: 2, ColumnName: "wa_msg_key"},
	}
	if !canonicalMessageIndexValid(valid) {
		t.Fatal("index kanonik dua kolom yang benar harus diterima")
	}
	for _, parts := range [][]canonicalIndexPart{
		{{Seq: 1, ColumnName: "agent_id"}},
		{{Seq: 1, ColumnName: "wa_msg_key"}, {Seq: 2, ColumnName: "agent_id"}},
		{{Seq: 1, ColumnName: "agent_id"}, {Seq: 2, ColumnName: "wa_msg_id"}},
		{{Seq: 1, ColumnName: "agent_id"}, {Seq: 2, ColumnName: "wa_msg_key", PrefixLength: 8}},
		{{Seq: 1, ColumnName: "agent_id", NonUnique: 1}, {Seq: 2, ColumnName: "wa_msg_key", NonUnique: 1}},
	} {
		if canonicalMessageIndexValid(parts) {
			t.Fatalf("index tidak valid diterima: %v", parts)
		}
	}
}

func TestCanonicalWAMessageIDColumnRequiresASCII64(t *testing.T) {
	if !canonicalWAMessageIDColumnValid(canonicalWAMessageIDColumnInfo{
		DataType: "varchar", MaxLength: 64, CollationName: "ascii_bin",
	}) {
		t.Fatal("kolom wa_msg_id kanonik harus diterima")
	}
	for _, info := range []canonicalWAMessageIDColumnInfo{
		{DataType: "varchar", MaxLength: 255, CollationName: "ascii_bin"},
		{DataType: "varchar", MaxLength: 64, CollationName: "utf8mb4_unicode_ci"},
		{DataType: "text", MaxLength: 64, CollationName: "ascii_bin"},
	} {
		if canonicalWAMessageIDColumnValid(info) {
			t.Fatalf("kolom wa_msg_id tidak valid diterima: %+v", info)
		}
	}
}

func TestCanonicalGeneratedColumnRequiresStoredBinaryExpression(t *testing.T) {
	if !canonicalGeneratedColumnValid(
		"varchar",
		64,
		"ascii_bin",
		"STORED GENERATED",
		"nullif(trim(`wa_msg_id`),_ascii'')",
	) {
		t.Fatal("generated column kanonik yang benar harus diterima")
	}
	if !canonicalGeneratedColumnValid(
		"varchar",
		64,
		"ascii_bin",
		"STORED GENERATED",
		"nullif(trim(`wa_msg_id`),_utf8mb4'')",
	) {
		t.Fatal("literal charset MySQL yang ekuivalen harus diterima")
	}
	if !canonicalGeneratedColumnValid(
		"varchar",
		64,
		"ascii_bin",
		"STORED GENERATED",
		"nullif(trim(`wa_msg_id`),_utf8mb4\\'\\')",
	) {
		t.Fatal("literal charset yang di-escape INFORMATION_SCHEMA harus diterima")
	}
	for _, tc := range []struct {
		dataType, collation, extra, expression string
		length                                 int64
	}{
		{"varchar", "ascii_general_ci", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
		{"varchar", "ascii_bin", "VIRTUAL GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "nullif(trim(`other_id`),'')", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "nullif(trim(`other_wa_msg_id`),'')", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "nullif(trim(`wa_msg_id`),`wa_msg_id`)", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "coalesce(nullif(trim(`wa_msg_id`),''),'')", 64},
		{"text", "ascii_bin", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
		{"varchar", "ascii_bin", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 32},
	} {
		if canonicalGeneratedColumnValid(tc.dataType, tc.length, tc.collation, tc.extra, tc.expression) {
			t.Fatalf("generated column tidak valid diterima: %+v", tc)
		}
	}
}

func TestCanonicalGeneratedColumnRepairsOnlyKnownLegacyCollation(t *testing.T) {
	if !canonicalGeneratedColumnLegacyCollationOnly(
		"varchar",
		64,
		"utf8mb4_unicode_ci",
		"STORED GENERATED",
		"nullif(trim(`wa_msg_id`),_utf8mb4\\'\\')",
	) {
		t.Fatal("signature legacy collation-only harus dapat diperbaiki")
	}
	for _, tc := range []struct {
		dataType, collation, extra, expression string
		length                                 int64
	}{
		{"varchar", "ascii_bin", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
		{"varchar", "utf8mb4_unicode_ci", "VIRTUAL GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
		{"varchar", "utf8mb4_unicode_ci", "STORED GENERATED", "lower(`wa_msg_id`)", 64},
		{"varchar", "utf8mb4_unicode_ci", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 32},
		{"text", "utf8mb4_unicode_ci", "STORED GENERATED", "nullif(trim(`wa_msg_id`),'')", 64},
	} {
		if canonicalGeneratedColumnLegacyCollationOnly(tc.dataType, tc.length, tc.collation, tc.extra, tc.expression) {
			t.Fatalf("signature asing tidak boleh diperbaiki otomatis: %+v", tc)
		}
	}
}

func TestWAMessageIDASCIIValidation(t *testing.T) {
	if !waMessageIDIsASCII("3EB0ABC_def-123") {
		t.Fatal("ID stanza ASCII valid ditolak")
	}
	if waMessageIDIsASCII("pesan-é") {
		t.Fatal("ID non-ASCII harus ditolak sebelum migrasi")
	}
}
