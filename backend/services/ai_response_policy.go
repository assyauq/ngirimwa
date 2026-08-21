package services

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	openai "github.com/sashabaranov/go-openai"
)

type aiResponsePolicy struct {
	Name         string
	MaxTokens    int
	MaxRunes     int
	MaxSentences int
}

var sentenceBoundaryPattern = regexp.MustCompile(`[.!?]+(?:\s|$)`)

// selectAIResponsePolicy memberi budget sesuai intent. Ini membuat aturan
// "jawab ringkas" menjadi kontrol runtime, bukan hanya imbauan di prompt.
func selectAIResponsePolicy(userMsg, retrievalQuery, productContext string, knowledgeCount int) aiResponsePolicy {
	lower := strings.ToLower(strings.TrimSpace(userMsg))
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) <= 4 {
		social := map[string]bool{
			"halo": true, "hai": true, "hi": true, "pagi": true, "siang": true,
			"sore": true, "malam": true, "makasih": true, "terimakasih": true,
			"terima": true, "kasih": true, "oke": true, "ok": true, "siap": true,
		}
		allSocial := len(words) > 0
		for _, word := range words {
			if !social[word] && word != "kak" && word != "min" {
				allSocial = false
				break
			}
		}
		if allSocial {
			return aiResponsePolicy{Name: "social", MaxTokens: 100, MaxRunes: 240, MaxSentences: 2}
		}
	}

	combined := strings.ToLower(userMsg + " " + retrievalQuery)
	for _, marker := range []string{"apa saja", "semua ", "daftar ", "pilihan ", "katalog", "banding", "perbedaan", "beda ", " vs ", " versus "} {
		if strings.Contains(combined, marker) {
			return aiResponsePolicy{Name: "catalog", MaxTokens: 520, MaxRunes: 1400, MaxSentences: 8}
		}
	}
	if looksLikeOrderProgressMessage(userMsg) {
		return aiResponsePolicy{Name: "transaction", MaxTokens: 220, MaxRunes: 500, MaxSentences: 3}
	}
	if knowledgeCount > 0 || productContext != "" {
		return aiResponsePolicy{Name: "factual", MaxTokens: 320, MaxRunes: 700, MaxSentences: 4}
	}
	return aiResponsePolicy{Name: "conversation", MaxTokens: 240, MaxRunes: 520, MaxSentences: 3}
}

func responseNeedsCondensing(reply string, policy aiResponsePolicy) bool {
	reply = strings.TrimSpace(reply)
	if reply == "" || strings.Contains(reply, "[[") {
		return false
	}
	if len([]rune(reply)) > policy.MaxRunes {
		return true
	}
	sentences := len(sentenceBoundaryPattern.FindAllString(reply, -1))
	if sentences > policy.MaxSentences {
		return true
	}
	nonEmptyLines := 0
	for _, line := range strings.Split(reply, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	return nonEmptyLines > policy.MaxSentences+2
}

func retryConciseReply(p aiPreset, messages []openai.ChatCompletionMessage, policy aiResponsePolicy) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}
	concise := make([]openai.ChatCompletionMessage, len(messages))
	copy(concise, messages)
	concise[0].Content += `

KEBIJAKAN PANJANG JAWABAN (WAJIB):
- Tulis ulang jawaban secara langsung dan natural.
- Maksimal ` + itoa(policy.MaxSentences) + ` kalimat.
- Jangan mengulang pertanyaan atau fakta yang sama.
- Jangan menambahkan pembuka, penutup, atau tawaran bantuan generik.
- Pertahankan seluruh angka dan fakta penting yang benar-benar menjawab pertanyaan.`
	req := openai.ChatCompletionRequest{
		Model: p.Model, Messages: concise, MaxTokens: policy.MaxTokens, Temperature: 0.2,
	}
	resp, err := clientForPreset(p).CreateChatCompletion(context.Background(), req)
	if err != nil || len(resp.Choices) == 0 {
		return "", false
	}
	out := sanitizeCustomerFacingReply(strings.TrimSpace(resp.Choices[0].Message.Content))
	if out == "" || responseNeedsCondensing(out, policy) {
		return "", false
	}
	return out, true
}

func itoa(value int) string {
	if value < 0 {
		return "0"
	}
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for value > 0 {
		pos--
		digits[pos] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[pos:])
}
