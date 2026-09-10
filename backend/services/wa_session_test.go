package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWASession_ConfiguredDirectory menguji penentuan path ketika WA_SESSION_DIR diset secara eksplisit.
func TestWASession_ConfiguredDirectory(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	resolved := ResolveSessionDir()
	if resolved != filepath.Clean(tempDir) {
		t.Fatalf("expected resolved dir %s, got %s", tempDir, resolved)
	}

	p1 := SessionFilePath(1)
	expectedP1 := filepath.Join(tempDir, "wa-session-agent-1.db")
	if p1 != expectedP1 {
		t.Errorf("agent 1 path mismatch: expected %s, got %s", expectedP1, p1)
	}

	p3 := SessionFilePath(3)
	expectedP3 := filepath.Join(tempDir, "wa-session-agent-3.db")
	if p3 != expectedP3 {
		t.Errorf("agent 3 path mismatch: expected %s, got %s", expectedP3, p3)
	}
}

// TestWASession_DevFallback menguji fallback default aman pada mode development jika WA_SESSION_DIR kosong.
func TestWASession_DevFallback(t *testing.T) {
	SetSessionDirForTesting("")
	t.Setenv("WA_SESSION_DIR", "")
	t.Setenv("APP_ENV", "development")

	resolved := ResolveSessionDir()
	expected := filepath.Clean("./data/whatsapp")
	if resolved != expected {
		t.Fatalf("expected dev fallback %s, got %s", expected, resolved)
	}
}

// TestWASession_ProductionMissingConfig menguji deteksi kegagalan konfigurasi di mode production.
func TestWASession_ProductionMissingConfig(t *testing.T) {
	SetSessionDirForTesting("")
	t.Setenv("WA_SESSION_DIR", "")
	t.Setenv("APP_ENV", "production")

	err := CheckSessionDirConfig()
	if err == nil {
		t.Fatal("expected error in production when WA_SESSION_DIR is missing, got nil")
	}
	if !strings.Contains(err.Error(), "WA_SESSION_DIR wajib diset") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestWASession_MultipleAgentsIsolation menguji isolasi path antar agen (Agent 1, 2, 3, N).
func TestWASession_MultipleAgentsIsolation(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	agentIDs := []uint{1, 2, 3, 10, 42}
	paths := make(map[uint]string)

	for _, id := range agentIDs {
		path := SessionFilePath(id)
		expected := filepath.Join(tempDir, "wa-session-agent-"+strings.TrimPrefix(filepath.Base(path), "wa-session-agent-"))
		if path != expected {
			t.Errorf("agent %d path mismatch: expected %s, got %s", id, expected, path)
		}

		if _, exists := paths[id]; exists {
			t.Fatalf("duplicate path generated for agent %d", id)
		}
		paths[id] = path

		// Pastikan file path berbeda untuk tiap agent
		for otherID, otherPath := range paths {
			if otherID != id && otherPath == path {
				t.Fatalf("agent %d collision with agent %d: path %s", id, otherID, path)
			}
		}
	}
}

// TestWASession_DSNParameters menguji kelengkapan parameter DSN SQLite (WAL mode, busy timeout, foreign keys).
func TestWASession_DSNParameters(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	dsn := sessionDSN(3)
	expectedPath := filepath.Join(tempDir, "wa-session-agent-3.db")

	if !strings.HasPrefix(dsn, "file:"+expectedPath) {
		t.Errorf("DSN missing correct file prefix, got: %s", dsn)
	}
	if !strings.Contains(dsn, "_foreign_keys=on") {
		t.Errorf("DSN missing _foreign_keys=on: %s", dsn)
	}
	if !strings.Contains(dsn, "_journal_mode=WAL") {
		t.Errorf("DSN missing _journal_mode=WAL: %s", dsn)
	}
	if !strings.Contains(dsn, "_busy_timeout=5000") {
		t.Errorf("DSN missing _busy_timeout=5000: %s", dsn)
	}
}

// TestWASession_DirectoryCreation menguji pembuatan direktori otomatis jika belum ada.
func TestWASession_DirectoryCreation(t *testing.T) {
	baseDir := t.TempDir()
	nestedDir := filepath.Join(baseDir, "nested", "storage", "whatsapp")
	SetSessionDirForTesting(nestedDir)
	defer SetSessionDirForTesting("")

	path := SessionFilePath(2)
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("expected directory to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected path to be a directory")
	}

	expectedFile := filepath.Join(nestedDir, "wa-session-agent-2.db")
	if path != expectedFile {
		t.Errorf("expected %s, got %s", expectedFile, path)
	}
}

// TestWASession_Agent1LegacyCompatibility menguji kompatibilitas file legacy wa-assistant.db untuk Agent 1.
func TestWASession_Agent1LegacyCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	// Skenario A: belum ada wa-session-agent-1.db, tapi ada wa-assistant.db di dalam dir sesi
	legacyFile := filepath.Join(tempDir, "wa-assistant.db")
	if err := os.WriteFile(legacyFile, []byte("legacy-data"), 0640); err != nil {
		t.Fatalf("failed to create dummy legacy file: %v", err)
	}

	p := SessionFilePath(1)
	if p != legacyFile {
		t.Errorf("expected legacy fallback to %s, got %s", legacyFile, p)
	}

	// Skenario B: setelah wa-session-agent-1.db dibuat, harus memprioritaskan wa-session-agent-1.db
	newAgent1File := filepath.Join(tempDir, "wa-session-agent-1.db")
	if err := os.WriteFile(newAgent1File, []byte("new-data"), 0640); err != nil {
		t.Fatalf("failed to create new agent1 file: %v", err)
	}

	pNew := SessionFilePath(1)
	if pNew != newAgent1File {
		t.Errorf("expected prioritized %s, got %s", newAgent1File, pNew)
	}
}

// TestWASession_HistoricalRuangkirimNamingCompatibility menguji kompatibilitas nama ruangkirim-agent-N.db.
func TestWASession_HistoricalRuangkirimNamingCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	// Buat dummy ruangkirim-agent-3.db
	histFile := filepath.Join(tempDir, "ruangkirim-agent-3.db")
	if err := os.WriteFile(histFile, []byte("historical-agent3"), 0640); err != nil {
		t.Fatalf("failed to create dummy historical file: %v", err)
	}

	p := SessionFilePath(3)
	if p != histFile {
		t.Errorf("expected historical fallback to %s, got %s", histFile, p)
	}
}

// TestWASession_RemoveWAFiles menguji pembersihan file sesi (db, wal, shm) saat RemoveWA dipanggil.
func TestWASession_RemoveWAFiles(t *testing.T) {
	tempDir := t.TempDir()
	SetSessionDirForTesting(tempDir)
	defer SetSessionDirForTesting("")

	agentID := uint(5)
	dbFile := filepath.Join(tempDir, "wa-session-agent-5.db")
	walFile := filepath.Join(tempDir, "wa-session-agent-5.db-wal")
	shmFile := filepath.Join(tempDir, "wa-session-agent-5.db-shm")

	_ = os.WriteFile(dbFile, []byte("db"), 0640)
	_ = os.WriteFile(walFile, []byte("wal"), 0640)
	_ = os.WriteFile(shmFile, []byte("shm"), 0640)

	RemoveWA(agentID)

	for _, f := range []string{dbFile, walFile, shmFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be deleted after RemoveWA", f)
		}
	}
}
