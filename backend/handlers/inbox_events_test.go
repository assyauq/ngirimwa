package handlers

import (
	"testing"
	"time"
)

func TestInboxEventHubScopesEventsPerAgent(t *testing.T) {
	hub := inboxEventHub{subscribers: make(map[uint]map[chan InboxEvent]struct{})}
	agentOne, cancelOne := hub.subscribe(1)
	defer cancelOne()
	agentTwo, cancelTwo := hub.subscribe(2)
	defer cancelTwo()

	published := hub.publish(1, "628111", "history", "")
	if published.AgentID != 1 || published.Sender != "628111" ||
		published.Kind != "history" || published.Revision == 0 {
		t.Fatalf("payload event tidak sesuai kontrak SSE: %+v", published)
	}

	select {
	case got := <-agentOne:
		if got != published {
			t.Fatalf("subscriber agent menerima payload berbeda: got=%+v want=%+v", got, published)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber agent yang benar tidak menerima event")
	}

	select {
	case got := <-agentTwo:
		t.Fatalf("event agent 1 bocor ke agent 2: %+v", got)
	default:
	}
}

func TestInboxEventHubDefaultsKindAndMonotonicRevision(t *testing.T) {
	hub := inboxEventHub{subscribers: make(map[uint]map[chan InboxEvent]struct{})}
	first := hub.publish(7, " 628222 ", "", "")
	second := hub.publish(7, "628222", "message", "")

	if first.Kind != "conversation" {
		t.Fatalf("kind kosong harus dinormalisasi, got %q", first.Kind)
	}
	if first.Sender != "628222" {
		t.Fatalf("sender harus di-trim, got %q", first.Sender)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision harus monotonik: first=%d second=%d", first.Revision, second.Revision)
	}
}

func TestInboxEventHubCarriesIncomingMessageIdentity(t *testing.T) {
	hub := inboxEventHub{subscribers: make(map[uint]map[chan InboxEvent]struct{})}
	event := hub.publish(3, "628333", "incoming", "wamid-123")
	if event.Kind != "incoming" || event.MessageID != "wamid-123" {
		t.Fatalf("identitas pesan incoming hilang: %+v", event)
	}
}

func TestInboxTypingEventIsTransient(t *testing.T) {
	hub := inboxEventHub{
		subscribers: make(map[uint]map[chan InboxEvent]struct{}),
		history:     make(map[uint][]InboxEvent),
	}
	live, cancelLive := hub.subscribe(5)
	defer cancelLive()
	published := hub.publishTransient(InboxEvent{
		AgentID: 5,
		Sender:  "628555",
		Kind:    "typing",
		Active:  true,
	})

	select {
	case got := <-live:
		if got != published || !got.Active {
			t.Fatalf("presence live berbeda: got=%+v want=%+v", got, published)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber live tidak menerima presence")
	}
	if len(hub.history[5]) != 0 {
		t.Fatal("presence sementara tidak boleh masuk replay history")
	}

	replayed, cancelReplay := hub.subscribeFrom(5, 0, true)
	defer cancelReplay()
	select {
	case got := <-replayed:
		t.Fatalf("presence basi ikut direplay: %+v", got)
	default:
	}
}

func TestInboxEventHubReplaysMissedEventsAfterRevision(t *testing.T) {
	hub := inboxEventHub{
		subscribers: make(map[uint]map[chan InboxEvent]struct{}),
		history:     make(map[uint][]InboxEvent),
	}
	first := hub.publish(9, "628901", "incoming", "wamid-1")
	second := hub.publish(9, "628902", "incoming", "wamid-2")
	third := hub.publish(9, "628903", "incoming", "wamid-3")

	events, cancel := hub.subscribeFrom(9, first.Revision, true)
	defer cancel()
	for _, want := range []InboxEvent{second, third} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("replay berbeda: got=%+v want=%+v", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("event revision %d tidak direplay", want.Revision)
		}
	}
	select {
	case unexpected := <-events:
		t.Fatalf("event lama/berlebih ikut direplay: %+v", unexpected)
	default:
	}
}

func TestInboxEventHubBoundsReplayHistory(t *testing.T) {
	hub := inboxEventHub{
		subscribers: make(map[uint]map[chan InboxEvent]struct{}),
		history:     make(map[uint][]InboxEvent),
	}
	for index := 0; index < inboxEventHistoryLimit+20; index++ {
		hub.publish(11, "62811", "message", "")
	}
	if got := len(hub.history[11]); got != inboxEventHistoryLimit {
		t.Fatalf("history harus dibatasi %d, got %d", inboxEventHistoryLimit, got)
	}
}

func TestInboxConversationLimit(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", 100},
		{"invalid", 100},
		{"0", 100},
		{"50", 50},
		{"999999", 200},
	}
	for _, test := range tests {
		if got := inboxConversationLimit(test.raw); got != test.want {
			t.Fatalf("limit %q = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestParseInboxConversationCursor(t *testing.T) {
	at, id, err := parseInboxConversationCursor("2026-07-29T17:34:09+07:00", "321")
	if err != nil || id != 321 || at.IsZero() {
		t.Fatalf("cursor valid gagal diparse: at=%v id=%d err=%v", at, id, err)
	}
	for _, test := range [][2]string{
		{"", "1"},
		{"2026-07-29T17:34:09+07:00", ""},
		{"bukan-waktu", "1"},
		{"2026-07-29T17:34:09+07:00", "abc"},
	} {
		if _, _, err := parseInboxConversationCursor(test[0], test[1]); err == nil {
			t.Fatalf("cursor invalid diterima: at=%q id=%q", test[0], test[1])
		}
	}
}

func TestInboxIncomingAfterID(t *testing.T) {
	if got, err := inboxIncomingAfterID(""); err != nil || got != 0 {
		t.Fatalf("cursor kosong: got=%d err=%v", got, err)
	}
	if got, err := inboxIncomingAfterID("42"); err != nil || got != 42 {
		t.Fatalf("cursor valid: got=%d err=%v", got, err)
	}
	if _, err := inboxIncomingAfterID("invalid"); err == nil {
		t.Fatal("cursor invalid harus ditolak")
	}
}
