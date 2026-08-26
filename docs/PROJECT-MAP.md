# Ngirimwa / Kirimwa — Project Architecture & Development Map

> **Purpose:** canonical development map for AI agents and human developers working on `assyauq/ngirimwa`.
>
> **Scope:** source repository structure, runtime architecture, backend/frontend responsibilities, database, authentication, WhatsApp agents, API, AI, deployment, UI/logo discovery, Git workflow, and development safety rules.
>
> **Current branch:** `main`
>
> **Important:** this document describes the repository and known production deployment. Runtime-only files such as `.env`, live databases/session state, the compiled `chatloop-server` binary, and systemd configuration are not necessarily stored in Git.

---

## 1. System Overview

Kirimwa is a WhatsApp business platform combining AI auto-reply, multi-agent WhatsApp, Inbox, CRM, broadcast/blast, scheduling, knowledge base, website crawling, products/orders, REST API, webhook, and tenant/user management.

High-level architecture:

```text
Browser
  |
  | HTTPS
  v
Tencent VPS / Production
  |
  +--> chatloop.service
  |      |
  |      +--> /var/www/chatloop/chatloop-server
  |              |
  |              +--> Gin HTTP API
  |              +--> React/Vite frontend/dist
  |              +--> AI services
  |              +--> WhatsApp services (WhatsMeow)
  |              +--> Schedulers / background jobs
  |
  +--> MySQL
  |
  +--> WhatsApp network
```

Repository/source-of-truth relationship:

```text
GitHub: assyauq/ngirimwa
        |
        | origin (SSH)
        v
VPS: /var/www/chatloop
        |
        +--> production binary + runtime configuration/data
```

---

## 2. Repository Map

```text
ngirimwa/
├── .github/
│   └── workflows/
│       └── release.yml
├── backend/
│   ├── cmd/
│   │   └── seed/
│   ├── config/
│   ├── database/
│   ├── handlers/

│   ├── models/
│   ├── services/
│   └── ui/
├── docs/
├── frontend/
│   ├── public/
│   ├── src/
│   ├── README.md
│   ├── package.json
│   ├── package-lock.json
│   ├── vite.config.ts
│   ├── tsconfig*.json
│   └── eslint.config.js
├── scripts/
├── .air.toml
├── .air.windows.toml
├── .env.example
├── .gitignore
├── NOTICE
├── README.md

├── go.mod
└── go.sum
```

### Runtime-only / environment files (do not assume present in Git)

```text
/var/www/chatloop/
├── .env                    # production secrets/config
├── wa-assistant.db*        # runtime DB/session artifacts may exist on VPS
├── data/                   # runtime media/session data
├── frontend/dist/          # production frontend build (depending on deployment)
└── chatloop-server         # compiled Go binary; intentionally untracked
```

---

## 3. Root Files

| Path | Responsibility | Notes |
|---|---|---|
| `README.md` | Product + developer overview | Functional feature list and local development guide |
| `go.mod` | Go module/dependencies | Module is `kirimwa`, Go 1.25.8 |
| `go.sum` | Go dependency checksums | Must stay synchronized with `go.mod` |
| `.env.example` | Environment variable template | Production `.env` must remain outside Git |
| `.gitignore` | Git exclusions | Prevents runtime/build/secrets from being committed |
| `.air.toml` | Air dev config | Go hot reload for Unix-like environments |
| `.air.windows.toml` | Air Windows config | Windows development |
| `scripts/` | Developer/build helpers | Cross-platform launch/build scripts |
| `.github/workflows/release.yml` | CI/release automation | Inspect before changing deployment/release semantics |
| `NOTICE` | Copyright/legal notice | Preserve |


---

## 4. Backend — Go Architecture

### 4.1 `backend/cmd/`

Contains executable utilities/seed commands rather than the primary production server.

Known path:

```text
backend/cmd/seed/main.go
```

Use for initial/maintenance data seeding. Do not confuse it with the production server entry point.

### 4.2 `backend/config/`

Known file:

```text
backend/config/config.go
```

Responsibilities:

- load `.env` early in package initialization
- read environment variables
- provide typed helpers for string/int/bool configuration
- support parent-directory `.env` discovery for commands started inside subdirectories

Important behavior:

```text
.env lookup:
  .env
  ../.env
  ../../.env
```

