package handlers

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendInboxClientDebugRecords(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "nested", "inbox-debug.jsonl")
	stored, err := appendInboxClientDebugRecords(
		logPath,
		7,
		"session-1",
		"Safari Test",
		[]inboxClientDebugEntry{
			{
				At: "2026-08-01T12:00:00.000Z", ElapsedMS: 1250,
				Event: " composer.input-sample\n", Details: map[string]interface{}{"length": float64(4)},
			},
			{Event: "\n\t"},
		},
	)
	if err != nil {
		t.Fatalf("appendInboxClientDebugRecords: %v", err)
	}
	if stored != 1 {
		t.Fatalf("stored = %d, want 1", stored)
	}
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permission = %o, want 600", got)
	}

	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("log kosong: %v", scanner.Err())
	}
	var record inboxClientDebugRecord
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	if record.AgentID != 7 || record.SessionID != "session-1" {
		t.Fatalf("scope record salah: %+v", record)
	}
	if record.Event != "composer.input-sample" {
		t.Fatalf("event = %q", record.Event)
	}
	if scanner.Scan() {
		t.Fatal("event kosong seharusnya tidak ditulis")
	}
}

func TestCleanInboxDebugLabelUsesRuneLimit(t *testing.T) {
	if got := cleanInboxDebugLabel("  ab\n🙂cd  ", 4); got != "ab🙂c" {
		t.Fatalf("clean label = %q, want %q", got, "ab🙂c")
	}
}
