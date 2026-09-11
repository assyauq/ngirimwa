# PHASE 2B.2 — SAAS IMPLEMENTATION PLAN & RUNTIME AUDIT
**Project:** RUANGKIRIM
**Repository:** `mrifatsyauqi/ruangkirim`
**Checkpoint:** Phase 2B.2 (Architecture Audit, Runtime Gap Analysis & Implementation Planning)
**Status:** READY FOR FINAL GATE REVIEW
**Scope:** Strictly Read-Only Audit & Implementation Planning (No Source Code / Schema / Runtime Mutations)

---

## 1. Executive Summary

Phase 2B.2 transitions RUANGKIRIM from an approved conceptual and database specification (`docs/SAAS-DOMAIN-MODEL-FINAL.md` and `docs/SAAS-DATABASE-SPEC-FINAL.md`) into an executable, risk-mitigated engineering blueprint.

The primary objective of this audit was to inspect the **live runtime**, **source code**, and **database schema** of both Production (`ruangkirim` on port 3031) and Staging (`ruangkirim_staging` on port 3032), identify every divergence between current state and the multi-tenant SaaS target, and construct the precise migration sequence and application changes required for Phase 2B.3 execution.

### Key Audit Highlights & Critical Findings

1. **AutoMigrate Runtime (P0 - Critical Stability Hazard):**
   GORM `DB.AutoMigrate(...)` executes synchronously on every application boot in `backend/database/database.go:71`. There is **zero governance**: the environment variable `AUTO_MIGRATE` is **never read** in Go source code. Furthermore, startup routines (`database.go:1066`) execute unmanaged data mutations (e.g., forcing `agent_id = 1` for all knowledge and chat history rows where `agent_id` is 0 or NULL, despite Agent 1 not existing in production). Strict governance must be introduced prior to Phase 2B.3.
2. **Current Database Baseline (Verified Read-Only):**
   Both Production (`ruangkirim`) and Staging (`ruangkirim_staging`) currently contain exactly **45 tables**. Production holds **29,941 chat histories**, **5,163 inbox read states**, **532 contacts**, **1 tenant** (Tenant 1, "Default"), **1 user** (User 1, "superadmin"), and **1 agent** (Agent 3, "Admin J&T Express Batang"). The WhatsApp session is stored in SQLite at `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db` (44.3 MB).
3. **Super Admin / Tenant Context Boundary (P0 - Security Hazard):**
   In `backend/handlers/auth.go:124-126`, when a Super Admin has `tenant_id == NULL`, the authentication middleware explicitly falls back to `tenant_id = 1`. In `isTenantAdmin(c)` (`auth.go:167`), `is_super_admin == true` automatically bypasses all tenant scoping. Super Admin platform operations are currently conflated with Tenant 1 workspace operations.
4. **Knowledge Nullability Divergence (P1 - Functional Requirement):**
   In MySQL, `knowledges.agent_id` is already defined as `BIGINT UNSIGNED NULL DEFAULT NULL`, but GORM Go struct `models.Knowledge` defines `AgentID uint` (`index`). Application queries in `services/embedding.go:53` query `agent_id = ?` exclusively, completely bypassing tenant-wide knowledge. Moreover, `database.go:1066` on every startup executes `UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL`, which destroys tenant-wide knowledge definitions.
5. **Unenforced Commercial Quota (P1 - Commercial Requirement):**
   `backend/handlers/plan_features.go` contains stubs `tenantPlanAllows()` and `agentPlanAllows()` that unconditionally return `true`. Sender creation (`CreateAgent`) and WhatsApp pairing (`ConnectNumber`, `ConnectPairingNumber`) contain no quota or subscription checks.
6. **Safe Backward-Compatible Rollout Path:**
   Phase 2B is designed to be backward-compatible and predominantly additive, with controlled constraint relaxation where required, including support for nullable knowledge agent ownership. Because high-volume transactional tables (`chat_histories` with 29,941 rows and `inbox_read_states` with 5,163 rows) are preserved without DDL mutation, operational impact is designed to minimize lock contention, supported by a structured rollback procedure and validation gates.

---

## 2. Current Runtime Baseline

| Component | Production Runtime | Staging Runtime | Audit Verification Method |
| :--- | :--- | :--- | :--- |
| **Host IP** | `43.173.7.8` | `43.173.7.8` | SSH (`ubuntu@43.173.7.8`) |
| **Working Directory** | `/var/www/ruangkirim` | `/var/www/ruangkirim-staging` | Filesystem inspection |
| **Binary Path** | `/var/www/ruangkirim/ruangkirim-server` | `/var/www/ruangkirim-staging/ruangkirim-staging-server` | Filesystem inspection |
| **Systemd Service** | `ruangkirim.service` | `ruangkirim-staging.service` | `systemctl status` |
| **Process ID / Status** | Active (Running) | Active (Running) | `systemctl status` |
| **HTTP Port** | `3031` | `3032` | `netstat -tlpn` |
| **Public Domain** | `ruangkirim.web.id` | `dev.ruangkirim.web.id` | Nginx config inspection |
| **Database Name** | `ruangkirim` | `ruangkirim_staging` | MySQL `SELECT DATABASE()` |
| **DB User / Host** | Localhost (MySQL 8.0) | Localhost (MySQL 8.0) | `mysql -e "STATUS"` |
| **WhatsApp Storage** | `/var/lib/ruangkirim/whatsapp/` | `/var/lib/ruangkirim-staging/whatsapp/` | Read-only file stat |
| **Active WA Session** | `wa-session-agent-3.db` (44.3 MB) | `wa-session-agent-1.db` (staging) | Read-only file stat |
| **Legacy ChatLoop** | `/var/www/chatloop` (Port 3030 inactive) | N/A | Systemd & port inspection |

---

## 3. Current Database Baseline

A read-only inspection was performed across both `ruangkirim` and `ruangkirim_staging` via MySQL `information_schema`.

### 3.1 Table Count & Schema Parity
- **Production (`ruangkirim`):** Exactly 45 tables.
- **Staging (`ruangkirim_staging`):** Exactly 45 tables.
- **Parity:** 100% schema parity across all 45 existing tables.

### 3.2 Production Row Counts & Inventory Baseline
- `tenants`: 1 row (ID 1, Name: "Default", Created: 2026-08-29)
- `users`: 1 row (ID 1, Username: "superadmin", Email: "super@wa-assistant.local", Role: "admin", IsSuperAdmin: 1, TenantID: NULL)
- `agents`: 1 row (ID 3, Name: "Admin J&T Express Batang", TenantID: 1, Number: "6282211700060")
- `chat_histories`: 29,941 rows (Agent 3)
- `inbox_read_states`: 5,163 rows (Agent 3)
- `contacts`: 532 rows (Agent 3)
- `knowledges`: 10 rows (Agent 3)
- `foreign_keys`: Only 2 foreign keys exist in the entire database:
  - `fk_ai_form_submissions_form`: `ai_form_submissions.form_id -> ai_forms.id`
  - `fk_product_orders_product`: `product_orders.product_id -> products.id`
  *(GORM default does not enforce FK constraints across core entities).*

### 3.3 Target Table Schemas (Current vs Phase 2B Target)

