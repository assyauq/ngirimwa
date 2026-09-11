# PHASE 2B.2 — SAAS IMPLEMENTATION PLAN & RUNTIME AUDIT
**Project:** RUANGKIRIM  
**Repository:** `mrifatsyauqi/ruangkirim`  
**Checkpoint:** Phase 2B.2 (Architecture Audit, Runtime Gap Analysis & Implementation Planning)  
**Status:** READY FOR IMPLEMENTATION REVIEW  
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
5. **Zero Quota Enforcement (P1 - Commercial Requirement):**  
   `backend/handlers/plan_features.go` contains stubs `tenantPlanAllows()` and `agentPlanAllows()` that unconditionally return `true`. Sender creation (`CreateAgent`) and WhatsApp pairing (`ConnectNumber`, `ConnectPairingNumber`) contain no quota or subscription checks.
6. **Zero-Downtime Safe Rollout Path:**  
   Because all planned Phase 2B database changes are strictly additive (new tables, new nullable/defaulted columns, and explicit backfills), and because `chat_histories` (29,941 rows) and `inbox_read_states` (5,163 rows) are preserved without DDL mutation, the migration can be executed safely with near-zero lock contention and trivial instant rollback capability.

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
| **`AUTO_MIGRATE` Env Flag**| **NEVER READ** in Go code | Setting `AUTO_MIGRATE=false` in `.env` has **ZERO** effect |
| **Production Impact** | AutoMigrate runs automatically on production deploy | Any incompatible struct tag can crash production |
| **Staging Impact** | Identical to production | Runs automatically on every staging reboot |
| **Test Impact** | Unit tests invoking `database.Init()` execute AutoMigrate | Modifies connected test DB |
| **Other DDL Mutations**| `ensureCanonicalChatMessageIDs()` runs raw `ALTER TABLE` | Raw DDL executed on every boot |
| **Data Mutations on Boot**| `seedDefaultTenant()` line 1066 overwrites NULL `agent_id` | **P0 Blocker:** Overwrites tenant-wide knowledges to `agent_id = 1` |

### 4.3 AutoMigrate Governance Design for Phase 2B.3
To meet ADR-10 requirements without breaking local developer velocity:
1. Introduce `config.EnvBool("AUTO_MIGRATE", false)` in `backend/database/database.go`.
2. In production systemd configuration (`ruangkirim.service`), set `AUTO_MIGRATE=false`.
3. In `database.Init()`:
   - If `AUTO_MIGRATE=false`: Skip `DB.AutoMigrate(...)`. Execute schema preflight validation only (verifies required tables and columns exist without modifying them). If required schema elements are missing, abort with explicit log.
   - If `AUTO_MIGRATE=true`: Execute controlled additive migration.
4. Eliminate unconditional mutation queries in `seedDefaultTenant()` lines 1065-1067:
   - Remove `UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;` so tenant-wide knowledge is preserved.
   - Replace with safe orphan re-parenting only when a record lacks both `tenant_id` and `agent_id`.

---

## 5. SaaS Architecture Gap Analysis

| SaaS Architecture Requirement | Current State in Repository | Target Phase 2B Specification | Implementation Gap / Action |
| :--- | :--- | :--- | :--- |
| **Multi-Tenancy** | Single tenant in practice (Tenant 1 "Default") | Multi-tenant workspace isolation with slug and lifecycle | Add `slug`, `status`, `trial_ends_at` to `tenants` |
| **Tenant Membership** | 1:1 `users.tenant_id` foreign key column | N:M `tenant_members` junction table with roles | Create `tenant_members` table; backfill User 1 |
| **Plans & Packaging** | No plan tables; hardcoded strings | Canonical `plans` and `plan_features` tables | Create `plans`, `plan_features`; seed plans |
| **Subscriptions** | Missing | `subscriptions` table with `is_current` unique index | Create `subscriptions` table; create Grandfathered sub |
| **Entitlement Service** | Stubs in `plan_features.go` returning `true` | Centralized `EntitlementService` interface | Implement `EntitlementService` in `backend/services/` |
| **Trial Management** | None | 30-day trial, max 1 active sender, trialing lifecycle | Enforce trial limits in EntitlementService |
| **Sender Limit** | Hardcoded comment "tidak ada batas jumlah nomor" | Governed by plan quota (`GetActiveSenderLimit`) | Enforce sender quota on Agent Create & WA Connect |
| **Usage Tracking** | None | `usage_counters` per (tenant, metric, period_start) | Create `usage_counters`; meter outbound & AI turns |
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
2. **Super Admin Auto-Fallback Hazard (P0):** A superadmin logging in is automatically injected with `tenant_id = 1` and `role = admin`. This bleeds platform administration into customer tenant context.
   - *Target:* Super Admin JWT claims should contain `tenant_id = 0` (or omitted) and `is_super_admin = true`. When operating on platform routes (`/settings/api-config`, `/admin/*`), no tenant context is required. When explicitly viewing a tenant, Super Admin must specify an explicit impersonation/context header (`X-Tenant-Context: <id>`), validated against audit logs.
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

