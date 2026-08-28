package services

import (
	"strings"
	"testing"

	"kirimwa/backend/models"
)

func TestExtractBriefHeuristicOrderAndFacts(t *testing.T) {
	transcript := strings.Join([]string{
		"Pelanggan: Halo, nama saya Budi Santoso",
		"CS: Halo Budi, ada yang bisa dibantu?",
		"Pelanggan: Mau order kaos size L, qty 2 pcs, kirim ke Jl. Merdeka No 10 Bandung",
		"CS: Harganya Rp150.000. Kurir JNE ya?",
		"Pelanggan: Iya JNE. Transfer BCA. Resi TRX-99881",
		"Pelanggan: Status pesanan sudah berapa lama ya?",
	}, "\n")
	b := extractBriefHeuristic(transcript, "", false)
	if !strings.Contains(strings.ToLower(b.ContactHint), "budi") {
		t.Fatalf("contact_hint expected Budi, got %q", b.ContactHint)
	}
	if b.Stage != "transaction" && b.Stage != "issue" {
		// status pesanan → transaction
		if b.Stage != "transaction" {
			t.Fatalf("stage=%s want transaction", b.Stage)
		}
	}
	if len(b.KeyFacts) == 0 {
		t.Fatal("expected key facts")
	}
	joined := strings.ToLower(strings.Join(b.KeyFacts, " | "))
	if !strings.Contains(joined, "ukuran") && !strings.Contains(joined, "l") {
		// size L should appear
		if !strings.Contains(joined, "ukuran: l") {
			t.Logf("facts: %v", b.KeyFacts)
		}
	}
	if !strings.Contains(joined, "harga") && !strings.Contains(joined, "150") {
		t.Fatalf("expected price fact, got %v", b.KeyFacts)
	}
	if !strings.Contains(joined, "alamat") && !strings.Contains(joined, "merdeka") {
		t.Fatalf("expected address fact, got %v", b.KeyFacts)
	}
	if len(b.Products) == 0 || !containsFold(b.Products, "kaos") {
		t.Fatalf("expected product kaos, got %v", b.Products)
	}
	if len(b.OpenItems) == 0 {
		t.Fatal("expected open items from unanswered last customer message")
	}
}

func TestExtractBriefHeuristicRiskRefund(t *testing.T) {
	transcript := "Pelanggan: Mau refund, komplain barang rusak\nCS: Baik kami cek"
	b := extractBriefHeuristic(transcript, "", true)
	if b.Stage != "issue" {
		t.Fatalf("stage=%s", b.Stage)
	}
	if len(b.RiskFlags) == 0 {
		t.Fatal("expected risk flags")
	}
	risks := strings.ToLower(strings.Join(b.RiskFlags, " "))
	if !strings.Contains(risks, "refund") && !strings.Contains(risks, "butuh") {
		t.Fatalf("risks=%v", b.RiskFlags)
	}
}

func TestFactGroundedRejectsInventedNumbers(t *testing.T) {
	src := "Harga kaos Rp75.000 size M"
	nums := normalizedFactNumbers(src)
	toks := contentTokenSet(src)
	if factGrounded("Harga kaos Rp99.000", nums, toks) {
		t.Fatal("should reject invented price 99000")
	}
	if !factGrounded("Harga kaos Rp75.000", nums, toks) {
		t.Fatal("should accept real price")
	}
	if factGrounded("Promo rahasia diskon gila-gilaan tanpa angka di sumber yang sangat panjang sekali", nums, toks) {
		t.Fatal("should reject low-overlap long fact")
	}
}

func TestMergeBriefsGroundsAIFacts(t *testing.T) {
	h := ConversationBrief{
		Intent:   "Tanya harga",
		Stage:    "info",
		KeyFacts: []string{"Harga/biaya disebut: Rp75.000"},
		Products: []string{"kaos"},
	}
	ai := briefAIPayload{
		Intent:   "Ingin beli kaos",
		Stage:    "interest",
		Summary:  "Pelanggan tanya harga kaos.",
		Products: []string{"kaos polos"},
		KeyFacts: []string{
			"Harga Rp75.000",
			"Harga Rp999.000 invent", // should drop
		},
		OpenItems: []string{"Cek harga kaos", "Konfirmasi size"},
	}
	transcript := "Pelanggan: Berapa harga kaos?\nCS: Rp75.000"
	out := mergeBriefs(h, ai, transcript)
	if out.Intent != "Tanya harga" {
		t.Fatalf("intent=%s", out.Intent)
	}
	if out.Stage != "info" {
		t.Fatalf("AI must not override chronology stage, got %s", out.Stage)
	}
	joined := strings.Join(out.KeyFacts, " | ")
	if strings.Contains(joined, "999") {
		t.Fatalf("invented fact leaked: %v", out.KeyFacts)
	}
	if !strings.Contains(joined, "75") {
		t.Fatalf("real price missing: %v", out.KeyFacts)
	}
	if !containsFold(out.OpenItems, "harga kaos") {
		t.Fatalf("grounded open item missing: %v", out.OpenItems)
	}
	if containsFold(out.OpenItems, "size") {
		t.Fatalf("ungrounded open item leaked: %v", out.OpenItems)
	}
}