#### A. `tenants` Table
| Column | Current Type | Nullable | Current Default | Phase 2B Target Status |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `BIGINT UNSIGNED` | NO | AUTO_INCREMENT | Keep existing PK |
| `name` | `LONGTEXT` | YES | NULL | Keep existing column |
| `created_at` | `DATETIME(3)` | YES | NULL | Keep existing column |
| `updated_at` | `DATETIME(3)` | YES | NULL | Keep existing column |
| `slug` | *Missing* | - | - | **[ADD]** `VARCHAR(64) NULL`, UNIQUE index |
| `status` | *Missing* | - | - | **[ADD]** `VARCHAR(24) NOT NULL DEFAULT 'trialing'` |
| `trial_ends_at`| *Missing* | - | - | **[ADD]** `DATETIME(3) NULL` |

#### B. `users` Table
| Column | Current Type | Nullable | Current Key | Phase 2B Target Status |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `BIGINT UNSIGNED` | NO | PRI | Keep existing PK |
| `username` | `VARCHAR(64)` | NO | UNI | Keep existing column & unique index |
| `password` | `LONGTEXT` | YES | NULL | Keep existing column |
| `role` | `VARCHAR(24)` | YES | 'owner' | Deprecate in favor of `tenant_members.role` |
| `email` | `VARCHAR(255)` | YES | None | Keep nullable in Phase 2B (Phase A compatibility) |
| `email_verified`| `TINYINT(1)` | YES | 0 | Keep existing column |
| `tenant_id` | `BIGINT UNSIGNED`| YES | MUL | Maintain as cached/last active tenant context |
| `is_super_admin`| `TINYINT(1)`| YES | 0 | Keep for platform-level operator role |

#### C. `agents` Table
| Column | Current Type | Nullable | Current Key | Phase 2B Target Status |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `BIGINT UNSIGNED` | NO | PRI | Keep existing PK |
| `tenant_id` | `BIGINT UNSIGNED` | NO | MUL | Retain strict tenant ownership |
| `name` | `LONGTEXT` | YES | NULL | Keep |
| `number` | `LONGTEXT` | YES | NULL | Keep |
| `device_j_id` | `LONGTEXT` | YES | NULL | Keep for WhatsApp session mapping |

#### D. `knowledges` Table
| Column | Current Type | Nullable | Current Key | Phase 2B Target Status |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `BIGINT UNSIGNED` | NO | PRI | Keep existing PK |
| `agent_id` | `BIGINT UNSIGNED` | **YES** | MUL | Already nullable in MySQL; update Go struct to `*uint` |
| `tenant_id` | *Missing* | - | - | **[ADD]** `BIGINT UNSIGNED NOT NULL`, index on `tenant_id` |
| `question` | `TEXT` | YES | NULL | Keep |
| `answer` | `TEXT` | YES | NULL | Keep |

#### E. `contacts` Table
| Column | Current Type | Nullable | Current Key | Phase 2B Target Status |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `BIGINT UNSIGNED` | NO | PRI | Keep existing PK |
| `agent_id` | `BIGINT UNSIGNED` | NO | Part of UNI | Keep operational ownership |
| `number` | `VARCHAR(32)` | NO | Part of UNI | Unique index: `(agent_id, number)` |
| `tenant_id` | *Missing* | - | - | **[ADD]** `BIGINT UNSIGNED NOT NULL`, index on `tenant_id` |

---

## 4. AutoMigrate Runtime Audit

### 4.1 Detailed Startup Call Chain
Inspection of `backend/main.go` and `backend/database/database.go` reveals the exact execution path on service startup:

```text
main() [backend/main.go:20]
 │
 ├── 1. database.Init() [backend/main.go:21]
 │    ├── config.Env(...) reads DB credentials from .env
 │    ├── rootDB.Exec("CREATE DATABASE IF NOT EXISTS ...")
 │    ├── gorm.Open(mysql.Open(dsn), ...)
 │    ├── preflightCanonicalChatSchema() [verifies wa_msg_key generated column]
 │    ├── DB.AutoMigrate( [backend/database/database.go:71-91]
 │    │     &models.User{}, &models.UserAgentAssignment{}, &models.CSActivityLog{}, ... (44 models)
 │    │   )
 │    ├── backfillKnowledgeCharCount()
 │    ├── backfillHistoricalDeliveryStatus()
 │    ├── backfillInboxLastMsgAt()
 │    ├── normalizeSenderFields()
 │    ├── ensureCanonicalChatMessageIDs() [executes ALTER TABLE chat_histories / CREATE UNIQUE INDEX]
 │    ├── recoverStuckCrawlJobs()
 │    ├── seedSuperAdmin()
 │    └── seedDefaultTenant() [backend/database/database.go:1043]
 │         ├── Checks if tenant 1 exists (creates if missing)
 │         ├── Counts agents for tenant 1
 │         └── Overwrites orphans:
 │             UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;  <-- CRITICAL DEFECT
 │             UPDATE chat_histories SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;
 │
 ├── 2. handlers.ConsolidateAllKnowledge() [backend/main.go:22]
 │    └── Loops distinct agent_ids in knowledges and deduplicates questions per agent
 │
 ├── 3. services.InitAI() [backend/main.go:27]
 ├── 4. services.InitEmbedding() [backend/main.go:28]
 ├── 5. services.InitWA(...) [backend/main.go:30]
 ├── 6. handlers.StartAgents [backend/main.go:43, background goroutine]
 ├── 7. handlers.CleanupStuckSchedules(), CleanupBroadcastJunk(), CleanupOrphanAssignments()
 ├── 8. gin.Default() router initialization [backend/main.go:56]
 └── 9. srv.ListenAndServe() [backend/main.go:280]
```

### 4.2 AutoMigrate Findings Matrix
| Audit Item | Current Behavior | Audit Result / Risk Level |
| :--- | :--- | :--- |
| **Call Site** | `backend/database/database.go:71-91` | Single call site across the entire application |
| **Execution Trigger** | Synchronous during `database.Init()` on every boot | **P0 Hazard:** Crashes startup if any DDL fails |
| **Order vs HTTP** | Runs **before** HTTP server initialization | Server does not open port until AutoMigrate finishes |
| **Order vs WhatsApp** | Runs **before** WhatsApp client connects | WA connection delayed until DB migration succeeds |
| **`AUTO_MIGRATE` Env Flag**| **NEVER READ** in Go code | Setting `AUTO_MIGRATE=false` in `.env` currently has no effect |
| **Production Impact** | AutoMigrate runs automatically on production deploy | Any incompatible struct tag can crash production |
| **Staging Impact** | Identical to production | Runs automatically on every staging reboot |
| **Test Impact** | Unit tests invoking `database.Init()` execute AutoMigrate | Modifies connected test DB |
| **Other DDL Mutations**| `ensureCanonicalChatMessageIDs()` runs raw `ALTER TABLE` | Raw DDL executed on every boot |
| **Data Mutations on Boot**| `seedDefaultTenant()` line 1066 overwrites NULL `agent_id` | **P0 Blocker:** Overwrites tenant-wide knowledges to `agent_id = 1` |

### 4.3 AutoMigrate Governance Design for Phase 2B.3
To meet ADR-10 requirements without breaking developer agility:
1. Introduce `config.EnvBool("AUTO_MIGRATE", false)` in `backend/database/database.go`.
2. In production systemd configuration (`ruangkirim.service`), set `AUTO_MIGRATE=false`.
3. When `AUTO_MIGRATE=false`, service startup must:
   - **NOT** execute GORM `AutoMigrate`.
   - **NOT** execute schema-changing DDL statements.
   - Perform read-only schema preflight validation (verifying required tables and columns exist).
   - Fail clearly with descriptive error logging if any required schema element is missing.
   - Avoid unrelated or unmanaged data mutations.