#### Finding 4: In-Memory RAG Retrieval Cache
- **File:** `backend/services/embedding.go:29, 43-68` (`KnowledgeFor`)
- **Current Behavior:**
  ```go
  var kbCache = map[uint][]KBItem{} // Keyed by agentID uint
  database.DB.Where("agent_id = ? AND active = ?", agentID, true).Find(&rows)
  ```
- **Classification:** **Class C (ASSUMES NON-NULL)**
- **Why it matters:** Queries DB strictly for `agent_id = ?`. Any tenant-wide knowledge (`agent_id IS NULL`) is completely omitted from the RAG context!
- **Required Change:**
  1. Retrieve `agent.TenantID` for the given `agentID`.
  2. Query `WHERE tenant_id = ? AND (agent_id = ? OR agent_id IS NULL) AND active = ?`.
  3. Apply override policy: if an agent-specific knowledge item matches a tenant-wide knowledge question, the agent item takes precedence.
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

### 12.1 Metered Events & Injection Points
| Metric | Event Trigger | Source Code File / Location | Idempotency / Counting Guard |
| :--- | :--- | :--- | :--- |
| **`messages_outbound`** | Successful outbound message send | `backend/services/wa.go:4034` / `handlers/api_public.go` | Increment only on non-error WhatsApp dispatch |
| **`ai_turns`** | Completed AI response generation | `backend/handlers/ai_metrics.go:26` | Increment alongside `models.AITurn` creation |
| **`broadcast_recipients`** | Dispatch to broadcast recipient | `backend/handlers/broadcast.go` (`sendBroadcastChunk`) | Increment per successfully sent recipient row |

### 12.2 Atomic Concurrency
To prevent race conditions during high concurrent message throughput:
```sql
INSERT INTO usage_counters (tenant_id, metric, period_start, count, updated_at)
VALUES (?, ?, ?, ?, NOW())
ON DUPLICATE KEY UPDATE count = count + VALUES(count), updated_at = NOW();
```

---

## 13. Tenant 1 Grandfathering Plan

### 13.1 Production Baseline of Tenant 1
- **Tenant ID:** 1 ("Default")
- **Active Agent:** Agent 3 ("Admin J&T Express Batang", `number: 6282211700060`)
- **Active History:** 29,941 chat messages, 5,163 read states, 532 contacts
- **Active WhatsApp Session:** `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`

### 13.2 Grandfathering Execution Strategy
1. **Tenant Attributes:**
   - `tenants.slug = 'default'`
   - `tenants.status = 'active'` (NOT trialing!)
   - `tenants.trial_ends_at = NULL` (No artificial commercial expiration)
2. **Grandfathered Subscription:**
   - Create entry in `subscriptions`:
     - `tenant_id = 1`
     - `plan_id = (SELECT id FROM plans WHERE code = 'enterprise')` (or `grandfathered`)
     - `status = 'active'`
     - `is_current = 1`
     - `starts_at = NOW()`
     - `ends_at = NULL` (Lifetime / unmetered renewal)
3. **Tenant Membership:**
   - Create entry in `tenant_members`:
     - `tenant_id = 1`, `user_id = 1`, `role = 'owner'`
