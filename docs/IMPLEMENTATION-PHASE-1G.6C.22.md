# Phase 1G.6C.22 Implementation Plan — WhatsApp Session Storage Isolation

## 1. Current State

Berdasarkan audit komprehensif read-only (Phase 1G.6C.21):
- **Production VPS:**
  - `ruangkirim.service` aktif di port 3031.
  - Membuka database session WhatsApp di `/var/www/ruangkirim/data/wa-session-agent-3.db` (+ wal, shm).
  - Direktori dedicated `/var/lib/ruangkirim/whatsapp` sudah ada (`0750 ubuntu:ubuntu`), namun belum digunakan oleh proses runtime aktif.
  - Database `ruangkirim` memiliki 45 tabel dengan Agent ID `3` (Nomor `6282211700060`).
- **Staging VPS:**
  - `ruangkirim-staging.service` aktif di port 3032.
  - Membuka database session WhatsApp di `/var/www/ruangkirim-staging/wa-assistant.db` (+ wal, shm).
  - Database `ruangkirim_staging` memiliki 45 tabel dengan Agent ID `1` (Nomor `6282211700060`).
- **Database Code Anomaly:**
  - Di `backend/database/database.go` baris 236 dan 271 terdapat sisa query normalisasi ke tabel `follow_ups` kolom `sender`, padahal tabel `follow_ups` tidak memiliki kolom `sender`.

---

## 2. Root Cause

1. **Path Hardcoded & Dependen pada CWD di `backend/services/wa.go`:**
   ```go
   func sessionDSN(agentID uint) string {
       path := legacyDBPath
       if agentID != 1 {
           os.MkdirAll("data", 0o755)
           path = fmt.Sprintf("data/wa-session-agent-%d.db", agentID)
       }
       return "file:" + path + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
   }
   ```
   - Ketika `agentID == 1`, fungsi selalu mengambil `legacyDBPath` (`./wa-assistant.db`).
   - Ketika `agentID != 1`, fungsi membuat dan mengambil path relatif `data/wa-session-agent-%d.db`.
   - Tidak ada mekanisme pembacaan path direktori dari environment variable, sehingga runtime bergantung sepenuhnya pada `CWD` proses binary.
2. **`RemoveWA(agentID uint)` juga meng-hardcode path relatif:**
   ```go
   base := fmt.Sprintf("data/wa-session-agent-%d.db", agentID)
   ```
3. **Pembersihan Database Belum Tuntas di Kode:**
   - Map `normalizeSenderFields()` di `backend/database/database.go` masih mengasumsikan `follow_ups` memiliki kolom `sender`.

---

## 3. Files That Will Change

1. **`backend/services/wa.go`:**
   - Tambahkan variabel package-level `sessionDir string`.
   - Modifikasi `InitWA(dbPath string)` untuk membaca `os.Getenv("WA_SESSION_DIR")` jika diset.
   - Tambahkan fungsi helper `SetSessionDir(dir string)` untuk unit test dan konfigurasi dinamis.
   - Refactor `sessionDSN(agentID uint) string` dan `sessionFilePath(agentID uint) string` agar menggunakan `sessionDir` jika tersedia, dengan fallback aman jika tidak diset (backward-compatible).
   - Pastikan `RemoveWA(agentID uint)` menggunakan path dari `sessionFilePath(agentID)`.
2. **`backend/services/wa_session_test.go` (File Baru):**
   - Unit tests komprehensif menguji:
     - Default session directory (fallback).
     - Custom `WA_SESSION_DIR` absolut.
     - Agent 1 vs Agent > 1.
     - Multiple agents isolation (Agent 1, 2, 3, N).
     - SQLite DSN parameters (`_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000`).
     - Directory auto-creation dan error handling.
3. **`backend/database/database.go`:**
   - Hapus `"follow_ups"` dari slice tabel di baris 236.
   - Hapus `"follow_ups": "sender"` dari map `tables` di baris 271.
4. **`.env.example`:**
   - Dokumentasikan variabel `WA_SESSION_DIR` dengan panduan nilai untuk Production, Staging, dan Local Dev.
5. **Dokumentasi:**
   - Update `docs/OPERATIONS-CHECKLIST.md`.

---

## 4. Proposed Configuration Design

Variabel environment baru: **`WA_SESSION_DIR`**

- **Production:**
  `WA_SESSION_DIR=/var/lib/ruangkirim/whatsapp`
- **Staging:**
  `WA_SESSION_DIR=/var/lib/ruangkirim-staging/whatsapp`
- **Local Dev:**
  Default kosong atau `./data/whatsapp` (otomatis fallback ke `./data/wa-session-agent-%d.db` jika unset, sehingga tidak merusak flow developer lokal).