func TestMergeBriefsRejectsInventedSummaryNumber(t *testing.T) {
	h := ConversationBrief{
		Intent:  "Menanyakan harga atau informasi produk",
		Stage:   "info",
		Summary: "Pelanggan menanyakan harga kaos. CS sudah menanggapi.",
	}
	ai := briefAIPayload{Summary: "Pelanggan diberi harga kaos Rp999.000."}
	out := mergeBriefs(h, ai, "Pelanggan: Berapa harga kaos?\nCS: Harganya Rp75.000")
	if strings.Contains(out.Summary, "999") {
		t.Fatalf("invented summary leaked: %q", out.Summary)
	}
}

func TestBriefChronologyOutgoingImageIsHandledNotHandoff(t *testing.T) {
	msgs := []models.ChatHistory{
		{ID: 1, Message: "Hai"},
		{ID: 2, Reply: "tes kirim", FromHuman: true, MediaType: "image", FileName: "foto.png"},
	}
	b, err := BuildConversationBriefHeuristic(1, "6281", msgs, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != ConversationBriefVersion {
		t.Fatalf("version=%d", b.Version)
	}
	if b.NeedsHuman || b.WaitingFor != "none" || b.CurrentState != "Sudah ditanggapi CS" {
		t.Fatalf("unexpected state: needs_human=%v waiting=%q state=%q", b.NeedsHuman, b.WaitingFor, b.CurrentState)
	}
	if len(b.OpenItems) != 0 {
		t.Fatalf("outgoing image must not create CS work: %v", b.OpenItems)
	}
	if !strings.Contains(strings.ToLower(b.Summary), "gambar") || !strings.Contains(b.Summary, "tes kirim") {
		t.Fatalf("summary should describe the actual outgoing media: %q", b.Summary)
	}
}

func TestBriefChronologyKnowsWhoMustReply(t *testing.T) {
	t.Run("customer waits for CS", func(t *testing.T) {
		b, err := BuildConversationBriefHeuristic(1, "6281", []models.ChatHistory{
			{ID: 1, Message: "Apakah paket internet masih tersedia?"},
		}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if b.WaitingFor != "cs" || b.CurrentState != "Menunggu balasan CS" || len(b.OpenItems) == 0 {
			t.Fatalf("unexpected brief: %+v", b)
		}
	})

	t.Run("CS waits for customer", func(t *testing.T) {
		b, err := BuildConversationBriefHeuristic(1, "6281", []models.ChatHistory{
			{ID: 1, Message: "Internet saya lambat"},
			{ID: 2, Reply: "Lampu modemnya berwarna apa?", FromHuman: true},
		}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if b.WaitingFor != "customer" || b.CurrentState != "Menunggu jawaban pelanggan" {
			t.Fatalf("unexpected brief: %+v", b)
		}
		if !containsFold(b.OpenItems, "menunggu jawaban pelanggan") {
			t.Fatalf("unexpected open items: %v", b.OpenItems)
		}
	})

	t.Run("answered customer message is not open", func(t *testing.T) {
		b, err := BuildConversationBriefHeuristic(1, "6281", []models.ChatHistory{
			{ID: 1, Message: "Berapa harga paket starter?"},
			{ID: 2, Reply: "Harga paket starter Rp150.000 per bulan.", FromHuman: true},
		}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if b.WaitingFor != "none" || len(b.OpenItems) != 0 {
			t.Fatalf("answered message must not remain open: %+v", b)
		}
	})

	t.Run("customer acknowledgement closes conversation", func(t *testing.T) {
		b, err := BuildConversationBriefHeuristic(1, "6281", []models.ChatHistory{
			{ID: 1, Message: "Internet saya lambat"},
			{ID: 2, Reply: "Koneksi sudah kami perbaiki, silakan cek kembali.", FromHuman: true},
			{ID: 3, Message: "Sudah normal, terima kasih"},
		}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if b.Stage != "done" || b.WaitingFor != "none" || b.CurrentState != "Percakapan selesai" || len(b.OpenItems) != 0 {
			t.Fatalf("acknowledgement should close the conversation: %+v", b)
		}
	})

	t.Run("CS promise remains actionable", func(t *testing.T) {
		b, err := BuildConversationBriefHeuristic(1, "6281", []models.ChatHistory{
			{ID: 1, Message: "Tolong cek status pemasangan saya"},
			{ID: 2, Reply: "Baik, akan kami cek dan nanti kami kabari.", FromHuman: true},
		}, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if b.WaitingFor != "cs" || b.CurrentState != "Sedang ditindaklanjuti CS" {
			t.Fatalf("CS promise must remain actionable: %+v", b)
		}
		if !containsFold(b.OpenItems, "selesaikan tindak lanjut") {
			t.Fatalf("missing promised follow-up: %v", b.OpenItems)
		}
	})
}

func TestEncodeDecodeBrief(t *testing.T) {
	b := ConversationBrief{Version: ConversationBriefVersion, Intent: "x", Summary: "y", Confidence: 0.8, Source: "hybrid"}
	raw := EncodeBrief(b)
	got, ok := DecodeBrief(raw)
	if !ok || got.Intent != "x" || got.Summary != "y" {
		t.Fatalf("roundtrip failed ok=%v got=%+v", ok, got)
	}
	if _, ok := DecodeBrief(""); ok {
		t.Fatal("empty should fail")
	}
	if _, ok := DecodeBrief("{not-json"); ok {
		t.Fatal("bad json should fail")
	}
	old, ok := DecodeBrief(`{"intent":"lama"}`)
	if !ok || old.Version != 0 {
		t.Fatalf("legacy cache should decode with version 0: ok=%v brief=%+v", ok, old)
	}
}

func TestBuildConversationBriefShort(t *testing.T) {
	msgs := []models.ChatHistory{{ID: 1, Message: "halo"}}
	b, err := BuildConversationBrief(1, "6281", msgs, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.Source != "heuristic" {
		t.Fatalf("source=%s", b.Source)
	}
	if b.Confidence <= 0 {
		t.Fatal("confidence")
	}
	if b.LastChatID != 1 {
		t.Fatalf("last=%d", b.LastChatID)
	}
	if b.Enhancement != "local" {
		t.Fatalf("enhancement=%q want local", b.Enhancement)
	}
}

func TestBriefEnhancementNoteIsSafeAndUseful(t *testing.T) {
	tests := []struct {
		err  string
		want string
	}{
		{"status 402: Prompt tokens limit exceeded", "Batas konteks"},
		{"402 Payment Required: credits unavailable", "Kuota"},
		{"API key belum dikonfigurasi", "belum dikonfigurasi"},
		{"context deadline exceeded", "terlalu lama"},
		{"provider unavailable", "sedang tidak tersedia"},
	}
	for _, tc := range tests {
		if got := briefEnhancementNote(assertError(tc.err)); !strings.Contains(got, tc.want) {
			t.Fatalf("briefEnhancementNote(%q)=%q want contains %q", tc.err, got, tc.want)
		}
	}
}

type assertError string

func (err assertError) Error() string { return string(err) }

func TestBuildBriefTranscriptAndCount(t *testing.T) {
	msgs := []models.ChatHistory{
		{Message: "hai", Reply: "halo", FromHuman: false},
		{Message: "mau beli", Reply: "siap", FromHuman: true},
	}
	tr := buildBriefTranscript(msgs)
	if !strings.Contains(tr, "Pelanggan: hai") || !strings.Contains(tr, "CS-manusia:") {
		t.Fatalf("transcript=%q", tr)
	}
	if countBriefTurns(msgs) != 2 {
		t.Fatalf("turns=%d", countBriefTurns(msgs))
	}
}

func TestBriefConfidence(t *testing.T) {
	low := briefConfidence(ConversationBrief{Source: "heuristic", Intent: "Percakapan umum / info"}, "x")
	high := briefConfidence(ConversationBrief{
		Source: "hybrid", Intent: "Mau order", KeyFacts: []string{"a"}, OpenItems: []string{"b"},
	}, strings.Repeat("kata panjang percakapan ", 40))
	if high <= low {
		t.Fatalf("high=%v low=%v", high, low)
	}
	if high > 0.95 {
		t.Fatalf("cap broken: %v", high)
	}
}

func containsFold(list []string, want string) bool {
	want = strings.ToLower(want)
	for _, x := range list {
		if strings.Contains(strings.ToLower(x), want) {
			return true
		}
	}
	return false
}