Do not hard-code secrets into source. Use `config.Env`, `EnvRequired`, `EnvInt`, `EnvBool`.

### 4.3 `backend/database/`

Known major file:

```text
backend/database/database.go
```

Responsibilities:

- open MySQL connection using environment variables
- create database if it does not exist
- configure connection pool
- run preflight schema checks
- run GORM `AutoMigrate`
- run data/backfill/normalization routines
- seed super admin/default tenant
- recovery of stuck background jobs

Key environment variables:

```text
DB_HOST
DB_PORT
DB_USER
DB_PASS
DB_NAME
DB_MAX_OPEN_CONNS
DB_MAX_IDLE_CONNS
DB_CONN_MAX_LIFETIME_MIN
```

Primary production DB: **MySQL**.

SQLite exists as a Go dependency and may be used elsewhere/runtime components, but the main application database initialization is MySQL-based.

### 4.4 `backend/models/`

ORM/domain models. The current migration list shows a broad SaaS domain including:

```text
User
UserAgentAssignment
CSActivityLog
LoginThrottle
Agent
ChatHistory
InboxReadState
Setting
AITurn
Knowledge
Handoff
Contact
ConversationMemory
CrawlJob
CrawlPage
Tenant
Broadcast
BroadcastRecipient
OptOut
ContactConsent
ScheduledMessage
ScheduledStatus
Label
ChatLabel
AutoReply
Flow
FlowSession
OTPCode
Template
FollowUp
FollowUpStep
FollowUpEnrollment
Product
ProductCheckoutSession
ProductOrder
AIForm
AIFormSession
AIFormSubmission
AppSetting
ClosingForm
ClosingRecord
ShippingCity
GroupGuardConfig
GroupModerationLog
MetaConversionEvent
```

Representative model files include:

```text
backend/models/models.go
backend/models/ai_form.go
backend/models/appsetting.go
backend/models/autoreply.go
backend/models/broadcast.go
backend/models/flow.go
backend/models/followup.go
backend/models/group.go
backend/models/label.go
backend/models/meta_tracking.go
backend/models/otp.go
backend/models/product.go
backend/models/saas.go
backend/models/scheduled.go
backend/models/template.go
```

When adding a persistent domain feature:

1. define/update its model,
2. add it to migration if needed,
3. add handler/service logic,
4. add frontend contract/UI,
5. add tests.

### 4.5 `backend/handlers/`

HTTP/API orchestration layer. This is a large, feature-rich package.

Known/observed domains include:

```text
auth.go
agents.go
ai_form.go
ai_metrics.go
api_broadcast.go
api_config.go
api_keys.go
api_otp.go
api_public.go
api_ratelimit.go
api_resources.go
autoreply.go
broadcast.go
broadcast_error_test.go
broadcast_guard.go
broadcast_rotation.go
chat.go
...
```

Responsibilities commonly include:

- parse HTTP request
- enforce auth/tenant/role rules
- validate input
- call services/database
- return JSON
- media serving/upload endpoints
- dashboard API endpoints

**Architecture rule:** keep complex business logic in `services/` rather than expanding handlers indefinitely.

### 4.6 `backend/services/`

Business logic/integration layer.

Known service files/domains include:

```text
ai.go
ai_advanced.go
ai_response_policy.go
embedding.go
crawler.go
crm_classifier.go
email.go
broadcast_policy.go
...
```

Major responsibilities:

- AI/OpenRouter integration
- semantic embeddings
- crawling/training
- WhatsApp integration
- broadcast policy and scheduling
- CRM/business logic
- retries/background tasks
- external integrations

**AI-agent rule:** inspect the relevant service before adding new business behavior in a handler.

### 4.7 `backend/ui/`

Terminal/startup UI helpers, including license/startup messaging. This is not the React dashboard UI.

---

## 5. Production Entry Point

Primary production executable source:

```text
backend/main.go
```

Startup order observed:

```text
1. database.Init()
2. knowledge consolidation
3. AI initialization
4. embedding initialization
5. WhatsApp initialization
6. WhatsApp event handler registration
7. start linked agents
8. reconnect watchdog
9. resume broadcasts
10. cleanup stuck schedules/broadcast junk/orphan assignments
11. seed shipping cities
12. scheduled message/media cleanup workers
13. failed-send retry worker
14. login throttle sweeper
15. Gin router + middleware
16. serve frontend/dist when available
17. listen on HTTP server
```