4. When `AUTO_MIGRATE=true` (e.g. in development environments), execute controlled additive migration.
5. Audit and govern other startup mutation paths:
   - `ensureCanonicalChatMessageIDs()`: Gate raw `ALTER TABLE` and `CREATE UNIQUE INDEX` calls behind migration preflight checks.
   - Backfill functions (`backfillKnowledgeCharCount()`, `backfillHistoricalDeliveryStatus()`, `backfillInboxLastMsgAt()`): Verify these run idempotently and safely without table locks.
   - `normalizeSenderFields()`: Ensure this routine does not block startup or conflict with normalized international numbers.
   - `seedDefaultTenant()` lines 1065-1067: Remove unconditional `UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;` so tenant-wide knowledge is preserved. Replace with safe orphan re-parenting only when a record lacks both `tenant_id` and `agent_id`.

---

## 5. SaaS Architecture Gap Analysis

| SaaS Architecture Requirement | Current State in Repository | Target Phase 2B Specification | Implementation Gap / Action |
| :--- | :--- | :--- | :--- |
| **Multi-Tenancy** | Single tenant in practice (Tenant 1 "Default") | Multi-tenant workspace isolation with slug and lifecycle | Add `slug`, `status`, `trial_ends_at` to `tenants` |
| **Tenant Membership** | 1:1 `users.tenant_id` foreign key column | N:M `tenant_members` junction table with roles | Create `tenant_members` table; backfill User 1 |
| **Plans & Packaging** | No plan tables; hardcoded strings | Canonical `plans` and `plan_features` tables | Create `plans`, `plan_features`; seed plans |
| **Subscriptions** | Missing | `subscriptions` table with `is_current` unique index | Create `subscriptions` table; attach Grandfathered sub |
| **Entitlement Service** | Stubs in `plan_features.go` returning `true` | Centralized `EntitlementService` interface | Implement `EntitlementService` in `backend/services/` |
| **Trial Management** | None | 30-day trial, max 1 active sender, trialing lifecycle | Enforce trial limits in EntitlementService |
| **Sender Limit** | Hardcoded comment "tidak ada batas jumlah nomor" | Governed by plan quota (`GetActiveSenderLimit`) | Enforce sender quota on Agent Create & WA Connect |
| **Usage Tracking** | None | `usage_counters` per (tenant, metric, period_start, period_end) | Create `usage_counters`; meter billable quotas |
| **Audit Logging** | `cs_activity_logs` only | System-wide `audit_logs` for compliance/security | Create `audit_logs` table |

---

## 6. Authentication & Tenant Context Gap Analysis

### 6.1 Current Authentication & Claims
- **Login Endpoint:** `POST /api/login` (`backend/handlers/auth.go:423`).
- **Identifier Lookup:** Queries `WHERE username = ?` exclusively. Cannot authenticate via email.
- **Password Verification:** Uses `bcrypt.CompareHashAndPassword`.
- **JWT Generation (`issueToken`):**
  ```go
  claims := jwt.MapClaims{
      "user_id":        u.ID,
      "role":           u.Role,
      "is_super_admin": u.IsSuperAdmin,
      "exp":            time.Now().Add(...).Unix(),
  }
  if u.TenantID != nil {
      claims["tenant_id"] = *u.TenantID
  } else if u.IsSuperAdmin {
      claims["tenant_id"] = uint(1) // Fallback hazard!
  }
  ```
- **Context Injection (`AuthMiddleware`):**
  Puts `user_id`, `tenant_id`, `role`, and `is_super_admin` into `*gin.Context`.

### 6.2 Gap Analysis vs Target
1. **Missing Membership Validation:** Current JWT relies solely on `users.tenant_id`. It does not check whether the user is an active member in `tenant_members`.
2. **Super Admin Platform Identity Boundary (P0):** Super Admin is a **PLATFORM identity** and must not implicitly receive Tenant 1 context. In the current implementation, a Super Admin without `tenant_id` falls back directly to Tenant 1. Tenant context must be explicitly selected and authorization-checked. Any explicit tenant context mechanism must:
   - Validate target tenant existence and active status.
   - Validate platform operator authorization.
   - Produce auditable log records in `audit_logs`.
   - Never bypass tenant isolation rules.
   - Never silently map Super Admin to Tenant 1.
   If an internal header or query parameter (e.g. `X-Tenant-Context`) is used, it must be treated strictly as a transport mechanism, not as proof of authorization.
3. **Tenant Switching:** Currently impossible without modifying `users.tenant_id` in the database.
   - *Target:* Introduce `POST /api/auth/switch-tenant` allowing a user with multiple memberships to select their active workspace and receive an updated JWT.

---

## 7. Tenant Isolation Audit

Comprehensive audit across all application entities and database tables:

| Resource | Primary Ownership | Current Scoping Mechanism | Cross-Tenant Leak Risk | Required Phase 2B Change | Priority |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`agents`** | Direct `tenant_id` | `WHERE tenant_id = ?` in `currentAgentID` | LOW (well scoped) | Enforce plan sender quota on creation | P1 |
| **`chat_histories`** | Indirect via `agent_id` | Scoped by `agent_id` validated via `currentAgentID` | LOW | **DO NOT ADD tenant_id** (preserve performance) | P0 Rule |
| **`inbox_read_states`** | Indirect via `agent_id` | Scoped by `agent_id` validated via `currentAgentID` | LOW | **DO NOT ADD tenant_id** (preserve performance) | P0 Rule |
| **`knowledges`** | Currently `agent_id` | `WHERE agent_id = ?` | HIGH (cannot share across agents) | Add `tenant_id`; make `agent_id` nullable | P0 |
| **`contacts`** | Currently `agent_id` | `WHERE agent_id = ?` | MEDIUM (no tenant-level CRM index) | Add `tenant_id` as denormalized index | P1 |
| **`broadcasts`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership; add quota check | P1 |
| **`follow_ups`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`flows`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`products`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`product_orders`** | Indirect via `product_id` | Joined via `products.agent_id` | LOW | Retain indirect ownership | P2 |
| **`ai_forms`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`templates`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`labels`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`auto_replies`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`crawl_jobs`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`opt_outs`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |
| **`otp_codes`** | Indirect via `agent_id` | `WHERE agent_id = ?` | LOW | Retain indirect ownership | P2 |

---

## 8. Knowledge agent_id NULL Audit

Per Audit Objective 3, every knowledge-related code block was analyzed and classified:
- **Class A:** NULL Safe
- **Class B:** Requires Application Adjustment
- **Class C:** Assumes Agent Ownership / Non-Null `agent_id`

### Classification & Impact Matrix

#### Finding 1: GORM Model Definition
- **File:** `backend/models/models.go:192`
- **Current Behavior:** `AgentID uint` (`gorm:"index" json:"agent_id"`)
- **Classification:** **Class C (ASSUMES NON-NULL)**
- **Why it matters:** Go `uint` cannot represent SQL `NULL`. A row with `agent_id = NULL` will deserialize into Go as `0`, which conflicts with orphan checks.
- **Required Change:** Change field type to `AgentID *uint` (`gorm:"index" json:"agent_id"`). Add `TenantID uint` (`gorm:"not null;index" json:"tenant_id"`).
- **Risk:** High (Affects all serializations).

