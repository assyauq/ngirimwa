package services

import (
	"strings"
	"testing"
	"time"

	"wa-assistant/backend/models"
)

func TestSelectAIResponsePolicy(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		kb   int
		want string
	}{
		{name: "greeting", msg: "Halo kak", want: "social"},
		{name: "factual", msg: "Berapa harga paket premium?", kb: 1, want: "factual"},
		{name: "catalog", msg: "Paket apa saja yang tersedia?", kb: 3, want: "catalog"},
		{name: "conversation", msg: "Saya masih bingung kak", want: "conversation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := selectAIResponsePolicy(tc.msg, tc.msg, "", tc.kb)
			if got.Name != tc.want {
				t.Fatalf("policy=%s, want=%s", got.Name, tc.want)
			}
		})
	}
}

func TestResponseNeedsCondensing(t *testing.T) {
	policy := aiResponsePolicy{Name: "factual", MaxTokens: 320, MaxRunes: 200, MaxSentences: 3}
	if responseNeedsCondensing("Harganya Rp75.000 ya kak. Stok tersedia.", policy) {
		t.Fatal("jawaban ringkas tidak boleh direvisi")
	}
	if !responseNeedsCondensing("Satu. Dua. Tiga. Empat.", policy) {
		t.Fatal("jawaban melewati batas kalimat harus direvisi")
	}
	if responseNeedsCondensing("[[START_FORM:12]]", policy) {
		t.Fatal("directive internal tidak boleh diubah")
	}
}

func TestAdaptiveKnowledgeLimit(t *testing.T) {
	if got := adaptiveKnowledgeLimit("berapa harga paket premium?"); got != 2 {
		t.Fatalf("pertanyaan spesifik limit=%d, want=2", got)
	}
	if got := adaptiveKnowledgeLimit("apa perbedaan paket A vs paket B?"); got != 3 {
		t.Fatalf("perbandingan limit=%d, want=3", got)
	}
	if got := adaptiveKnowledgeLimit("produk apa saja yang tersedia?"); got != 5 {
		t.Fatalf("katalog limit=%d, want=5", got)
	}
}

func TestKnowledgeFreshnessPrefersVerification(t *testing.T) {
	created := time.Now().AddDate(-2, 0, 0)
	updated := time.Now().AddDate(-1, 0, 0)
	verified := time.Now().Add(-time.Hour)
	k := models.Knowledge{CreatedAt: created, UpdatedAt: updated, VerifiedAt: &verified}
	if got := knowledgeFreshnessTime(k); !got.Equal(verified) {
		t.Fatalf("freshness=%s, want verified_at=%s", got, verified)
	}
	k.VerifiedAt = nil
	if got := knowledgeFreshnessTime(k); !got.Equal(updated) {
		t.Fatalf("freshness=%s, want updated_at=%s", got, updated)
	}
}

func TestEffectiveKnowledgeItems(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	items := []KBItem{
		{K: models.Knowledge{ID: 1}},
		{K: models.Knowledge{ID: 2, EffectiveFrom: &future}},
		{K: models.Knowledge{ID: 3, EffectiveUntil: &past}},
		{K: models.Knowledge{ID: 4, EffectiveFrom: &past, EffectiveUntil: &future}},
	}
	got := effectiveKnowledgeItems(items, now)
	ids := make([]uint, 0, len(got))
	for _, item := range got {
		ids = append(ids, item.K.ID)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 4 {
		t.Fatalf("effective knowledge ids=%v, want=[1 4]", ids)
	}
}

func TestConcisePolicyPromptKeepsGroundingLanguage(t *testing.T) {
	p := selectAIResponsePolicy("berapa harga paket?", "berapa harga paket?", "Harga: Rp75.000", 1)
	if p.MaxTokens > 400 || p.MaxSentences > 4 {
		t.Fatalf("policy faktual terlalu longgar: %+v", p)
	}
	if strings.TrimSpace(p.Name) == "" {
		t.Fatal("policy wajib bernama agar dapat diaudit")
	}
}