4. **Safety Verification:**
   - Zero commercial popups or quota restrictions for Tenant 1.
   - Agent 3 remains connected and operational throughout.

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
| **04** | Create New SaaS Tables (`plans`, `subscriptions`, etc.) | Step 03 | `SHOW TABLES LIKE 'plans'` | `DROP TABLE IF EXISTS ...` |
| **05** | Add Additive Columns (`tenants.slug`, `contacts.tenant_id`) | Step 04 | `DESCRIBE tenants; DESCRIBE contacts;` | Columns are nullable/defaulted |
| **06** | Backfill Data (`contacts.tenant_id = agents.tenant_id`) | Step 05 | `SELECT COUNT(*) FROM contacts WHERE tenant_id = 0` | Re-run backfill |
| **07** | Add Constraints/Indexes (`idx_contacts_tenant_id`) | Step 06 | `SHOW INDEX FROM contacts` | `DROP INDEX ...` |
| **08** | Seed Plans & Plan Features | Step 07 | `SELECT COUNT(*) FROM plans` | `DELETE FROM plans` |
| **09** | Create Tenant 1 Grandfathered Subscription | Step 08 | `SELECT * FROM subscriptions WHERE tenant_id = 1` | `DELETE FROM subscriptions` |
| **10** | Create Tenant 1 Membership for User 1 | Step 09 | `SELECT * FROM tenant_members WHERE tenant_id = 1` | `DELETE FROM tenant_members` |
| **11** | Validate Data Integrity | Step 10 | Run validation SQL suite | Stop if anomalies found |
| **12** | Local / Staging Compatibility Testing | Step 11 | Run Go unit/integration test suite | Fix application issues |
| **13** | Execute Migration on Staging (`ruangkirim_staging`) | Step 12 | Verify staging API and WhatsApp | Restore staging dump |
| **14** | Execute Staging SaaS Multi-Tenant Isolation Tests | Step 13 | Run automated isolation test suite | Fix code defects |
| **15** | Production Deployment Approval Gate | Step 14 | Explicit sign-off checkpoint | Do not proceed without approval |
| **16** | Execute Migration on Production (`ruangkirim`) | Step 15 | Run production DDL script | Execute instant rollback plan |
| **17** | Production Verification & Sanity Check | Step 16 | Monitor WhatsApp traffic, Agent 3 health | Notify stakeholders |

---

## 15. Application Implementation Plan

In Phase 2B.3, code modifications will be grouped into distinct layers:

### Layer 1: Core Models (`backend/models/`)
- Update `models.Tenant` (`Slug`, `Status`, `TrialEndsAt`).
- Update `models.User` (maintain compatibility, reference `tenant_members`).
- Update `models.Knowledge` (`TenantID uint`, `AgentID *uint`).
- Update `models.Contact` (`TenantID uint`).
- Introduce new structs: `Plan`, `PlanFeature`, `Subscription`, `TenantMember`, `UsageCounter`, `AuditLog`.

### Layer 2: Database & AutoMigrate Governance (`backend/database/`)
- Support `AUTO_MIGRATE=false` environment control.
- Add preflight schema validation on boot when `AUTO_MIGRATE=false`.
- Remove destructive seeder overwrites in `seedDefaultTenant()` line 1066.

### Layer 3: Entitlement & Subscription Service (`backend/services/`)
- Create `backend/services/entitlement.go`:
  - `CanUseFeature(tenantID uint, feature string) bool`
  - `CanConsumeQuota(tenantID uint, metric string, count int) bool`
  - `ConsumeQuota(tenantID uint, metric string, count int) error`
  - `GetActiveSenderLimit(tenantID uint) (int, error)`
  - `GetTenantSubscriptionState(tenantID uint) (string, error)`

### Layer 4: Authentication & Context Middleware (`backend/handlers/`, `backend/middleware/`)
- Decouple Super Admin from Tenant 1 fallback.
- Validate active tenant membership via `tenant_members`.
- Implement `POST /api/auth/switch-tenant`.