#### Finding 2: Startup Seeder Orphan Mutation
- **File:** `backend/database/database.go:1066`
- **Current Behavior:** `DB.Model(&models.Knowledge{}).Where("agent_id = 0 OR agent_id IS NULL").Update("agent_id", 1)`
- **Classification:** **Class C (CRITICAL HAZARD)**
- **Why it matters:** On every boot, this overwrites any intentional tenant-wide knowledge (`agent_id IS NULL`) and sets its `agent_id = 1`!
- **Required Change:** Remove this unconditional update completely.
- **Risk:** P0 Blocker.

#### Finding 3: Startup Deduplication
- **File:** `backend/handlers/knowledge_store.go:248-253` (`ConsolidateAllKnowledge`)
- **Current Behavior:**
  ```go
  var agentIDs []uint
  database.DB.Model(&models.Knowledge{}).Distinct("agent_id").Pluck("agent_id", &agentIDs)
  for _, agentID := range agentIDs { consolidateKnowledgeForAgent(agentID) }
  ```
- **Classification:** **Class B (REQUIRES ADJUSTMENT)**
- **Why it matters:** Deduplication only operates on distinct non-null `agent_id` values. Tenant-wide knowledges (`agent_id IS NULL`) are never deduplicated.
- **Required Change:** Update `ConsolidateAllKnowledge()` to deduplicate per `tenant_id` where `agent_id IS NULL`, in addition to per-agent deduplication.
- **Risk:** Medium.

#### Finding 4: In-Memory RAG Retrieval Cache & Deterministic Selection
- **File:** `backend/services/embedding.go:29, 43-68` (`KnowledgeFor`)
- **Current Behavior:**
  ```go
  var kbCache = map[uint][]KBItem{} // Keyed by agentID uint
  database.DB.Where("agent_id = ? AND active = ?", agentID, true).Find(&rows)
  ```
- **Classification:** **Class C (ASSUMES NON-NULL)**
- **Why it matters:** Queries DB strictly for `agent_id = ?`. Any tenant-wide knowledge (`agent_id IS NULL`) is completely omitted from the RAG context.
- **Required Deterministic Selection Logic:**
  Knowledge retrieval **MUST NOT rely on database row ordering** to determine which answer wins. The implementation must follow deterministic selection logic:
  1. Retrieve tenant-wide knowledge (`tenant_id = ? AND agent_id IS NULL AND active = 1`).
  2. Retrieve agent-specific knowledge (`tenant_id = ? AND agent_id = ? AND active = 1`).
  3. Match relevant knowledge candidates against the incoming message.
  4. When the same logical question or canonical key exists in both scopes, the **agent-specific item deterministically overrides the tenant-wide item**.
  5. The resulting prompt injection must be deterministic regardless of SQL row order.
- **Risk:** High (Core AI accuracy).

#### Finding 5: Cache Invalidation
- **File:** `backend/services/embedding.go:35, 230` (`InvalidateKB`, `IndexKnowledge`)
- **Current Behavior:** `InvalidateKB(agentID uint)` and `InvalidateKB(k.AgentID)`
- **Classification:** **Class B (REQUIRES ADJUSTMENT)**
- **Why it matters:** If `k.AgentID == nil`, `*k.AgentID` causes a nil pointer panic. Invalidation of a tenant-wide knowledge item must invalidate the cache for ALL agents belonging to that tenant.
- **Required Change:** Add `InvalidateTenantKB(tenantID uint)` that clears cache for all agents in the tenant. Guard `InvalidateKB` calls with nil checks.
- **Risk:** High.

#### Finding 6: Knowledge Listing & Filtering Endpoints
- **File:** `backend/handlers/chat.go:135` (`ListKnowledge`)
- **Current Behavior:** `database.DB.Where("agent_id = ?", aid).Find(&kb)`
- **Classification:** **Class B (REQUIRES ADJUSTMENT)**
- **Why it matters:** The UI currently only sees agent-scoped knowledge. Tenant Admin needs to view and manage tenant-wide knowledge.
- **Required Change:** Support query parameter `?scope=all` or separate `/api/knowledge` (tenant-wide) vs `/api/agents/:id/knowledge` (agent overrides).
- **Risk:** Low.

#### Finding 7: Knowledge Deletion & Update
- **File:** `backend/handlers/chat.go:208, 325` (`UpdateKnowledge`, `DeleteKnowledge`)
- **Current Behavior:** Scoped strictly with `.Where("agent_id = ?", aid)`
- **Classification:** **Class B (REQUIRES ADJUSTMENT)**
- **Why it matters:** Cannot update or delete tenant-wide knowledge (`agent_id IS NULL`).
- **Required Change:** If updating tenant-wide item, verify caller is Tenant Admin and scope by `tenant_id = currentTenantID(c) AND id = ? AND agent_id IS NULL`.
- **Risk:** Medium.

---

## 9. Contacts tenant_id Audit

### 9.1 Current Architecture
- **Ownership:** `models.Contact` has `AgentID uint` (`uniqueIndex:idx_contact_agent_number;not null`).
- **Composite Uniqueness:** Handled by `idx_contact_agent_number` on `(agent_id, number)`.
- **Phone Number Conflict Risk:** Multiple agents can communicate with the same customer phone number without colliding in MySQL, because `agent_id` is part of the unique key.

### 9.2 Audit Questions & Answers
1. **Current ownership boundary:** Operational ownership belongs to `agent_id`.
2. **Is `agent_id` always validated?** Yes, via `resolveAgent(c)`.
3. **Is tenant context always enforced?** Implicitly via `agents.tenant_id = currentTenantID(c)` during agent resolution.
4. **Where `tenant_id` should be populated:**
   - Handlers: `CreateSavedContact`, `APISaveContact`, `syncCRMContactNames`, and auto-contact creation on incoming WhatsApp messages (`OnWAMessage`).
   - Population formula: `contact.TenantID = agent.TenantID`.
5. **Could any query accidentally trust `tenant_id` alone?**
   - Direct lookup `WHERE tenant_id = ? AND number = ?` would return multiple rows if multiple agents in the same tenant chatted with the same number. Lookups must continue to qualify `agent_id` for agent operational flows.
6. **Required Indexes:**
   - Add `idx_contacts_tenant_id` on `(tenant_id)` for tenant-wide CRM listing and export.
   - Retain `idx_contact_agent_number` on `(agent_id, number)` for uniqueness.

---

## 10. Subscription & Entitlement Gap Analysis

### 10.1 Gap Matrix vs Approved Design
| Component | Current Implementation | Target Specification | Gap Severity |
| :--- | :--- | :--- | :--- |
| **`plans` Table** | None | Defines plan code, name, price, max senders | P0 |
| **`plan_features` Table** | None | Key-value feature limits per plan | P0 |
| **`subscriptions` Table** | None | Tracks commercial lifecycle, start/end date, current flag | P0 |
| **`CanUseFeature`** | `tenantPlanAllows()` returns true | Checks active subscription plan feature flags | P1 |
| **`CanConsumeQuota`** | None | Checks if current usage < subscription quota limit | P1 |
| **`ConsumeQuota`** | None | Atomically increments `usage_counters` | P1 |
| **`GetActiveSenderLimit`**| None | Returns 1 for trial; plan quota for active subscriptions | P0 |
| **`GetTenantSubscriptionState`** | None | Returns commercial and operational state | P0 |

### 10.2 Subscription Current-Record Concurrency & Invariants
The database constraint `UNIQUE KEY uk_subscriptions_tenant_current (tenant_id, is_current)` enforces at the MySQL engine level that there is **AT MOST ONE** current subscription (`is_current = 1`) per tenant. It does NOT guarantee **EXACTLY ONE** current subscription.

