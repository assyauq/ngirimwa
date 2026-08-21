package handlers

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// StartMediaCleanup menghapus file media lama secara berkala agar disk VPS tidak penuh.
func StartMediaCleanup(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	run := func() { safeRun("cleanupMedia", func() { cleanupMedia(retentionDays) }) }
	go func() {
		run()
		t := time.NewTicker(24 * time.Hour)
		for range t.C {
			run()
		}
	}()
}

func cleanupMedia(days int) {
	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0
	filepath.WalkDir("data/media", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil && info.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	if removed > 0 {
		log.Printf("Media cleanup: %d file lama dihapus (> %d hari)", removed, days)
	}
}

// CleanupBroadcastJunk menghapus sisa thread sistem WhatsApp (status@broadcast /
// Status-Story, serta @newsletter) yang sempat tersimpan sebelum filter jalur
// live aktif. Sender berdomain ini bukan nomor pelanggan, jadi aman dibersihkan
// dari riwayat chat, read-state Inbox, dan CRM kontak. Dijalankan sekali saat
// startup; idempoten.
func CleanupBroadcastJunk() {
	const (
		broadcast  = "%@broadcast"
		newsletter = "%@newsletter"
	)

	if res := database.DB.Where("sender LIKE ? OR sender LIKE ?", broadcast, newsletter).
		Delete(&models.ChatHistory{}); res.Error != nil {
		log.Printf("Cleanup broadcast junk (chat_histories) gagal: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("Cleanup broadcast junk: %d baris chat_histories sistem dihapus", res.RowsAffected)
	}

	if res := database.DB.Where("sender LIKE ? OR sender LIKE ?", broadcast, newsletter).
		Delete(&models.InboxReadState{}); res.Error != nil {
		log.Printf("Cleanup broadcast junk (inbox_read_states) gagal: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("Cleanup broadcast junk: %d baris inbox_read_states sistem dihapus", res.RowsAffected)
	}

	// Kontak sistem (mis. Number = "status@broadcast") adalah polusi CRM best-effort:
	// bila ada FK yang menghalangi, error hanya dicatat dan Inbox tetap bersih.
	if res := database.DB.Where("number LIKE ? OR number LIKE ?", broadcast, newsletter).
		Delete(&models.Contact{}); res.Error != nil {
		log.Printf("Cleanup broadcast junk (contacts) gagal: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("Cleanup broadcast junk: %d kontak sistem dihapus", res.RowsAffected)
	}
}

// CleanupOrphanAssignments menghapus relasi CS→agent yang yatim: baris
// user_agent_assignments yang agent_id-nya sudah tidak ada di tabel agents.
// Kondisi ini muncul bila sebuah nomor WhatsApp pernah dihapus sebelum cascade
// cleanup aktif, dan menyebabkan form CS memunculkan "Nomor X" tanpa checkbox
// serta memblokir penyimpanan. Dijalankan sekali saat startup; idempoten.
func CleanupOrphanAssignments() {
	res := database.DB.Where("agent_id NOT IN (?)",
		database.DB.Model(&models.Agent{}).Select("id"),
	).Delete(&models.UserAgentAssignment{})
	if res.Error != nil {
		log.Printf("Cleanup orphan assignment gagal: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("Cleanup orphan assignment: %d relasi CS yatim dihapus", res.RowsAffected)
	}
}
