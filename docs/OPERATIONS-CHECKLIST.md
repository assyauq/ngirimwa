# Ruangkirim — Operations Checklist

Dokumen ini menjadi checkpoint operasional untuk pemisahan Ruangkirim dari ChatLoop dan deployment production secara bertahap.

> **Current checkpoint:** Phase 1G.6C.20 — systemd production runtime active and HTTP smoke test passed.
>
> **Do not continue to the WhatsApp session/QR phase** until the current checkpoint has been verified locally and the checklist is updated.

## Before Clone

- [ ] ChatLoop is healthy
- [ ] stable commit identified
- [ ] working tree reviewed
- [ ] backups available
- [ ] repository access verified

## After Clone

- [x] new VPS directory exists: `/var/www/ruangkirim`
- [x] repository is isolated from ChatLoop
- [x] no production `.env` copied
- [x] no production WhatsApp session copied
- [x] no production ChatLoop database copied
- [x] frontend dependencies install cleanly
- [x] frontend build passes
- [x] backend tests pass
- [x] backend production binary builds

## Phase 1G.6 — WhatsApp Session Storage

- [x] `/var/lib/ruangkirim/whatsapp` created
- [x] storage ownership is `ubuntu:ubuntu`
- [x] storage permissions are restricted
- [x] legacy relative WhatsApp session paths removed
- [x] Ruangkirim session path uses `/var/lib/ruangkirim/whatsapp`
- [x] per-agent SQLite session path implemented
- [x] ChatLoop WhatsApp storage remains isolated
- [x] Ruangkirim WhatsApp storage verified empty before login

## Phase 1G.6C — Database & Runtime Isolation

- [x] Ruangkirim MySQL database exists
- [x] database contains 45 tables
- [x] seed data verified
- [x] `follow_ups` schema verified
- [x] `follow_ups` is not treated as a sender-normalization table
- [x] backend tests pass after database cleanup
- [x] backend production binary builds successfully
- [x] Ruangkirim uses port `3031`
- [x] ChatLoop continues using port `3030`
- [x] ChatLoop process remains `/var/www/chatloop/chatloop-server`
- [x] Ruangkirim runtime verified separately
- [x] Ruangkirim WhatsApp storage remains isolated
- [x] login throttle schema verified
- [x] `locked_until` supports `NULL`
- [x] invalid login returns HTTP `401`
- [x] login throttle row is created without MySQL datetime error
- [x] root endpoint returns HTTP `200`
- [x] production binary installed at `/var/www/ruangkirim/ruangkirim-server`

## Current Runtime State

- **Ruangkirim source:** `/var/www/ruangkirim`
- **Ruangkirim binary:** `/var/www/ruangkirim/ruangkirim-server`
- **Ruangkirim port:** `3031`
- **ChatLoop source:** `/var/www/chatloop`
- **ChatLoop port:** `3030`
- **WhatsApp storage:** `/var/lib/ruangkirim/whatsapp`
- **Systemd service:** not yet created

## Phase 1G.6C.16 — Checkpoint

Production binary has been built and installed.

The binary checksum verified against the build artifact:

`ac45d7241c4fa716018024e58d8f331d8192384e24e03d1ce86aa28c0f5ba114`

Manual runtime verification passed:

- port `3031` available to Ruangkirim
- port `3030` remains owned by ChatLoop
- Ruangkirim process runs independently
- Ruangkirim WhatsApp storage is empty before session login
- database contains 45 tables
- invalid login returns `401`
- login throttle writes `NULL` to `locked_until` correctly

**Next planned step:** WhatsApp session/QR isolation verification.

Do not modify or stop `chatloop.service` during Ruangkirim deployment.