Application logic must maintain the operational invariant:
- Exactly one current subscription should exist for a tenant in normal operational state.
- Historical subscriptions are maintained with `is_current = NULL`.
- When activating, renewing, or switching current subscriptions, the update must be performed inside a database transaction:
  1. Lock tenant subscription state (`SELECT ... FOR UPDATE`).
  2. Safely demote the existing current record (`UPDATE subscriptions SET is_current = NULL, updated_at = NOW() WHERE tenant_id = ? AND is_current = 1`).
  3. Promote or insert the new current record (`is_current = 1`).
  4. Validate that exactly one current record exists before committing transaction.
  5. Handle concurrency conflicts gracefully via transactional retries.

---

## 11. Sender Quota Audit

### 11.1 Sender Connection Paths
1. `POST /api/agents` (`CreateAgent` in `backend/handlers/agents.go:1816`):
   - Currently creates agent directly without checking total agent count.
2. `POST /api/agents/:id/wa/connect` (`ConnectNumber` in `backend/handlers/numbers.go:72`):
   - Calls `services.WA(id).Connect(a.DeviceJID)`. No quota check.
3. `POST /api/agents/:id/wa/connect-pairing` (`ConnectPairingNumber` in `backend/handlers/numbers.go:45`):
   - Calls `services.WA(id).ConnectPairing(...)`. No quota check.
4. Auto-Reconnect Watchdog (`services.StartReconnectWatchdogCtx` in `backend/services/wa_watchdog.go`):
   - Re-establishes connection for agents that have saved credentials.

### 11.2 Quota Enforcement Points (Phase 2B.3)
- In `ConnectNumber` and `ConnectPairingNumber`:
  ```go
  limit, err := entitlementService.GetActiveSenderLimit(agent.TenantID)
  activeCount := countActiveConnectedSenders(agent.TenantID)
  if activeCount >= limit && !isCurrentlyConnected(agent.ID) {
      c.JSON(403, gin.H{"error": "Batas nomor aktif telah tercapai. Upgrade paket untuk menambah nomor."})
      return
  }
  ```
- For Trialing Tenants: `limit = 1`. If Agent 3 is connected, attempting to connect Agent 4 will return `403 Forbidden`.

---

## 12. Usage Counter Design Review

### 12.1 Quota Metrics vs Analytics Metrics
The implementation must strictly distinguish between metrics that govern commercial quota limits (billable/limiting counters) versus metrics tracked solely for operational analytics:
- **Authoritative Commercial Quota Metrics:**
  - `messages_outbound`: Outbound messages sent via WhatsApp or public REST API.
  - `ai_turns`: Completed AI customer service turns.
- **Analytics-Only Metric:**
  - `broadcast_recipients`: Recipient rows dispatched in marketing or notification broadcast campaigns.

| Metric | Metric Class | Event Trigger | Phase 2B.3 Enforcement | Idempotency / Counting Guard |
| :--- | :--- | :--- | :--- | :--- |
| **`messages_outbound`** | Commercial Quota | Outbound message dispatched via WhatsApp or REST API | Authoritative Billable Quota | Increment only on successful message dispatch |
| **`ai_turns`** | Commercial Quota | Completed AI response generation turn | Authoritative Billable Quota | Increment alongside `models.AITurn` creation |
| **`broadcast_recipients`** | Analytics Only | Recipient row dispatched in broadcast campaign | Analytics Only (Deferred Quota) | Increment per dispatched recipient |

> [!IMPORTANT]
> **Commercial Quota & Broadcast Boundary Lock:**
> 1. **No Double-Counting:** An individual broadcast delivery must never consume the same commercial allowance twice through both `broadcast_recipients` and `messages_outbound`.
> 2. **Phase 2B.3 Quota Boundary:** In Phase 2B.3, commercial quota enforcement applies exclusively to `messages_outbound` and `ai_turns`. Broadcast deliveries dispatched via WhatsApp are counted under `messages_outbound` for message quota accounting, while `broadcast_recipients` remains strictly an analytics metric.
> 3. **Commercial Policy Deferral:** Whether broadcast campaigns will be subject to a separate commercial broadcast quota allocation remains an **OPEN COMMERCIAL DECISION** to be finalized in a future billing phase (Phase 2C). No separate broadcast quota gate is enforced in Phase 2B.3.

### 12.2 Atomic Concurrency
To prevent race conditions during concurrent message processing:
```sql
INSERT INTO usage_counters (tenant_id, metric_key, period_start, period_end, used_value, updated_at)
VALUES (?, ?, ?, ?, ?, NOW())
ON DUPLICATE KEY UPDATE used_value = used_value + VALUES(used_value), updated_at = NOW();
```

---

## 13. Tenant 1 Grandfathering Plan

### 13.1 Production Baseline of Tenant 1
- **Tenant ID:** 1 ("Default")
- **Active Agent:** Agent 3 ("Admin J&T Express Batang", `number: 6282211700060`)
- **Active History:** 29,941 chat messages, 5,163 read states, 532 contacts
- **Active WhatsApp Session:** `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`

### 13.2 Grandfathering Execution Strategy & Invariants
Tenant 1 is the existing single-tenant live production workspace. Under Phase 2B, Tenant 1 is classified as a **migration-time grandfathered/manual administrative exception** whose persisted representation must remain fully compatible with the canonical subscription schema without altering the general multi-tenant architecture.

#### Canonical Invariants:
1. **Canonical Schema Alignment:**
   - Tenant 1 uses the canonical **`business`** plan (`code = 'business'`).
   - No new plan codes (e.g. `enterprise`, `grandfathered`, `lifetime`) are introduced.
   - No new database columns or schema flags (e.g. `is_grandfathered`) are added. The generic subscription schema itself does not semantically identify Tenant 1 as grandfathered.
2. **Persisted Record Values:**
   - `tenants.slug = 'default'`
   - `tenants.status = 'active'` (NOT trialing)
   - `tenants.trial_ends_at = NULL`
   - `subscriptions.tenant_id = 1`
   - `subscriptions.plan_id = (SELECT id FROM plans WHERE code = 'business')`
   - `subscriptions.status = 'active'`
   - `subscriptions.is_current = 1`
   - `subscriptions.payment_provider = 'manual'`
   - `subscriptions.current_period_start = NOW()`
   - `subscriptions.current_period_end`: In accordance with the canonical database schema, `current_period_end` remains `DATETIME(3) NOT NULL`. Tenant 1's administrative period value is strictly a **technical migration representation and NOT a commercial expiry date**.
3. **Tenant Membership:**
   - Attach User 1 (superadmin) as owner in `tenant_members` (`tenant_id = 1, user_id = 1, role = 'owner'`).

### 13.3 Runtime Boundary & Entitlement Consumption
The runtime distinguishes Tenant 1's administrative exception through service boundary separation rather than schema mutation or generic code bypasses:

1. **Bootstrap / Migration Provisioning Boundary:**
   - Grandfathering is strictly a **controlled bootstrap/migration provisioning concern**.
   - The migration script (`001_phase2b_saas_schema.sql`) and `SubscriptionService` provisioning logic establish Tenant 1's subscription in an active state under the canonical `business` plan.
2. **Runtime Entitlement Evaluation (`EntitlementService`):**
   - Runtime entitlement evaluation is **purely state-driven**. It checks whether the tenant is `active`, whether a current subscription (`is_current = 1`) exists with `status = 'active'`, and whether the associated plan features permit the requested action.
   - `EntitlementService` evaluates the established subscription state directly without inspecting how or why it was provisioned.