### Layer 5: Handlers & Services Scoping
- Update `backend/services/embedding.go`: support tenant-wide knowledge in `KnowledgeFor`.
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
| **TS-G** | Trial Expiration | Subscription `ends_at` < NOW() | Call API / send message | Returns quota/trial expired error | P1 |
| **TS-H** | Subscription Activation| Upgrade tenant to paid plan | Update subscription record | `status = active`, plan features enabled | P1 |
| **TS-I** | Subscription Expiration| Paid sub reaches `ends_at` | Scheduled reconciliation | Transitions to `past_due` then `suspended` | P2 |
| **TS-J** | Past Due Grace Period | Tenant in `past_due` | Test inbound vs outbound message | Inbound recorded; outbound warns/blocks | P2 |
| **TS-K** | Suspension | Tenant in `suspended` | Call dashboard / API endpoints | Read-only mode; outbound blocked | P1 |
| **TS-L** | Tenant 1 Grandfathering| Inspect Tenant 1 | Check limits & status | Active, no expiration, no commercial blocks | P0 |
| **TS-M** | Sender Quota (Trial) | Trial tenant connects 1 agent | Attempt connecting 2nd agent | 2nd connection rejected (limit = 1) | P0 |
| **TS-N** | Sub Concurrency | 2 threads activate current sub | Concurrent `is_current = 1` updates | Exactly one succeeds; UK constraint enforced | P0 |
| **TS-O** | Usage Counters | Send 5 outbound messages | Inspect `usage_counters` table | Count atomically equals 5 | P1 |
| **TS-P** | Knowledge Tenant Scope | Create knowledge `agent_id = NULL`| Agent 3 asks matching question | AI answers using tenant-wide knowledge | P0 |
| **TS-Q** | Knowledge Agent Override| Tenant & Agent have same question| Ask matching question | Agent-specific answer overrides tenant answer | P1 |
| **TS-R** | Knowledge NULL agent_id| Insert knowledge `agent_id = NULL`| Reboot backend service | `agent_id` remains NULL (not overwritten to 1)| P0 |
| **TS-S** | Contact tenant_id | Create contact via API/Inbox | Inspect `contacts.tenant_id` | Automatically equals `agent.tenant_id` | P1 |
| **TS-T** | API Feature Gate | Free plan without API feature | Call `/api/v1/messages` | Returns 403 Feature Not Allowed | P1 |
| **TS-U** | Webhook Feature Gate | Plan without Webhook feature | Configure Webhook URL | Returns 403 Feature Not Allowed | P2 |
| **TS-V** | User Compatibility | Existing User 1 (superadmin) | Login with existing password | Succeeds without disruption | P0 |
| **TS-W** | Agent 3 Compatibility | Existing Agent 3 in prod | Send and receive WhatsApp chats | Fully operational, history intact | P0 |
| **TS-X** | WhatsApp Preservation | Execute full migration | Inspect SQLite session file | File unmodified, checksum & size preserved | P0 |

---

## 17. Staging Rollout Plan

1. **Safety Preflight:**
   - Confirm Staging backup exists.
   - Verify Staging WhatsApp session (`wa-session-agent-1.db`) is intact.
2. **Execute Staging Schema Migration:**
   - Execute DDL script on `ruangkirim_staging`.
   - Run data backfill on staging contacts and knowledges.
3. **Deploy Staging Binary:**
   - Deploy Phase 2B.3 binary to `/var/www/ruangkirim-staging/`.
   - Set `AUTO_MIGRATE=false` in staging `.env`.
   - Restart `ruangkirim-staging.service`.
4. **Execute Full Test Suite (TS-A through TS-X):**
   - Verify multi-tenant isolation, trial limits, and sender quotas.
5. **Stability Soak Period:**
   - Monitor staging service logs for 24 hours.

---

## 18. Production Rollout Plan

1. **Production Freeze & Preflight:**
   - Announce maintenance window (estimated duration: 15 minutes).
   - Ensure zero pending background crawl jobs.
2. **Verify WhatsApp Safety:**
   - Confirm `/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db` permissions are read-only to migration scripts.
3. **Take Complete Production Backup:**
   - `sudo mysqldump -u root ruangkirim > /var/backups/ruangkirim/pre_phase2b_dump.sql`
   - Verify dump integrity and row count.
4. **Execute Production SQL Migration:**
   - Run approved Phase 2B migration script via MySQL CLI.
   - Execution time: < 500ms (additive DDL on small tables, zero alterations on `chat_histories`).
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

| Scenario | Rollback Trigger | Execution Steps | Data Impact |
| :--- | :--- | :--- | :--- |
| **Application Crash on Boot** | Binary panic / startup abort | Restore previous binary (`ruangkirim-server.bak.phase2a`); `systemctl restart ruangkirim` | Zero data loss |
| **WhatsApp Connection Loss** | WA fails to reconnect on startup | Stop service; verify SQLite session permissions; restart service | Zero data loss (SQLite untouched) |
| **DDL Migration Failure** | Error during SQL execution | Execute rollback script (drop added tables/columns); binary unchanged | Zero production data loss |
| **Data Corruption during Backfill**| Backfill produces invalid mappings | Restore MySQL database from pre-migration dump; re-point binary | Rollback to pre-migration snapshot |

