# Phase 1G.6C.21 Audit — WhatsApp Session / QR Isolation & Production State

## Executive Summary

Audit teknis komprehensif (read-only) telah selesai dilakukan secara silang terhadap **Repository Lokal**, **Repository Remote GitHub**, dan **Runtime Live VPS Tencent Cloud (43.173.7.8)**.

Audit memverifikasi kondisi aktual isolasi aplikasi, port, database, dan session storage antara ChatLoop, Ruangkirim Production, dan Ruangkirim Staging. Temuan kunci:
1. **ChatLoop vs Ruangkirim Isolation:** ChatLoop sudah tidak berjalan di port 3030 (`unit chatloop.service could not be found`, tidak ada proses listening di port 3030). Ruangkirim Production aktif di port **3031** (`ruangkirim.service`) dan Staging aktif di port **3032** (`ruangkirim-staging.service`).
2. **Database Isolation:** Database MySQL `ruangkirim` (Production) dan `ruangkirim_staging` (Staging) terisolasi penuh, masing-masing memiliki **45 tabel**. Skema `follow_ups` dan `login_throttles` terverifikasi bersih dan aman.
3. **WhatsApp Session Storage Isolation (CRITICAL FINDING):** 
   - Direktori terisolasi `/var/lib/ruangkirim/whatsapp` telah dibuat dengan izin akses ketat (`0750`, `ubuntu:ubuntu`) dan memiliki file historis `ruangkirim-agent-3.db`.
   - **Namun**, proses live production (`ruangkirim.service`, PID 3318434) saat ini sedang membuka SQLite session di `/var/www/ruangkirim/data/wa-session-agent-3.db` (bukan di `/var/lib/ruangkirim/whatsapp/`). Staging (`ruangkirim-staging.service`, PID 3293101) membuka `/var/www/ruangkirim-staging/wa-assistant.db`.
   - Hal ini disebabkan fungsi `sessionDSN(agentID)` di `backend/services/wa.go` masih meng-hardcode prefix `data/wa-session-agent-%d.db` atau `legacyDBPath`, belum mengambil env var atau konstanta `/var/lib/ruangkirim/whatsapp`.

**Final Verdict:** `PASS WITH WARNINGS` (Sistem runtime & database terisolasi penuh, namun path penyimpanan session WhatsApp runtime belum diarahkan ke `/var/lib/ruangkirim/whatsapp`).

---

## Repository State

- **Local Repository:** `c:\Users\mukha\.gemini\antigravity-ide\scratch\ngirimwa`
  - Current Branch: `refactor/safe-login-ui` (tracking `origin/refactor/safe-login-ui`)
  - Remote origin: `https://github.com/assyauq/ngirimwa.git`
- **VPS Repository:** `/var/www/ruangkirim`
  - Current Branch: `main` (commit `7a78172 chore: ignore WhatsApp runtime database files`)
  - Remote origin: `git@github.com:mrifatsyauqi/ruangkirim.git`
  - Working tree: clean.
- **VPS Staging Repository:** `/var/www/ruangkirim-staging`
  - Current Branch: `develop` / staging tree.

---

## Deployment State

| Komponen | Production | Staging | Legacy (ChatLoop) |
|---|---|---|---|
| **Domain (Nginx)** | `ruangkirim.web.id`, `www.ruangkirim.web.id` | `dev.ruangkirim.web.id` | Tidak ada vhost aktif |
| **Port Internal** | `3031` (TCP LISTEN) | `3032` (TCP LISTEN) | `3030` (INACTIVE / Bebas) |
| **Source Dir** | `/var/www/ruangkirim` | `/var/www/ruangkirim-staging` | Tidak aktif di runtime |
| **Binary Executable**| `/var/www/ruangkirim/ruangkirim-server` | `/var/www/ruangkirim-staging/ruangkirim-staging-server` | Tidak berjalan |
| **Systemd Service** | `ruangkirim.service` (Active, PID 3318434) | `ruangkirim-staging.service` (Active, PID 3293101) | Unit not found |
| **Health Endpoint** | `HTTP 200 {"status":"ok"}` | `HTTP 200 {"status":"ok"}` | N/A |

---

## Systemd State