Resolusi konfigurasi:
- `InitWA(dbPath string)` membaca `os.Getenv("WA_SESSION_DIR")`.
- Jika `WA_SESSION_DIR` diset, nilai tersebut di-trim dan diubah menjadi path absolut atau direktori target.
- Direktori dipastikan ada via `os.MkdirAll(dir, 0750)` saat path diakses.

---

## 5. Proposed Session Path Design

Fungsi inti path resolution:
```go
func sessionFilePath(agentID uint) string {
    dir := strings.TrimSpace(sessionDir)
    if dir == "" {
        // Fallback backward-compatible jika WA_SESSION_DIR tidak dikonfigurasi
        if agentID == 1 {
            return legacyDBPath
        }
        _ = os.MkdirAll("data", 0755)
        return fmt.Sprintf("data/wa-session-agent-%d.db", agentID)
    }

    _ = os.MkdirAll(dir, 0750)
    target := filepath.Join(dir, fmt.Sprintf("wa-session-agent-%d.db", agentID))

    // Kompatibilitas Agent 1: Jika file spesifik agent-1 belum ada namun wa-assistant.db ada di direktori tersebut
    if agentID == 1 {
        legacyInDir := filepath.Join(dir, "wa-assistant.db")
        if _, err := os.Stat(target); os.IsNotExist(err) {
            if _, errLeg := os.Stat(legacyInDir); errLeg == nil {
                return legacyInDir
            }
        }
    }
    return target
}

func sessionDSN(agentID uint) string {
    path := sessionFilePath(agentID)
    return "file:" + path + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
}
```

---

## 6. Session Naming Compatibility

Pola nama file:
- Untuk semua agent: `wa-session-agent-{agentID}.db`
  - Agent 1: `wa-session-agent-1.db` (atau fallback `wa-assistant.db` jika ada)
  - Agent 2: `wa-session-agent-2.db`
  - Agent 3: `wa-session-agent-3.db`
- WAL & SHM:
  - SQLite secara otomatis membuat `{nama_file}.db-wal` dan `{nama_file}.db-shm`.
- Fungsi `RemoveWA` menghapus `{sessionFilePath(agentID)}` beserta akhiran `-wal` dan `-shm`.

---

## 7. Migration Strategy (High Level)

1. **Implementasi & Test Lokal:**
   - Buat perubahan kode dan pastikan semua unit test lulus 100%.
2. **Deploy ke STAGING:**
   - Siapkan direktori `/var/lib/ruangkirim-staging/whatsapp` (`0750 ubuntu:ubuntu`).
   - Salin file sesi staging `wa-assistant.db*` ke storage baru (atau jadikan `wa-session-agent-1.db`).
   - Set `WA_SESSION_DIR=/var/lib/ruangkirim-staging/whatsapp` di `/var/www/ruangkirim-staging/.env`.
   - Deploy binary staging dan restart `ruangkirim-staging.service`.
   - Verifikasi melalui `lsof` bahwa staging membuka file di `/var/lib/ruangkirim-staging/whatsapp/`.
3. **PRODUCTION MIGRATION (TERKUNCI):**
   - **TIDAK DIJALANKAN DI FASE INI.** Memerlukan persetujuan eksplisit.

---

## 8. Test Strategy

1. **Unit Testing (`backend/services/wa_session_test.go`):**
   - Test fallback behavior saat `WA_SESSION_DIR` kosong.
   - Test isolated path saat `WA_SESSION_DIR` diisi direktori kustom.
   - Test isolasi antar agent (Agent 1, Agent 2, Agent 3).
   - Test DSN options (WAL mode, foreign keys, busy timeout).
   - Test `RemoveWA` path handling.
2. **Regression Testing:**
   - `go test ./...`
   - `go vet ./...`
   - `go build ./...`
   - Pastikan tidak ada kegagalan test di package `handlers` (dengan test JWT secret aman), `database`, dan `services`.

---

## 9. Staging Deployment Strategy

1. **Persiapan VPS Staging:**
   ```bash
   sudo mkdir -p /var/lib/ruangkirim-staging/whatsapp
   sudo chown -R ubuntu:ubuntu /var/lib/ruangkirim-staging
   sudo chmod 0750 /var/lib/ruangkirim-staging /var/lib/ruangkirim-staging/whatsapp
   ```
2. **Migrasi File Sesi Staging (Jika Ada):**
   - Salin `/var/www/ruangkirim-staging/wa-assistant.db*` ke `/var/lib/ruangkirim-staging/whatsapp/wa-assistant.db*`.
3. **Konfigurasi Staging `.env`:**
   - Tambahkan `WA_SESSION_DIR=/var/lib/ruangkirim-staging/whatsapp` ke `/var/www/ruangkirim-staging/.env`.
4. **Build & Restart Staging:**
   - Build binary `ruangkirim-staging-server`.
   - Restart service: `sudo systemctl restart ruangkirim-staging.service`.