This sequence is sensitive. Do not reorder startup dependencies without understanding their runtime implications.

---

## 6. Authentication & Authorization Map

Authentication stack:

```text
Login
  ↓
bcrypt password validation
  ↓
throttle / lockout controls
  ↓
JWT (HS256)
  ↓
Authorization: Bearer <JWT>
  ↓
AuthMiddleware
  ↓
DB user lookup
  ↓
context: user_id / tenant_id / role / is_super_admin
```

Key file:

```text
backend/handlers/auth.go
```

Important security functions:

```text
mustJWTSecret()
AuthMiddleware()
RequireSuperAdmin()
RequireTenantAdmin()
CSRouteGuard()
```

JWT secret must be configured in `.env` and be a strong random value of at least 32 characters.

CORS behavior is environment-dependent:

- development may use `*`
- production rejects `*` and requires explicit `CORS_ALLOWED_ORIGINS`

Do not weaken auth or tenant isolation for convenience.

---

## 7. Multi-Tenant / Role Model

Core hierarchy:

```text
Tenant
├── Owner / Admin
│   ├── users
│   ├── agents
│   ├── configuration
│   └── operational controls
│
└── CS
    └── restricted operational Inbox capabilities
```

Enforcement exists at the backend level. Frontend menu hiding is not considered security.

---

## 8. WhatsApp Agent Architecture

Primary dependency:

```text
go.mau.fi/whatsmeow
```

Conceptual flow:

```text
WhatsApp
   ↓
WhatsMeow
   ↓
WA service layer
   ↓
agent lifecycle + events
   ↓
handlers
   ↓
chat history / inbox / AI / CRM / broadcast
```

Registered event categories include:

```text
OnWAMessage
OnDeviceLinked
OnWAOwnMessage
OnWAHistorySync
OnWAHistoryChatState
OnWAWhatsAppReadState
OnLabelEdit
OnLabelAssoc
OnAgentConnected
OnWAReceipt
OnWAChatPresence
OnWAMessageRevoked
```

Multi-agent endpoints are scoped as:

```text
/api/agents/:id/...
```

Important agent lifecycle operations include:

```text
connect
pairing connect
logout
status
reconnect watchdog
start linked agents
```

Do not create a second WhatsApp client stack without explicitly deciding whether it replaces or complements WhatsMeow.

---

## 9. AI Architecture

Documented capabilities:

- AI auto reply
- OpenRouter as central model gateway
- semantic embedding/search
- knowledge base
- website crawler → page selection → training
- persona
- tone selection
- handoff to human
- AI metrics/evaluation

Relevant services include:

```text
backend/services/ai.go
backend/services/ai_advanced.go
backend/services/ai_response_policy.go
backend/services/embedding.go
backend/services/crawler.go
```

README describes OpenRouter as the central provider for chat/persona/extraction/embedding. Model selection is intended to be configurable from the dashboard rather than hard-coded in `.env` for ordinary model changes.

AI feature development should preserve:

```text
knowledge retrieval
→ prompt/persona
→ model call
→ response policy
→ handoff / safety checks
→ WhatsApp send
→ logging/metrics
```

---

## 10. Knowledge Base

Knowledge endpoints support:

```text
List
Create
Update
Delete
Generate
Import
Crawl
Train crawl pages
Usage metrics
Persona regeneration
Image upload/serve
```

Recent code also supports knowledge images and image-aware AI responses. The image implementation stores an image path/mime and exposes protected image URLs/tokens for frontend/media access.

Runtime image files are expected under a data path such as:

```text
data/knowledge/
```

Do not put runtime customer media into Git.

---

## 11. CRM / Inbox

The current domain includes:

```text
contacts
labels
chat history
conversation memory
handoffs
pipeline/stage concepts
follow-ups
activity logs
orders/products
```

Inbox is agent-scoped and includes event cursors, conversation read state, client debugging, history sync, brief/summary, send/send-media, typing presence, and message revoke operations.

When changing Inbox behavior, inspect both:

```text
handlers/inbox-related code
services/WhatsApp + chat processing code
models.ChatHistory / InboxReadState / ConversationMemory
frontend inbox screens
```

---

## 12. Broadcast / Blast

Documented functions:

- bulk text/media sending
- scheduled blast
- group blast
- personalization `{nama}`
- recipient selection from multiple sources
- WhatsApp-number checks
- opt-out
- consent tracking
- risk level/policy
- randomized delay/rest periods
- humanized typing
- resume after restart
- pause/retry when WhatsApp limits occur

Important services/handlers include broadcast policy/guard/rotation code.

Treat outbound messaging as a high-risk subsystem. Do not remove safety/consent/rate-limit controls when adding UI or API features.

---

## 13. Public REST API

Public API base:

```text
/api/v1
```

Protected by:

```text
APIKeyMiddleware
```

Known endpoint families:

```text
messages
message media
message analysis
OTP
number check
status
contacts
 groups
chats
broadcasts
```

Dashboard-managed agent API configuration includes API key rotation/revocation and webhook settings.

When changing public API behavior:

1. preserve backward compatibility where possible,
2. update docs/examples,
3. update frontend API client if used,
4. add regression tests.

---

## 14. Frontend Architecture

Technology:

```text
React
TypeScript
Vite
```

Frontend root:

```text
frontend/
├── public/
├── src/
├── package.json
├── package-lock.json
├── vite.config.ts
└── tsconfig*.json
```

Production behavior in backend:

```text
STATIC_DIR (default frontend/dist)
        ↓
/assets
/favicon.svg
/icons.svg
SPA fallback → frontend/dist/index.html
```

Development launcher documented in README:

```text
npm run dev
```

which starts:

```text
frontend Vite: 127.0.0.1:5173
backend Air:    127.0.0.1:3030
```

---

## 15. Frontend UI / Logo Mapping

Known asset/UI roots:

```text
frontend/src/       # React application code
frontend/public/    # public/static assets
frontend/index.html # HTML entry
```

Production static output:

```text
frontend/dist/
frontend/dist/assets/
frontend/dist/favicon.svg
frontend/dist/icons.svg
```

**Do not guess logo paths.** Before changing branding:

1. inspect `frontend/src` for logo imports/components,
2. inspect `frontend/public`,
3. inspect `frontend/index.html`,
4. search for the current brand name/text,
5. search for `.svg`, `.png`, `.webp`, `.ico` imports,
6. identify shared layout/theme components,
7. update source, not only `dist`.

Build frontend afterward so production `dist` reflects source.

Recommended search terms:

```text
logo
favicon
chatloop
brand
icon
Logo
```

---

## 16. Deployment Architecture (Current Production)

Current known VPS path:

```text
Tencent Lighthouse
    |
    +--> /var/www/chatloop
             |
             +--> source checkout
             +--> .env
             +--> runtime data
             +--> chatloop-server (binary)
```

Current systemd service:

```text
chatloop.service
```

Known configuration:

```ini
User=ubuntu
Group=ubuntu
WorkingDirectory=/var/www/chatloop
ExecStart=/var/www/chatloop/chatloop-server
Restart=always
RestartSec=5
```

The production service has already been migrated away from the temporary `developer` user.

**Important:** editing source files in GitHub does not automatically deploy them to production unless a deployment mechanism pulls/builds/restarts the server.

---

## 17. Git Workflow — Canonical Rules

Current remote:

```text
git@github.com:assyauq/ngirimwa.git
```

Default branch:

```text
main
```

Production binary:

```text
chatloop-server
```

is intentionally untracked.

### Recommended development flow

```text
main
  |
  +--> feature/<short-name>
          |
          +--> changes
          +--> tests
          +--> review
          +--> merge
                 |
                 v
                main
                 |
                 v
             deployment
```

Do not commit:

```text
.env
production secrets
live customer data
WhatsApp session secrets
compiled binaries unless explicitly intended
```

### Safe update pattern on VPS

Before deployment:

```bash
git status
git branch --show-current
git log -1 --oneline
```

Never blindly run `git pull` on production if there are uncommitted changes.

---

## 18. AI Agent Development Rules

All coding agents working on this project should follow these rules.

### Rule A — Understand before editing

Read the relevant:

```text
model
handler
service
frontend client/component
migration implications
```

before changing behavior.

### Rule B — Preserve contracts

Do not casually rename:

```text
API endpoints
JSON field names
DB columns
model fields
environment variables
agent IDs/JIDs
```

without migration/compatibility planning.

### Rule C — Prefer focused changes

Avoid:

```text
full-project rewrite
large framework migration
unrelated refactor
```

when the task is a localized feature or UI change.

