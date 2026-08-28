package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	inboxClientDebugMaxBody    = 256 << 10
	inboxClientDebugMaxEntries = 100
)

var inboxClientDebugMu sync.Mutex

type inboxClientDebugEntry struct {
	At        string                 `json:"at"`
	ElapsedMS int64                  `json:"elapsed_ms"`
	Event     string                 `json:"event"`
	Details   map[string]interface{} `json:"details"`
}

type inboxClientDebugRequest struct {
	SessionID string                  `json:"session_id"`
	Entries   []inboxClientDebugEntry `json:"entries"`
}

type inboxClientDebugRecord struct {
	ServerAt  time.Time              `json:"server_at"`
	AgentID   uint                   `json:"agent_id"`
	SessionID string                 `json:"session_id"`
	ClientAt  string                 `json:"client_at,omitempty"`
	ElapsedMS int64                  `json:"elapsed_ms"`
	Event     string                 `json:"event"`
	Details   map[string]interface{} `json:"details,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
}

func cleanInboxDebugLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > max {
		value = string(runes[:max])
	}
	return value
}

func appendInboxClientDebugRecords(
	logPath string,
	agentID uint,
	sessionID string,
	userAgent string,
	entries []inboxClientDebugEntry,
) (stored int, err error) {
	if err = os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	encoder := json.NewEncoder(file)
	for _, entry := range entries {
		event := cleanInboxDebugLabel(entry.Event, 96)
		if event == "" {
			continue
		}
		record := inboxClientDebugRecord{
			ServerAt:  time.Now(),
			AgentID:   agentID,
			SessionID: sessionID,
			ClientAt:  cleanInboxDebugLabel(entry.At, 48),
			ElapsedMS: entry.ElapsedMS,
			Event:     event,
			Details:   entry.Details,
			UserAgent: userAgent,
		}
		if err = encoder.Encode(record); err != nil {
			return stored, err
		}
		stored++
	}
	if err = file.Sync(); err != nil {
		return stored, err
	}
	return stored, nil
}

// InboxClientDebug menerima diagnostik performa dari browser yang sudah login.
// Payload dibatasi ketat dan tidak pernah berisi isi draft/pesan.
func InboxClientDebug(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, inboxClientDebugMaxBody)
	var req inboxClientDebugRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Log diagnostik tidak valid"})
		return
	}
	if len(req.Entries) == 0 {
		c.Status(http.StatusNoContent)
		return
	}
	if len(req.Entries) > inboxClientDebugMaxEntries {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Terlalu banyak entri diagnostik"})
		return
	}

	sessionID := cleanInboxDebugLabel(req.SessionID, 80)
	if sessionID == "" {
		sessionID = "unknown"
	}
	userAgent := cleanInboxDebugLabel(c.GetHeader("User-Agent"), 300)
	logPath := filepath.Join(".tmp", "inbox-debug.jsonl")

	inboxClientDebugMu.Lock()
	stored, err := appendInboxClientDebugRecords(logPath, agentID, sessionID, userAgent, req.Entries)
	inboxClientDebugMu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Log diagnostik belum dapat disimpan"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"stored": stored})
}
