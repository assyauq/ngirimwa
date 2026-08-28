# Ruangkirim — Cloning & Migration Plan

## 1. Objective

Create a completely independent Ruangkirim project from the stable ChatLoop source baseline while preserving application capabilities and separating all production identity and runtime state.

## 2. Source of Truth

ChatLoop is the temporary reference implementation. Ruangkirim is a new project with a new Git history.

```text
ChatLoop source
    ↓ audit
Approved stable baseline
    ↓ copy source only
/var/www/ruangkirim
    ↓ new Git repository
mrifatsyauqi/ruangkirim
```

Do **not** copy the `.git` directory.

## 3. VPS Layout

```text
/var/www/chatloop       # reference / existing system
/var/www/ruangkirim     # independent new system
```

ChatLoop must remain operational until Ruangkirim passes functional and production verification.

## 4. Safe Clone Rules

Copy application source and required project configuration templates, but exclude:

- `.git/`
- `.env` and production environment files
- database dumps containing production data
- API keys and private credentials
- WhatsApp session files
- runtime session directories
- logs
- caches
- `node_modules/`
- `vendor/` when dependencies can be installed cleanly
- build artifacts such as `dist/` when they are reproducible

## 5. Migration Sequence

### Phase 0 — Baseline Audit

Record:

- current Git commit
- branch and remote
- working-tree state
- application structure
- backend/frontend versions
- database configuration shape
- systemd services
- queues and workers
- Redis/cache usage
- WhatsApp session storage
- Nginx configuration

### Phase 1 — Repository Preparation

Create the empty GitHub repository `mrifatsyauqi/ruangkirim` and verify push access.

### Phase 2 — Source Clone

Create `/var/www/ruangkirim`, copy only approved source/configuration templates, and initialize a new Git repository.

### Phase 3 — Initial Git Push

The first commit must represent a clean project initialization, not inherited ChatLoop history.

Recommended message:

```text
feat: initialize Ruangkirim platform
```

### Phase 4 — Identity Migration

Audit all occurrences of:

```text
ChatLoop
chatloop
CHATLOOP
Ngertikode
ngertikode
```

Classify every occurrence before changing it. User-facing brand references become Ruangkirim; third-party names and required technical references are not blindly renamed.

### Phase 5 — Runtime Isolation

Create new database, environment, service, storage, queue/cache namespace, and WhatsApp session state.

### Phase 6 — Fresh Installation

A blank Ruangkirim database must be installable from migrations/seeds without importing ChatLoop production data.

### Phase 7 — Functional Verification

Test authentication, dashboard, APIs, queues, scheduler, WhatsApp subsystem, frontend build, and runtime services.

### Phase 8 — Production Cutover

Configure independent Nginx/server block, domain, SSL, services, workers, scheduler, and monitoring. Keep ChatLoop available for rollback/reference.

## 6. Definition of Done

- [ ] New Git repository
- [ ] New Git history
- [ ] `/var/www/ruangkirim`
- [ ] Fresh `.env`
- [ ] New application encryption key where applicable
- [ ] Fresh database
- [ ] Independent session state
- [ ] Independent cache/queue namespace
- [ ] Independent service
- [ ] Independent storage/logging
- [ ] Ruangkirim branding
- [ ] No unintended ChatLoop user-facing branding
- [ ] Frontend build passes
- [ ] Backend starts
- [ ] Authentication works
- [ ] API works
- [ ] WhatsApp subsystem works
- [ ] Production deployment verified

## 7. Rollback

Never delete ChatLoop during migration. If Ruangkirim fails validation, stop the new deployment and return traffic to the known-good ChatLoop runtime. Investigate using the Ruangkirim Git commit and logs.
