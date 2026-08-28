# Ruangkirim — Operations Checklist

Dokumen ini menjadi checkpoint operasional untuk pemisahan Ruangkirim dari ChatLoop dan deployment production secara bertahap.

> **Current checkpoint:** Phase 1G.6C.16 — production binary installed, systemd service belum dibuat.
>
> **Do not continue to Phase 1G.6C.17** until the current checkpoint has been verified locally and the checklist is updated.

## Before Clone

- [ ] ChatLoop is healthy
- [ ] stable commit identified
- [ ] working tree reviewed
- [ ] backups available
- [ ] repository access verified

## After Clone

- [x] new VPS directory exists: `/var/www/ruangkirim`
- [x] `.git` is newly initialized / repository isolated
- [x] no production `.env` copied
- [x] no production WhatsApp session copied
- [x] no production ChatLoop database copied
- [x] frontend dependencies install cleanly
- [x] frontend build passes
- [x] backend tests pass
- [x] backend build passes

## Phase 1F — Branding / Frontend Identity

- [x] Ruangkirim logo source validated
- [x] required Ruangkirim logo asset filenames normalized
- [x] active frontend logo imports resolve
- [x] frontend production build passes
- [x] targeted `CHATLOOP_*` frontend identity references removed from audited files
- [ ] full repository-wide legacy branding audit completed
- [ ] final visual QA completed on desktop and mobile

## Phase 1G — Infrastructure Isolation

### 1G.4–1G.5 — Fresh Environment

- [x] fresh `.env` created from `.env.example`
- [x] `.env` permission is `600`
- [x] `DB_HOST=127.0.0.1`
- [x] `DB_PORT=3306`
- [x] `DB_USER=ruangkirim`
- [x] `DB_NAME=ruangkirim`
- [x] `JWT_SECRET` generated
- [x] `SUPERADMIN_USERNAME` configured
- [x] `SUPERADMIN_PASSWORD` configured
- [x] `PORT=3031` configured

### 1G.6B — WhatsApp Storage Isolation

- [x] dedicated storage root created: `/var/lib/ruangkirim/whatsapp`
- [x] storage ownership is `ubuntu:ubuntu`
- [x] storage permissions are restricted (`750`)
- [x] Ruangkirim session paths use `/var/lib/ruangkirim/whatsapp`
- [x] old relative `data/wa-session-agent-*` paths removed from `backend/services/wa.go`
- [x] old `./wa-assistant.db` session path removed from `backend/services/wa.go`
- [x] ChatLoop WhatsApp session files remain under `/var/www/chatloop/data`
- [x] Ruangkirim storage remained empty during isolation verification

### 1G.6C — Database / Runtime Isolation

- [x] fresh MySQL database `ruangkirim` created
- [x] dedicated MySQL user `ruangkirim` verified
- [x] application database contains 45 tables after migration/seed
- [x] seed data verified (`superadmin`, `Default`, `CS Utama`)
- [x] `follow_ups` schema audited; no invalid `sender` assumption remains
- [x] login throttle schema audited
- [x] `LoginThrottle.LockedUntil` changed to nullable `*time.Time`
- [x] login throttle insert no longer produces MySQL zero-date error
- [x] backend `go test ./...` passes
- [x] backend production binary builds successfully
- [x] ChatLoop remains isolated on port `3030`
- [x] Ruangkirim runtime verified independently on port `3031`
- [x] HTTP root smoke test returns `200`
- [x] invalid login smoke test returns `401`
- [x] login throttle rows are written with `locked_until=NULL` when not locked
- [x] no Ruangkirim WhatsApp session files created during pre-login runtime test
- [x] production binary installed at `/var/www/ruangkirim/ruangkirim-server`
- [x] installed binary checksum matches validated build artifact

## Database

- [x] fresh database
- [x] migrations pass
- [x] seeds pass
- [x] application reads/writes
- [x] no ChatLoop production data copied into Ruangkirim database
- [x] database user verified over `127.0.0.1`

## Runtime

- [ ] systemd service active
- [ ] worker active
- [ ] scheduler active if required
- [ ] Redis/cache isolated
- [ ] queue isolated
- [x] application database isolated
- [x] storage isolated
- [ ] logs isolated
- [x] WhatsApp session storage isolated
- [x] ChatLoop remains active on `3030`
- [x] Ruangkirim test runtime uses `3031`
- [ ] Ruangkirim production runtime managed by systemd
- [ ] reboot persistence verified

## Production Deployment

- [ ] `/etc/systemd/system/ruangkirim.service` created and reviewed
- [ ] systemd daemon reloaded
- [ ] `ruangkirim.service` enabled
- [ ] `ruangkirim.service` started successfully
- [ ] service restart behavior verified
- [ ] service logs verified with `journalctl`
- [ ] Nginx independent
- [ ] domain correct
- [ ] TLS correct
- [ ] health endpoint/application verified
- [ ] login verified with valid credentials
- [ ] core workflow verified
- [ ] WhatsApp QR/pairing verified
- [ ] WhatsApp session file creation verified under `/var/lib/ruangkirim/whatsapp`
- [ ] ChatLoop WhatsApp sessions verified untouched
- [ ] rollback binary/commit recorded

## Safety Rules During Phase 1G

- Do not copy ChatLoop production `.env` into Ruangkirim.
- Do not copy ChatLoop MySQL data into the Ruangkirim database.
- Do not copy ChatLoop WhatsApp session files into Ruangkirim.
- Do not reuse ChatLoop port `3030`; Ruangkirim uses `3031`.
- Do not delete or modify `/var/www/chatloop/data/wa-session-agent-*` as part of Ruangkirim migration.
- Do not replace `chatloop.service` while installing `ruangkirim.service`.
- Do not run both applications against the same WhatsApp session SQLite files.
- Do not make production systemd changes until the manual runtime checkpoint has passed.

## Current Runtime Snapshot

```text
ChatLoop
  service: chatloop.service
  binary: /var/www/chatloop/chatloop-server
  port: 3030
  status: active

Ruangkirim
  binary: /var/www/ruangkirim/ruangkirim-server
  port: 3031
  systemd: not yet created

Database
  MySQL database: ruangkirim
  tables: 45

WhatsApp storage
  Ruangkirim: /var/lib/ruangkirim/whatsapp
  ChatLoop:   /var/www/chatloop/data
```

## Final Audit

```bash
git status --short
git log -1 --oneline
git diff --check
```

Also perform a repository-wide review for unintended legacy branding, leaked secrets, shared runtime paths, shared ports, shared databases, and shared WhatsApp session storage.