---

## 20. Risk Register

| Risk ID | Severity | Status | Description & Mitigation |
| :--- | :--- | :--- | :--- |
| **RSK-01** | **P0** | READY FOR IMPLEMENTATION | **AutoMigrate Uncontrolled Boot:** Unmanaged AutoMigrate mutates schema automatically. Mitigated by `AUTO_MIGRATE=false` governance. |
| **RSK-02** | **P0** | READY FOR IMPLEMENTATION | **Seeder Orphan Knowledge Mutation:** `database.go:1066` overwrites NULL `agent_id` to 1. Mitigated by removing this update line in Phase 2B.3. |
| **RSK-03** | **P0** | READY FOR IMPLEMENTATION | **Super Admin Conflation:** Super Admin defaults to Tenant 1. Mitigated by decoupling platform claims (`tenant_id = 0`). |
| **RSK-04** | **P0** | VERIFIED | **WhatsApp Session Disruption:** WhatsApp session path could be corrupted. Mitigated by strictly excluding session files from migration scope. |
| **RSK-05** | **P1** | READY FOR IMPLEMENTATION | **Subscription `is_current` Race Condition:** Concurrent subscription updates. Mitigated by InnoDB transactions and unique key `(tenant_id, is_current)`. |
| **RSK-06** | **P1** | READY FOR IMPLEMENTATION | **Large Table Lock:** DDL on `chat_histories` causing downtime. Mitigated by strictly omitting `chat_histories` from DDL changes. |
| **RSK-07** | **P2** | READY FOR IMPLEMENTATION | **User Email Uniqueness Conflict:** Missing or duplicate emails breaking authentication. Mitigated by staged email migration (Phase A: keep nullable). |

---

## 21. Open Questions

1. **Payment Gateway Integration (Commercial Policy):**  
   Which payment gateway provider (e.g., Midtrans, Xendit, or Tripay) will be implemented in Phase 2C for automated payment processing?  
   *Current Status:* Open commercial decision. Phase 2B implements the subscription state machine and manual billing reconciliation.
2. **Inbound WhatsApp Metering:**  
   Should incoming messages be counted towards usage limits, or should inbound processing remain entirely free/unmetered?  
   *Recommended Policy:* Inbound messages unmetered; only outbound messages, broadcast dispatches, and completed AI turns consume quota.
3. **Scheduled Reconciliation Mechanism:**  
   Should trial and subscription expiration reconciliation be handled via an internal background goroutine / ticker inside the Go backend, or an external systemd timer / cron job calling a protected internal endpoint?  
   *Recommended Policy:* Internal Go ticker running every 1 hour, complemented by request-time lazy evaluation during entitlement checks.

---

## 22. Phase 2B.3 Recommended Implementation Order

To execute Phase 2B.3 with maximum safety and zero risk of regression:

1. **Step 1: AutoMigrate Governance & Safety Refactoring**  
   Implement `AUTO_MIGRATE=false` environment flag in `backend/database/database.go`. Remove line 1066 orphan overwrite.
2. **Step 2: Core Model Extensions**  
   Update GORM model structs in `backend/models/` for `Tenant`, `Knowledge`, `Contact`, `User`, and introduce new SaaS entities.
3. **Step 3: SQL Migration Script Creation**  
   Author standalone, idempotent SQL migration and rollback scripts (`migrations/001_phase2b_saas_schema.sql`).
4. **Step 4: Entitlement & Quota Service Implementation**  
   Build `backend/services/entitlement.go` to replace `plan_features.go` stubs.
5. **Step 5: Authentication & Tenant Membership Refactoring**  
   Update `backend/handlers/auth.go` to enforce `tenant_members` and decouple Super Admin from Tenant 1.
6. **Step 6: Knowledge & RAG Scoping Implementation**  
   Refactor `backend/services/embedding.go` to support tenant-wide knowledge retrieval and overrides.
7. **Step 7: Sender Quota Enforcement**  
   Add quota checks to `backend/handlers/agents.go` and `numbers.go`.
8. **Step 8: Staging Deployment & Multi-Tenant Verification**  
   Execute migration on Staging; run test suite TS-A through TS-X.
9. **Step 9: Production Deployment & Verification**  
   Execute migration on Production following the approved maintenance checklist.