### Rule D — Keep backend authorization authoritative

Never rely only on frontend menu hiding for security.

### Rule E — Protect runtime state

Do not delete/change:

```text
production DB
WA session data
.env
media
```

unless explicitly requested and backed up.

### Rule F — UI changes must remain source-driven

Change React source/assets and rebuild; do not hand-edit generated `frontend/dist` as the primary solution.

### Rule G — Build/test before deployment

At minimum where applicable:

```bash
go test ./...
```

and frontend lint/build commands defined by `frontend/package.json`.

### Rule H — Deployment is a separate step

A successful code change is not the same as a successful production deployment.

---

## 19. High-Risk Areas

Treat these as production-critical:

1. WhatsApp connection/session lifecycle
2. broadcast/rate-limit/anti-spam controls
3. authentication/JWT/tenant isolation
4. database migrations/backfills
5. public API authentication
6. webhook/API secrets
7. production `.env`
8. customer media/session data
9. deployment/systemd configuration

---

## 20. Current Known State

As of the documented audit:

```text
Repository: assyauq/ngirimwa
Branch: main
Language: Go + React/TypeScript
Backend: Gin + GORM
Primary DB: MySQL
WhatsApp: whatsmeow
AI: OpenRouter-compatible AI services + embeddings
Auth: bcrypt + JWT HS256
Architecture: monolithic Go backend + React SPA
Multi-tenant: yes
Multi-agent WhatsApp: yes
Public REST API: yes (/api/v1)
Frontend production serving: Go serves frontend/dist
Production runtime: Tencent Lighthouse VPS
Service: chatloop.service
Production user: ubuntu
```

---

## 21. What Is NOT Proven Solely by This Repository Map

This document intentionally does not invent facts that require production-only inspection.

Items requiring VPS inspection include:

```text
exact .env values
actual MySQL server/version and credentials
actual database size/content
WhatsApp session DB/session state
Nginx/Cloudflare proxy configuration
TLS certificate issuance/renewal mechanism
exact frontend/dist contents currently deployed
exact systemd environment overrides/drop-ins
cron timers outside the repository
OS-level firewall rules
Tencent Lighthouse firewall rules
production upload/media volumes
```

When a task depends on one of these, inspect the VPS rather than guessing from source.

---

## 22. Recommended Next Audit Pass

For a deeper code-level map, inspect in this order:

```text
1. frontend/src full tree + route/layout structure
2. frontend API client/service layer
3. frontend auth/session persistence
4. exact logo/theme/assets
5. backend route registration (complete)
6. WhatsApp service implementation + agent lifecycle
7. database models + relationships
8. AI/knowledge retrieval pipeline
9. broadcast scheduler/policy pipeline
10. public REST API request/response contracts
11. GitHub Actions release workflow
12. production deployment commands
```

The next document/branch should only be created after the above audit identifies the exact files involved in the requested feature.

---

## 23. Quick Agent Orientation

Before touching code, an AI agent should answer:

```text
- What user-facing feature am I changing?
- Which frontend page/component owns it?
- Which API endpoint owns it?
- Which handler owns the request?
- Which service owns the business logic?
- Which model/table stores the state?
- Is the feature agent-scoped or tenant-scoped?
- Is there a role/permission constraint?
- Does the change touch WhatsApp runtime state?
- Does it require a DB migration?
- Does it require a frontend rebuild?
- Does it require a production restart?
- Could it expose a secret or customer data?
```

If any answer is unknown, inspect the repository/source before editing.

---

## 24. Canonical References

- Repository: `https://github.com/assyauq/ngirimwa`
- Main server source: `backend/main.go`
- Backend module/dependencies: `go.mod`
- Auth: `backend/handlers/auth.go`
- Database: `backend/database/database.go`
- Models: `backend/models/`
- Handlers/API: `backend/handlers/`
- Services: `backend/services/`
- Frontend: `frontend/`
- Developer scripts: `scripts/`
- CI/release: `.github/workflows/release.yml`
- Runtime config template: `.env.example`

---

## 25. Change Log for This Map

### Initial architecture map

Created as the canonical project orientation document after migrating the production repository to `assyauq/ngirimwa`.

Purpose of this file:

- reduce repeated discovery by AI agents,
- prevent unsafe blind changes,
- keep frontend/backend/API/database/deployment relationships explicit,
- provide a stable starting point for future development tasks.