3. **Scheduled Reconciliation Boundary:**
   - The scheduled reconciliation engine evaluates subscriptions subject to automated lifecycle transitions (e.g. trialing tenants expiring after 30 days, or payment-gateway subscriptions requiring webhook/billing renewal).
   - Scheduled reconciliation **MUST NOT infer Tenant 1 expiry from the administrative `current_period_end` timestamp**.
   - Manual administrative subscriptions (`payment_provider = 'manual'`) are exempted from automated gateway expiration sweeps at the reconciliation boundary, ensuring Tenant 1 remains continuously active without automated commercial expiry.

### 13.4 Prevention of Generic Tenant-ID Bypass Architecture
The implementation must maintain strict multi-tenant code hygiene. Architecture such as:
```go
// STRICTLY PROHIBITED IN GENERIC ENTITLEMENT
if tenantID == 1 {
    bypassExpiry()
}
```
is **STRICTLY PROHIBITED** inside:
- `EntitlementService`
- Generic quota evaluation services
- Generic subscription state evaluators
- Generic reconciliation engines

Any logic specific to Tenant 1's historical setup is strictly confined to the initial migration script and bootstrap seeding functions. It is **NOT** a reusable or generic entitlement pattern.

---

## 14. Migration Implementation Plan

The canonical 17-step implementation sequence strictly adheres to `docs/SAAS-DATABASE-SPEC-FINAL.md`:

```text
[01. AutoMigrate Governance] ──> [02. Schema Preflight] ──> [03. Backup & Verification]
                                                                     │
[06. Backfill Data] <── [05. Add Additive Columns] <── [04. Create New SaaS Tables]
       │
[07. Add Constraints/Indexes] ──> [08. Seed Plans/Features] ──> [09. Grandfather Tenant 1]
                                                                      │
[12. Compatibility Tests] <── [11. Data Integrity Audit] <── [10. Tenant 1 Membership]
       │
[13. Staging Migration] ──> [14. Staging Isolation Tests] ──> [15. Production Approval]
                                                                      │
                                [17. Production Verification] <── [16. Production Migration]
```

### Step-by-Step Execution Matrix
| Step | Action | Prerequisite | Verification Query / Check | Rollback Consideration |
| :--- | :--- | :--- | :--- | :--- |
| **01** | Implement AutoMigrate governance (`AUTO_MIGRATE=false`) | Code inspection | Build binary, test with `AUTO_MIGRATE=false` | Revert Go file edit |
| **02** | Schema Preflight | DB access | Verify 45 tables, 0 lock blockers | Abort if lock active |
| **03** | Full Database Backup & Verification | Storage check | `mysqldump ruangkirim > backup.sql` & check size | Abort if backup fails |
| **04** | Create New SaaS Tables (`plans`, `subscriptions`, etc.) | Step 03 | `SHOW TABLES LIKE 'plans'` | Controlled schema stop |
| **05** | Add Additive Columns (`tenants.slug`, `contacts.tenant_id`) | Step 04 | `DESCRIBE tenants; DESCRIBE contacts;` | Columns are nullable/defaulted |
| **06** | Backfill Data (`contacts.tenant_id = agents.tenant_id`) | Step 05 | `SELECT COUNT(*) FROM contacts WHERE tenant_id = 0` | Re-run backfill |
| **07** | Add Constraints/Indexes (`idx_contacts_tenant_id`) | Step 06 | `SHOW INDEX FROM contacts` | Controlled index removal |
| **08** | Seed Plans & Plan Features (`trial`, `starter`, `pro`, `business`) | Step 07 | `SELECT COUNT(*) FROM plans` | Delete seeded rows |
| **09** | Create Tenant 1 Grandfathered Subscription | Step 08 | `SELECT * FROM subscriptions WHERE tenant_id = 1` | Delete subscription row |
| **10** | Create Tenant 1 Membership for User 1 | Step 09 | `SELECT * FROM tenant_members WHERE tenant_id = 1` | Delete member row |
| **11** | Validate Data Integrity | Step 10 | Run validation SQL suite | Stop if anomalies found |
| **12** | Local / Staging Compatibility Testing | Step 11 | Run Go unit/integration test suite | Fix application issues |
| **13** | Execute Migration on Staging (`ruangkirim_staging`) | Step 12 | Verify staging API and WhatsApp | Restore staging dump |
| **14** | Execute Staging SaaS Multi-Tenant Isolation Tests | Step 13 | Run automated isolation test suite | Fix code defects |
| **15** | Production Deployment Approval Gate | Step 14 | Explicit sign-off checkpoint | Do not proceed without approval |
| **16** | Execute Migration on Production (`ruangkirim`) | Step 15 | Run production DDL script | Follow defined rollback procedure |
| **17** | Production Verification & Sanity Check | Step 16 | Monitor WhatsApp traffic, Agent 3 health | Notify stakeholders |

---

## 15. Application Implementation Plan

In Phase 2B.3, code modifications will be grouped into distinct layers:

### Layer 1: Core Models (`backend/models/`)
- Update `models.Tenant` (`Slug`, `Status`, `TrialEndsAt`).
- Update `models.User` (maintain backward compatibility, reference `tenant_members`).
- Update `models.Knowledge` (`TenantID uint`, `AgentID *uint`).
- Update `models.Contact` (`TenantID uint`).
- Introduce new structs: `Plan`, `PlanFeature`, `Subscription`, `TenantMember`, `UsageCounter`, `AuditLog`.

### Layer 2: Database & AutoMigrate Governance (`backend/database/`)
- Support `AUTO_MIGRATE=false` environment control.
- Add read-only preflight schema validation on boot when `AUTO_MIGRATE=false`.
- Remove destructive seeder overwrites in `seedDefaultTenant()` line 1066.

### Layer 3: Entitlement & Subscription Service (`backend/services/`)
- Create `backend/services/entitlement.go`:
  - `CanUseFeature(tenantID uint, feature string) bool`
  - `CanConsumeQuota(tenantID uint, metric string, count int) bool`
  - `ConsumeQuota(tenantID uint, metric string, count int) error`
  - `GetActiveSenderLimit(tenantID uint) (int, error)`
  - `GetTenantSubscriptionState(tenantID uint) (string, error)`
  - **Boundary Rule:** Purely state-driven evaluation consuming active subscription and plan feature records. Strictly prohibits hardcoded `tenantID == 1` bypass checks.
- Create `backend/services/subscription.go`:
  - Handles subscription creation, promotion, demotion, and transactional current-record switching.
  - Manages isolated migration bootstrap provisioning for Tenant 1 without bleeding exceptions into generic entitlement.
  - Enforces authoritative commercial quota metrics (`messages_outbound`, `ai_turns`), while deferring separate broadcast quota gating.

### Layer 4: Authentication & Context Middleware (`backend/handlers/`, `backend/middleware/`)
- Decouple Super Admin from Tenant 1 fallback.
- Validate active tenant membership via `tenant_members`.
- Implement `POST /api/auth/switch-tenant`.

### Layer 5: Handlers & Services Scoping
- Update `backend/services/embedding.go`: implement deterministic knowledge retrieval (agent-specific override over tenant-wide).
- Update `backend/handlers/numbers.go`: enforce sender limits in `ConnectNumber` and `ConnectPairingNumber`.
- Update `backend/handlers/api_public.go`: enforce feature gates and quota consumption.

---

## 16. Test Strategy

