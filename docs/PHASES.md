# Ruangkirim — Execution Phases

## Phase 0 — Baseline Audit

Record the stable ChatLoop commit, branch, remote, source structure, runtime stack, services, database, cache/queue, WhatsApp state and Nginx configuration.

**Exit:** baseline is documented and working tree is understood.

## Phase 1 — GitHub Preparation

Repository `mrifatsyauqi/ruangkirim` exists and push access is verified.

**Exit:** empty/new repository ready.

## Phase 2 — Source Clone

Copy approved source only to `/var/www/ruangkirim`. Do not copy `.git`, secrets, production data, sessions or runtime artifacts.

**Exit:** source exists independently on VPS.

## Phase 3 — New Git History

Initialize Git and push the clean source to Ruangkirim GitHub.

**Exit:** GitHub contains a clean Ruangkirim baseline.

## Phase 4 — Branding

Migrate user-facing identity and appropriate technical identifiers after classification.

**Exit:** no unintended legacy branding remains.

## Phase 5 — Database

Create fresh database and validate migrations/seeds from zero.

**Exit:** clean database installation works.

## Phase 6 — Runtime Isolation

Separate service, storage, cache/queue, scheduler and WhatsApp session state.

**Exit:** Ruangkirim runs independently from ChatLoop.

## Phase 7 — Functional Testing

Test frontend, backend, authentication, APIs, queues and WhatsApp subsystem.

**Exit:** all critical flows pass.

## Phase 8 — Production Infrastructure

Configure Nginx, domain, TLS, services and monitoring without disrupting ChatLoop.

**Exit:** Ruangkirim is reachable through its intended production endpoint.

## Phase 9 — Cutover

Move production traffic only after a known-good Ruangkirim deployment is verified.

**Exit:** production traffic is served by Ruangkirim and rollback remains possible.

## Phase 10 — Post-Cutover Audit

Review logs, service health, database writes, sessions, queues, branding, backups and Git state.

**Exit:** Ruangkirim is declared stable.

## Change Protocol

Every phase follows:

```text
Audit → Change → Build/Test → Diff Review → Commit → Push
```

Do not mix unrelated architecture changes into the initial clone unless explicitly documented.