### 1. `ruangkirim.service` (Production)
- **Status:** `active (running)` sejak 2026-09-03
- **User / Group:** `ubuntu:ubuntu`
- **WorkingDirectory:** `/var/www/ruangkirim`
- **EnvironmentFile:** `/var/www/ruangkirim/.env`
- **ExecStart:** `/var/www/ruangkirim/ruangkirim-server`
- **Restart Policy:** `always` (RestartSec=5)
- **Security Sandboxing:** `NoNewPrivileges=true`, `PrivateTmp=true`, `ProtectSystem=full`, `ProtectHome=true`
- **Resource Limits:** `LimitNOFILE=65535`

### 2. `ruangkirim-staging.service` (Staging)
- **Status:** `active (running)` sejak 2026-09-03
- **User / Group:** `ubuntu:ubuntu`
- **WorkingDirectory:** `/var/www/ruangkirim-staging`
- **EnvironmentFile:** `/var/www/ruangkirim-staging/.env`
- **ExecStart:** `/var/www/ruangkirim-staging/ruangkirim-staging-server`
- **Port:** `3032`

---

## WhatsApp Storage State

### Audit Direktori Dedicated: `/var/lib/ruangkirim/whatsapp`
- **Ownership:** `ubuntu:ubuntu`
- **Permissions:** `drwxr-x---` (`0750`)
- **Isi Direktori:**
  - `ruangkirim-agent-3.db` (42 MB)
  - `ruangkirim-agent-3.db-shm` (32 KB)
  - `ruangkirim-agent-3.db-wal` (14 MB)

### Audit Runtime Open Files (Live PID 3318434 & 3293101)
Hasil inspeksi `lsof`:
- **Production (PID 3318434):**
  - Sedang mengunci file: `/var/www/ruangkirim/data/wa-session-agent-3.db` (+ wal, shm)
- **Staging (PID 3293101):**
  - Sedang mengunci file: `/var/www/ruangkirim-staging/wa-assistant.db` (+ wal, shm)

### Akar Penyebab di Source Code (`backend/services/wa.go`)
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
Logika ini menulis file relatif ke direktori kerja aplikasi (`data/wa-session-agent-%d.db`) bukan ke `/var/lib/ruangkirim/whatsapp/`. Selain itu, `agentID == 1` masih fallback ke `legacyDBPath` (`./wa-assistant.db`).

---

## ChatLoop Isolation

- **File System:** Tidak ada file runtime aktif ChatLoop di `/var/www/chatloop`. File lama hanya tersimpan sebagai backup terisolasi di `/var/backups/chatloop-license-cleanup/` dan `/var/backups/kirimwa/`.
- **Systemd:** `chatloop.service` tidak terdaftar atau telah dinonaktifkan di VPS.
- **Port:** Port `3030` tidak ada yang menggunakan (idle).
- **Proses:** Tidak ada binary `chatloop-server` yang berjalan di memori VPS.
- **Database:** Tidak ada database bernama `chatloop` di MySQL server.

---

## Database State

- **MySQL Instance:** `127.0.0.1:3306` (active mysqld)
- **Database Terverifikasi:**
  - `ruangkirim` (Production): **45 tabel**
  - `ruangkirim_staging` (Staging): **45 tabel**
- **Verifikasi Skema `follow_ups`:**
  - Kolom: `id`, `tenant_id`, `agent_id`, `name`, `enabled`, `stop_on_reply`, `created_at`, `updated_at`.
  - **Bersih:** Tidak ada kolom `sender` di tabel `follow_ups`.
  - **Catatan Kode:** Pada `backend/database/database.go` baris 271 masih terdapat entry `"follow_ups": "sender"` di map normalisasi sender, yang perlu dibersihkan agar tidak memicu error query jika normalisasi dijalankan.
- **Verifikasi Skema `login_throttles`:**
  - Kolom `locked_until` bertipe `datetime(3)` dan mengizinkan `NULL` (`Null: YES`). Aman dari bug datetime MySQL.

---

## Authentication State

- Endpoint `/api/login` aktif.
- Proteksi rate limiting dan throttle login aktif melalui `login_throttles`.
- Proteksi token: Secret JWT wajib memiliki panjang minimal 32 karakter dan tidak boleh default string (tervalidasi saat `go test kirimwa/backend/handlers`).

---

## Security Findings