5. **Verifikasi Staging:**
   - `systemctl status ruangkirim-staging.service`
   - `curl -i http://127.0.0.1:3032/health`
   - `sudo lsof -p $(pgrep -f ruangkirim-staging-server) | grep '\.db'`
   - Pastikan open file descriptor mengarah ke `/var/lib/ruangkirim-staging/whatsapp/`.

---

## 10. Production Migration Strategy (APPROVAL REQUIRED)

> [!IMPORTANT]
> **PRODUKSI TIDAK AKAN DISENTUH ATAU DIMIGRASIKAN PADA FASE INI.**
> Bagian ini adalah rancangan operasional yang WAJIB menunggu persetujuan eksplisit pengguna.

Langkah-langkah terencana saat maintenance window disetujui:
1. **Stop Service:**
   `sudo systemctl stop ruangkirim.service`
2. **Pastikan Proses Berhenti & Checksum Terdata:**
   `sudo lsof -p $(pgrep -f ruangkirim-server) 2>/dev/null` (harus kosong).
3. **Backup File Sesi Aktif:**
   Buat backup lengkap `/var/www/ruangkirim/data/wa-session-agent-3.db*` ke folder backup terisolasi `/var/backups/ruangkirim/session-backup-$(date +%Y%m%d%H%M%S)/`.
4. **Pindahkan / Salin File Sesi ke Dedicated Storage:**
   Salin `wa-session-agent-3.db`, `wa-session-agent-3.db-wal`, `wa-session-agent-3.db-shm` ke `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db*`.
5. **Set Ownership & Permissions:**
   `sudo chown -R ubuntu:ubuntu /var/lib/ruangkirim/whatsapp`
   `sudo chmod 0750 /var/lib/ruangkirim/whatsapp`
   `sudo chmod 0640 /var/lib/ruangkirim/whatsapp/*`
6. **Update `.env` Production:**
   Tambahkan `WA_SESSION_DIR=/var/lib/ruangkirim/whatsapp`.
7. **Deploy Binary Baru & Start Service:**
   `sudo systemctl start ruangkirim.service`
8. **Verifikasi Lsof & Health Endpoint:**
   Pastikan PID baru membuka file di `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db` dan `/health` bernilai HTTP 200 OK.

---

## 11. Rollback Strategy

Jika terjadi kendala saat staging deployment:
1. Kembalikan `.env` staging (hapus baris `WA_SESSION_DIR`).
2. Rollback git commit staging.
3. Rebuild binary staging dan restart service `ruangkirim-staging.service`.
4. Staging akan kembali membuka file sesi di direktori sebelumnya.

---

## 12. Risks & Mitigations

| Risiko | Level | Mitigasi |
|---|---|---|
| SQLite WAL corrupted saat file disalin saat proses berjalan | Critical | JANGAN PERNAH menyalin database saat binary masih berjalan. Stop service terlebih dahulu sebelum menyalin file sesi. |
| Pengembang lokal tidak menset `WA_SESSION_DIR` | Low | Berikan fallback otomatis ke `./data/wa-session-agent-%d.db` jika `WA_SESSION_DIR` kosong. |
| Permintaan migrasi tanpa sengaja menyentuh ChatLoop | High | Isolasi ChatLoop sudah 100% diverifikasi. ChatLoop tidak aktif dan port 3030 tidak digunakan. |
| Production service terganggu saat staging di-deploy | High | Staging menggunakan port 3032, DB `ruangkirim_staging`, dan storage `/var/lib/ruangkirim-staging/`. Tidak ada sumber daya bersama dengan production. |

---

## 13. Verification Checklist

- [ ] Kode `sessionFilePath` dan `sessionDSN` terimplementasi di `backend/services/wa.go`.
- [ ] Obsolete `"follow_ups"` mapping dihapus dari `backend/database/database.go`.
- [ ] Unit tests baru di `backend/services/wa_session_test.go` lulus (`go test -v ./backend/services -run TestWASession`).
- [ ] `go test ./...` lulus di semua package.
- [ ] `go vet ./...` tidak menemukan masalah.
- [ ] `go build ./...` berhasil tanpa warning/error.
- [ ] Staging deployed dan terverifikasi via `lsof` menggunakan `/var/lib/ruangkirim-staging/whatsapp/`.
- [ ] Production service tetap berjalan normal tanpa interupsi.

---

## 14. Definition of Done

- [ ] Implementasi konfigurasi `WA_SESSION_DIR` selesai dan diuji secara lokal.
- [ ] Mapping `follow_ups.sender` dibersihkan dari `database.go`.
- [ ] Seluruh unit test lokal lulus (`go test ./...`).
- [ ] Staging VPS berhasil di-deploy dan terverifikasi menggunakan dedicated session storage.
- [ ] Production migration plan terdokumentasi dan terisolasi, menunggu persetujuan eksplisit.
- [ ] Dokumentasi `OPERATIONS-CHECKLIST.md` diperbarui.