| Test ID | Area | Setup | Action | Expected Result | Priority |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **TS-A** | Authentication | User with 2 memberships | Login with username/password | Returns active tenant context and valid JWT | P0 |
| **TS-B** | Tenant Membership | User removed from tenant | Call protected tenant endpoint | Returns 403 Forbidden | P0 |
| **TS-C** | Tenant Switching | User member of Tenant 1 & 2 | Call `POST /api/auth/switch-tenant` | JWT updated to Tenant 2; context switches | P1 |
| **TS-D** | Tenant Isolation | Tenant A calls Tenant B agent ID | Request `GET /api/agents/:id/contacts` | Returns 404 Not Found (no cross-tenant leak) | P0 |
| **TS-E** | Super Admin Boundary | Super Admin login | Check context `tenant_id` | `tenant_id == 0`; access platform APIs allowed | P0 |
| **TS-F** | Trial Creation | Register new tenant | Inspect `tenants` and `subscriptions` | `status = trialing`, `trial_ends_at = NOW()+30d` | P1 |
| **TS-G** | Trial Expiration | Subscription period expired | Call API / send message | Returns quota/trial expired error | P1 |
| **TS-H** | Subscription Activation| Upgrade tenant to paid plan | Update subscription record | `status = active`, plan features enabled | P1 |
| **TS-I** | Subscription Expiration| Paid sub period expires | Scheduled reconciliation | Transitions to `past_due` then `suspended` | P2 |
| **TS-J** | Past Due Grace Period | Tenant in `past_due` | Test inbound vs outbound message | Inbound recorded; outbound warns/blocks | P2 |
| **TS-K** | Suspension | Tenant in `suspended` | Call dashboard / API endpoints | Read-only mode; outbound blocked | P1 |
| **TS-L** | Tenant 1 Grandfathering| Inspect Tenant 1 | Check limits & status | Active on canonical business tier, exempt from automated expiry | P0 |
| **TS-M** | Sender Quota (Trial) | Trial tenant connects 1 agent | Attempt connecting 2nd agent | 2nd connection rejected (limit = 1) | P0 |
| **TS-N** | Sub Concurrency | 2 threads activate current sub | Concurrent `is_current = 1` updates | At most one succeeds; UK constraint enforced | P0 |
| **TS-O** | Usage Counters | Send 5 outbound messages | Inspect `usage_counters` table | Count atomically equals 5 | P1 |
| **TS-P** | Knowledge Tenant Scope | Create knowledge `agent_id = NULL`| Agent 3 asks matching question | AI answers using tenant-wide knowledge | P0 |
| **TS-Q** | Knowledge Agent Override| Tenant & Agent have same question| Ask matching question | Agent-specific answer deterministically overrides | P1 |
| **TS-R** | Knowledge NULL agent_id| Insert knowledge `agent_id = NULL`| Reboot backend service | `agent_id` remains NULL (not overwritten to 1)| P0 |
| **TS-S** | Contact tenant_id | Create contact via API/Inbox | Inspect `contacts.tenant_id` | Automatically equals `agent.tenant_id` | P1 |
| **TS-T** | API Feature Gate | Plan without API feature | Call `/api/v1/messages` | Returns 403 Feature Not Allowed | P1 |
| **TS-U** | Webhook Feature Gate | Plan without Webhook feature | Configure Webhook URL | Returns 403 Feature Not Allowed | P2 |
| **TS-V** | User Compatibility | Existing User 1 (superadmin) | Login with existing password | Succeeds without disruption | P0 |
| **TS-W** | Agent 3 Compatibility | Existing Agent 3 in prod | Send and receive WhatsApp chats | Fully operational, history intact | P0 |
| **TS-X** | WhatsApp Preservation | Execute full migration | Inspect SQLite session file | Session untouched, connection preserved | P0 |

---

## 17. Staging Rollout Plan

To ensure thorough verification prior to production execution, the rollout follows an exact multi-gate sequence:

```text
Implementation
      ↓
Local tests
      ↓
Migration SQL review
      ↓
SQL syntax & idempotency validation
      ↓
Staging backup
      ↓
Staging migration execution
      ↓
Staging application deployment
      ↓
SaaS multi-tenant isolation tests
      ↓
Regression tests
      ↓
24-hour observation / soak period
      ↓
Production approval gate
      ↓
Production backup verification
      ↓
Production migration execution
      ↓
Production verification
```

1. **SQL Review & Preflight:** Verify migration scripts are strictly idempotent and syntactically valid against MySQL 8.0.
2. **Staging Backup:** Take full cold snapshot of `ruangkirim_staging` and verify file size.
3. **Execute Migration:** Apply DDL and data backfill scripts on Staging. Measure and record execution duration.
4. **Deploy Staging Binary:** Deploy Phase 2B.3 binary with `AUTO_MIGRATE=false`.
5. **Run Isolation & Regression Suite:** Execute TS-A through TS-X.
6. **24-Hour Soak Period:** Monitor error logs and WhatsApp stability under synthetic load.

---

## 18. Production Rollout Plan

1. **Production Freeze & Preflight:**
   - Announce maintenance window.
   - Verify zero pending crawl jobs.
2. **Production WhatsApp Safety Protocol:**
   - **Migration procedure must exclude WhatsApp session files from mutation scope.**
   - Session integrity must be verified before and after deployment.
   - Any unexpected WhatsApp session mutation is a production incident condition requiring immediate investigation.
   - Do NOT modify the session file (`/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`).
   - Do NOT calculate, replace, or relocate session contents.
3. **Take Complete Production Backup & Verify:**
   - `sudo mysqldump -u root ruangkirim > /var/backups/ruangkirim/pre_phase2b_dump.sql`
   - Verify dump integrity and verify table row count.
4. **Execute Production SQL Migration:**
   - Run approved Phase 2B migration script via MySQL CLI.
   - Migration duration must be measured during staging and recorded before production approval.
5. **Deploy Production Binary:**
   - Backup current binary: `cp ruangkirim-server ruangkirim-server.bak.phase2a`
   - Deploy new binary: `/var/www/ruangkirim/ruangkirim-server`
   - Set `AUTO_MIGRATE=false` in production `.env`.
   - Restart service: `sudo systemctl restart ruangkirim.service`
6. **Post-Deployment Verification:**
   - Verify HTTP status: `curl http://localhost:3031/health`
   - Verify WhatsApp connection: Agent 3 remains connected.
   - Test outbound chat and verify `chat_histories` write.
   - Check `systemctl status ruangkirim.service` and journal logs.

---

## 19. Rollback Strategy

The rollback plan is divided into three distinct operational categories based on failure mode:

### Category A: Application Rollback
**Trigger:** Application binary panics, fails startup preflight, or exhibits regression while database schema changes remain backward compatible.
**Procedure:**
1. Restore previous known-good binary: `cp ruangkirim-server.bak.phase2a ruangkirim-server`.
2. Restart only the affected application service: `sudo systemctl restart ruangkirim.service`.
3. Verify application health check (`/health`).
4. Verify WhatsApp agent connectivity and message processing.
5. Confirm backward compatibility with database schema.

### Category B: Migration Stop / Forward Fix
**Trigger:** DDL execution partially fails or halts before application depends on the new schema.
**Procedure:**
1. Stop migration immediately.
2. Inspect exact database state via `information_schema`.
3. **Do not automatically drop production structures** (avoid unmanaged `DROP TABLE` or `DROP COLUMN` assumptions).
4. Determine whether the migration script can safely resume via forward fix or requires a controlled, audited rollback script.