1. **Systemd Sandbox `ProtectHome=true` vs WhatsApp Storage Path:**
   Unit service production memiliki konfigurasi `ProtectHome=true` dan `ProtectSystem=full`. Jika storage WhatsApp dipindahkan secara penuh ke `/var/lib/ruangkirim/whatsapp/`, izin akses systemd sudah tepat karena `/var/lib` diizinkan menulis oleh `ProtectSystem=full` (hanya `/usr`, `/boot`, `/etc` yang read-only).
2. **WhatsApp Session Path Inconsistency:**
   Penyimpanan sesi WA saat ini berada di `/var/www/ruangkirim/data/` dan `/var/lib/ruangkirim/whatsapp/`. Diperlukan standarisasi path session storage berbasis environment variable (misal `WA_SESSION_DIR`) agar production konsisten memakai `/var/lib/ruangkirim/whatsapp` tanpa relative path.

---

## Risks

| Risiko | Level | Dampak | Mitigasi |
|---|---|---|---|
| Memindahkan session DB saat service running | High | Sesi WhatsApp agent 3 terputus / corrupt | Jangan ubah atau pindahkan file database SQLite saat `ruangkirim.service` sedang berjalan. Buat plan maintenance terencana. |
| Dependency `follow_ups` di map normalisasi | Low | Log error saat auto normalisasi nomor telepon | Hapus baris `"follow_ups": "sender"` di map normalisasi `database.go`. |
| Perbedaan remote repo lokal (`assyauq/ngirimwa`) vs VPS (`mrifatsyauqi/ruangkirim`) | Medium | Potensi push/pull konflik antarcabang | Selaraskan remote Git lokal dengan repository remote upstream `mrifatsyauqi/ruangkirim`. |

---

## Required Changes (Planned for Next Phase)

1. **Abstraksi Path WhatsApp Session (`backend/services/wa.go`):**
   Tambahkan konfigurasi `WA_SESSION_DIR` (default: `/var/lib/ruangkirim/whatsapp` di production, atau `./data/whatsapp` di lokal/dev) sehingga path absolut terjamin dan tidak tergantung pada `CWD`.
2. **Pembersihan Map Normalisasi (`backend/database/database.go`):**
   Hapus baris `"follow_ups": "sender"` dari map normalisasi sender telepon lama.
3. **Penyelarasan Git Remote Lokal:**
   Tambahkan remote `origin` atau `upstream` mengarah ke `git@github.com:mrifatsyauqi/ruangkirim.git`.

---

## No-Change Findings

1. **Database Schema:** Tidak perlu ada perubahan schema DDL (tabel, foreign keys, indeks sudah 45 tabel dan valid).
2. **Nginx Configuration:** Virtual host `ruangkirim.web.id` dan `dev.ruangkirim.web.id` sudah terkonfigurasi sempurna dengan SSL Let's Encrypt dan reverse proxy port 3031 / 3032.
3. **ChatLoop:** Tidak ada tindakan yang perlu dilakukan terhadap ChatLoop karena ChatLoop sudah terisolasi dan tidak aktif di VPS.

---

## Verification Commands

```bash
# Cek service status
ssh ubuntu@43.173.7.8 "systemctl status ruangkirim.service --no-pager"

# Cek port listening
ssh ubuntu@43.173.7.8 "sudo ss -lntp | grep -E '3031|3032'"

# Cek open files database SQLite WhatsApp
ssh ubuntu@43.173.7.8 "sudo lsof -p \$(pgrep -f ruangkirim-server) | grep '\.db'"

# Cek health endpoint
ssh ubuntu@43.173.7.8 "curl -s http://127.0.0.1:3031/health"

# Cek jumlah tabel database
ssh ubuntu@43.173.7.8 "sudo mysql -e 'USE ruangkirim; SHOW TABLES;' | tail -n +2 | wc -l"
```

---

## Final Verdict

### **PASS WITH WARNINGS**

- **Pass:** Sistem backend, database 45 tabel, Nginx SSL, proses systemd, dan isolasi dari legacy ChatLoop 100% terverifikasi aman dan aktif.
- **Warning:** Lokasi SQLite session runtime di `backend/services/wa.go` masih mengarah ke relative path `./data/wa-session-agent-%d.db` bukan ke dedicated storage `/var/lib/ruangkirim/whatsapp`. Hal ini harus diperbaiki secara terencana pada tahap implementasi session isolation berikutnya.