### Category C: Database Restore
**Trigger:** Unrecoverable data corruption occurred, destructive migration mistake occurred, or data integrity validation fails materially.
**Procedure:**
1. Requires verified pre-migration backup, explicit executive authorization, and a controlled maintenance window.
2. Stop application service: `sudo systemctl stop ruangkirim.service`.
3. Restore database from pre-migration backup dump: `mysql -u root ruangkirim < /var/backups/ruangkirim/pre_phase2b_dump.sql`.
4. Validate table counts (45 tables) and row counts.
5. Re-point application binary to previous known-good version.
6. Restart service and execute post-restore verification.

---

## 20. Risk Register & Implementation Blockers

### 20.1 Risk Register

| Risk ID | Severity | Status | Description & Mitigation |
| :--- | :--- | :--- | :--- |
| **RSK-01** | **P0** | READY FOR IMPLEMENTATION | **AutoMigrate Uncontrolled Boot:** Unmanaged AutoMigrate mutates schema automatically. Mitigated by `AUTO_MIGRATE=false` governance. |
| **RSK-02** | **P0** | READY FOR IMPLEMENTATION | **Seeder Orphan Knowledge Mutation:** `database.go:1066` overwrites NULL `agent_id` to 1. Mitigated by removing this update line in Phase 2B.3. |
| **RSK-03** | **P0** | READY FOR IMPLEMENTATION | **Super Admin Conflation:** Super Admin defaults to Tenant 1. Mitigated by decoupling platform claims (`tenant_id = 0`). |
| **RSK-04** | **P0** | VERIFIED | **WhatsApp Session Disruption:** WhatsApp session path could be corrupted. Mitigated by strictly excluding session files from migration scope. |
| **RSK-05** | **P1** | READY FOR IMPLEMENTATION | **Subscription `is_current` Race Condition:** Concurrent subscription updates. Mitigated by InnoDB transactions and unique key `(tenant_id, is_current)`. |
| **RSK-06** | **P1** | READY FOR IMPLEMENTATION | **Large Table Lock:** DDL on `chat_histories` causing downtime. Mitigated by strictly omitting `chat_histories` from DDL changes. |
| **RSK-07** | **P2** | READY FOR IMPLEMENTATION | **User Email Uniqueness Conflict:** Missing or duplicate emails breaking authentication. Mitigated by staged email migration (Phase A: keep nullable). |

### 20.2 Explicit Implementation Blockers
The following conditions must be resolved before production DDL execution:

- **BLOCKER 1:** AutoMigrate governance (`AUTO_MIGRATE=false` runtime support) must be implemented and verified in code.
- **BLOCKER 2:** The knowledge orphan/NULL mutation in `seedDefaultTenant()` (`database.go:1066`) must be removed before tenant-wide knowledge is enabled.
- **BLOCKER 3:** Knowledge model, query, and cache changes (supporting nullable `agent_id` and deterministic overrides) must be implemented and tested before tenant-wide knowledge migration is considered safe.
- **BLOCKER 4:** Migration backup and restore procedure must be validated on staging before executing any production DDL.
- **BLOCKER 5:** Super Admin tenant-context separation must be implemented and tested before multi-tenant customer access is enabled.
- **BLOCKER 6:** Subscription current-record transaction semantics must be implemented and tested before subscription activation workflows.
- **BLOCKER 7:** Any unresolved commercial policy affecting quota semantics must be decided before implementing quota enforcement that depends on it.

---

## 21. Open Questions

1. **Payment Gateway Provider (Phase 2C):**
   Which payment gateway provider (e.g., Midtrans, Xendit, Tripay) will be integrated in Phase 2C for automated recurring billing?
   *Current Status:* Open commercial decision. Phase 2B implements the subscription state machine and manual billing reconciliation.
2. **Inbound WhatsApp Metering Policy:**
   - Inbound messages: RECOMMENDED as unmetered (free customer communication).
   - Outbound messages: Authoritative commercial quota metric (`messages_outbound`).
   - AI turns: Commercial quota metric (`ai_turns`).
   - Broadcast recipients: Analytics-only metric for Phase 2B.3 (`broadcast_recipients`).
   *Current Status:* Inbound processing recommended as unmetered.
3. **Broadcast Quota Semantics vs General Outbound Quota:**
   Whether broadcast delivery additionally consumes a separate commercial quota remains an **OPEN COMMERCIAL DECISION**.
   *Phase 2B.3 Boundary Lock:* In Phase 2B.3, separate broadcast quota enforcement is deferred. Broadcast deliveries dispatched over WhatsApp are accounted under `messages_outbound`, while `broadcast_recipients` tracks recipient volume strictly for analytics. A single broadcast delivery must never consume commercial quota twice across multiple metrics.
4. **Scheduled Reconciliation Architecture:**
   Should subscription and trial expiration reconciliation execute as an internal background Go ticker or an external systemd timer / cron calling an internal endpoint?
   *Recommended Policy:* Internal Go ticker running every 1 hour, complemented by request-time lazy evaluation during entitlement checks.
5. **Inbound Processing Behavior During Suspension:**
   When a tenant is suspended, should customer inbound chats still be received?
   *Recommended Policy:* Silently receive and store inbound messages in `chat_histories` to preserve customer conversation history, while blocking human outbound responses and disabling AI auto-replies.

---

## 22. Phase 2B.3 Recommended Implementation Order

Phase 2B.3 execution must strictly preserve the following 21-step safety sequence:

1. **AutoMigrate Governance:** Implement `AUTO_MIGRATE=false` flag and preflight schema verification in `backend/database/database.go`.
2. **Remove Unsafe Startup Mutations:** Remove destructive `agent_id = 1` overwrites in `seedDefaultTenant()`.
3. **Core Models:** Extend GORM model structs in `backend/models/` for `Tenant`, `Knowledge`, `Contact`, and new SaaS entities.
4. **Database Migration Scripts:** Author standalone, idempotent SQL migration and rollback scripts (`migrations/001_phase2b_saas_schema.sql`).
5. **Entitlement/Subscription Service:** Build `backend/services/entitlement.go` to replace `plan_features.go` stubs.
6. **Authentication & Tenant Membership:** Update `backend/handlers/auth.go` to enforce `tenant_members` and decouple Super Admin from Tenant 1.
7. **Tenant Isolation Enforcement:** Audit all handlers to ensure strict tenant scoping.
8. **Knowledge/RAG Tenant-Wide Scope:** Refactor `backend/services/embedding.go` to support tenant-wide knowledge with deterministic overrides.
9. **Contact Tenant Scope:** Update contact creation to populate `tenant_id` from agent context.
10. **Sender Quota:** Add quota checks to `backend/handlers/agents.go` and `numbers.go`.
11. **Usage Counters:** Integrate atomic counter increments for outbound messages and AI turns.
12. **API / Webhook Feature Gates:** Add entitlement gates to public REST API and webhook configuration.
13. **Audit Logging:** Integrate `audit_logs` recordings for administrative and subscription events.
14. **Automated Tests:** Execute test suite TS-A through TS-X locally.
15. **Staging Migration:** Execute SQL migration script against `ruangkirim_staging`.
16. **Staging Deployment:** Deploy Phase 2B.3 binary to staging environment.
17. **Staging Isolation & Regression Testing:** Verify multi-tenant isolation, sender limits, and trial lifecycle.
18. **Observation Period:** Monitor staging runtime stability for 24 hours.
19. **Production Approval Gate:** Review staging results and obtain explicit authorization.
20. **Production Migration:** Execute SQL migration script against `ruangkirim` production database.
21. **Production Verification:** Confirm service health, table parity (51 tables), and active WhatsApp Agent 3 connectivity.
