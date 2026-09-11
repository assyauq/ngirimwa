# PHASE 2B.3.1 — AUTOMIGRATE GOVERNANCE & STARTUP MUTATION SAFETY
**Project:** RUANGKIRIM  
**Repository:** `mrifatsyauqi/ruangkirim`  
**Branch:** `develop`  
**Status:** COMPLETED & VERIFIED LOCALLY  

---

## 1. Objective
The objective of **Phase 2B.3.1** is to establish absolute startup safety and migration governance before executing explicit multi-tenant SaaS database migrations. This ensures that:
1. Application boot in production never executes uncontrolled schema-changing DDL or unmanaged data mutations.
2. The `AUTO_MIGRATE` environment variable is strictly parsed and enforced in Go runtime code.
3. When `AUTO_MIGRATE=false` (the production-safe default), startup performs only a read-only schema preflight and halts safely if required tables or columns are absent.
4. Unsafe orphan mutations (specifically reassigning `knowledges.agent_id` from NULL/0 to Agent 1) are permanently eliminated.

---

## 2. Previous Unsafe Behavior
Prior to Phase 2B.3.1, application startup exhibited several critical governance defects:
- **Uncontrolled AutoMigrate:** `DB.AutoMigrate(...)` in `backend/database/database.go:71` executed unconditionally on every process boot.
- **Unread Environment Flag:** Setting `AUTO_MIGRATE=false` in `.env` had zero effect because the flag was never parsed or checked in Go source code.
- **Raw Startup DDL:** `rootDB.Exec("CREATE DATABASE IF NOT EXISTS ...")`, `ALTER TABLE chat_histories ...`, and `CREATE UNIQUE INDEX uidx_chat_agent_wa_key` executed during application boot regardless of environment.
- **Destructive Knowledge Orphan Mutation:** `seedDefaultTenant()` (`database.go:1059, 1066`) unconditionally executed:
  ```sql
  UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;
  ```
  This destroyed valid `agent_id = NULL` state, which is required for tenant-wide shared knowledge in Phase 2B.
- **Unmanaged Chat History Reassignment:** `seedDefaultTenant()` (`database.go:1067`) unconditionally executed `UPDATE chat_histories SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL`, attempting to reassign chat history to Agent 1 (which does not exist in production, where Agent 3 is the active agent).

---

## 3. New `AUTO_MIGRATE` Behavior
A strict, centralized migration switch is established in `backend/config/config.go` and consumed by `backend/database/database.go`:
- **Default Value:** `false` (production-safe default).
- **Accepted Truthy Values:** `"1"`, `"true"`, `"yes"`, `"on"`.
- **Accepted Falsy Values:** `""` (unset), `"0"`, `"false"`, `"no"`, `"off"`.
- **Helper Function:** `config.AutoMigrateEnabled() bool`
- **Design Rule:** No implicit environment guessing (e.g. `APP_ENV == "production"`). `AUTO_MIGRATE` is the single authoritative migration switch.

---

## 4. Startup Flow: Before vs. After

### Before (Uncontrolled Boot)
```text
database.Init()
  ├── Connect root DSN -> Exec CREATE DATABASE IF NOT EXISTS
  ├── Connect target DSN
  ├── preflightCanonicalChatSchema()
  ├── DB.AutoMigrate(45 models...) [UNCONTROLLED DDL]
  ├── backfillKnowledgeCharCount()
  ├── backfillHistoricalDeliveryStatus()
  ├── backfillInboxLastMsgAt()
  ├── normalizeSenderFields()
  ├── ensureCanonicalChatMessageIDs() -> ALTER TABLE + CREATE UNIQUE INDEX [UNCONTROLLED DDL]
  ├── recoverStuckCrawlJobs()
  ├── seedSuperAdmin()
  └── seedDefaultTenant()
        ├── UPDATE knowledges SET agent_id = 1 [UNSAFE MUTATION]
        └── UPDATE chat_histories SET agent_id = 1 [UNMANAGED MUTATION]
```

### After (Governed Boot)
```text
database.Init()
  ├── Read autoMigrate := config.AutoMigrateEnabled()
  │
  ├── [IF AUTO_MIGRATE=false (Production Safe)]
  │     ├── Connect directly to target DSN (NO root connection, NO CREATE DATABASE)
  │     ├── preflightCurrentSchema() [READ-ONLY: 45 tables + canonical chat schema]
  │     │     └── If missing/invalid -> fatalDatabaseStartup() & exit
  │     └── Skip AutoMigrate, Skip ensureCanonicalChatMessageIDs() DDL
  │
  ├── [IF AUTO_MIGRATE=true (Developer Migration Mode)]
  │     ├── Connect root DSN -> CREATE DATABASE IF NOT EXISTS
  │     ├── Connect target DSN
  │     ├── preflightCanonicalChatSchema()
  │     ├── DB.AutoMigrate(45 models...)
  │     └── ensureCanonicalChatMessageIDs() [DDL: wa_msg_key + unique index]
  │
  ├── backfillKnowledgeCharCount() [Safe/Idempotent]
  ├── backfillHistoricalDeliveryStatus() [Safe/Idempotent]
  ├── backfillInboxLastMsgAt() [Safe/Idempotent]
  ├── normalizeSenderFields() [Safe/Idempotent]
  ├── recoverStuckCrawlJobs() [Operational recovery]
  ├── seedSuperAdmin() [Safe platform bootstrap]
  └── seedDefaultTenant()
        ├── Ensure Tenant 1 exists
        ├── Ensure default agent if 0 agents exist
        └── [IF AUTO_MIGRATE=true ONLY]
              ├── Normalize legacy agent tenant_id
              └── Normalize legacy chat_histories agent_id
              (Knowledge mutation is PERMANENTLY REMOVED)
```

---

## 5. Read-Only Schema Preflight
When `AUTO_MIGRATE=false`, `preflightCurrentSchema()` executes against `information_schema`:
1. Queries `information_schema.TABLES` where `TABLE_SCHEMA = DATABASE()`.
2. Validates that all 45 required tables for the current application exist (`RequiredCurrentTables`).
3. Validates `chat_histories.wa_msg_id`: must exist, type `varchar`, length 64, collation `ascii_bin`.
4. Validates `chat_histories.wa_msg_key`: must exist, type `varchar`, length 64, collation `ascii_bin`, extra `STORED GENERATED`.
5. Validates `chat_histories.uidx_chat_agent_wa_key`: must exist, unique, composite on `(agent_id, wa_msg_key)`.
6. **No writes, no DDL:** Any failure halts startup cleanly with an explanatory log message:
   `skema database belum siap: N tabel wajib belum ditemukan (...); jalankan migrasi dengan AUTO_MIGRATE=true atau gunakan skrip migrasi eksplisit`
7. **Exclusion of Future Tables:** Future SaaS tables (`tenant_members`, `plans`, `plan_features`, `subscriptions`, `usage_counters`, `audit_logs`) are intentionally excluded until explicit migration checkpoints.

---

## 6. Raw DDL Governance
Every raw DDL statement in the codebase was audited and gated:
1. `CREATE DATABASE IF NOT EXISTS`: Gated behind `if autoMigrate`. In production (`AUTO_MIGRATE=false`), the application connects directly to the existing database without attempting root DDL.
2. `ALTER TABLE chat_histories MODIFY COLUMN wa_msg_id ...`: Gated inside `ensureCanonicalChatMessageIDs()`, which only executes when `autoMigrate == true`.
3. `ALTER TABLE chat_histories ADD/MODIFY COLUMN wa_msg_key ...`: Gated inside `ensureCanonicalChatMessageIDs()` when `autoMigrate == true`.
4. `CREATE UNIQUE INDEX uidx_chat_agent_wa_key ...`: Gated inside `ensureCanonicalChatMessageIDs()` when `autoMigrate == true`.

---

## 7. Startup Mutation Policy
The startup mutation policy is enforced as follows:
- **`AUTO_MIGRATE=false` (Production Safe):**
  - Zero GORM AutoMigrate.
  - Zero raw DDL (`CREATE DATABASE`, `ALTER TABLE`, `CREATE UNIQUE INDEX`).
  - Zero automatic schema repairs.
  - Read-only schema preflight.
  - Zero destructive orphan reassignments.
  - Safe operational routines (`recoverStuckCrawlJobs`, `seedSuperAdmin`) execute idempotently.
- **`AUTO_MIGRATE=true` (Developer Migration Mode):**
  - Controlled migration path for local development and initial schema generation.
  - Only existing model migrations execute; no premature SaaS schema is added here.

---

## 8. Knowledge Mutation Removal
The unconditional orphan reassignment:
```sql
UPDATE knowledges SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL;
```
has been **PERMANENTLY REMOVED** from both the `if agentCount == 0` block and the general seeder block in `seedDefaultTenant()`.

**Rationale:**
Under Phase 2B multi-tenant architecture, `knowledges.agent_id = NULL` represents tenant-wide knowledge shared across all agents in the workspace. Automatically forcing `agent_id = 1` destroys this ownership model. Tenant-wide knowledge retrieval will be handled natively in the upcoming Knowledge/RAG checkpoint.

---

## 9. Chat History Startup Mutation Decision
- **Audit Findings:** Production chat history contains 29,941 messages, all correctly associated with Agent 3 (`agent_id = 3`). In the GORM model and MySQL schema, `chat_histories.agent_id` is defined as `not null`. Zero orphan messages (`agent_id = 0` or NULL) exist in production.
- **Decision:** The mutation `UPDATE chat_histories SET agent_id = 1 WHERE agent_id = 0 OR agent_id IS NULL` is **isolated from normal startup**. It never executes when `AUTO_MIGRATE=false`.
- In migration mode (`AUTO_MIGRATE=true`), it is retained strictly as a development bootstrap fallback.

---

## 10. Tests Executed
Dedicated unit tests were implemented and verified locally:
1. **`TestAutoMigrateSwitchGovernance` (`backend/database`):**
   - TEST-A: `AUTO_MIGRATE=false` or unset disabled migration mode.
   - TEST-B: `AUTO_MIGRATE=true` enables migration mode.
2. **`TestPreflightPassesWhenRequiredSchemaExists` (`backend/database`):**
   - TEST-C: Preflight passes when all 45 required tables and canonical columns/indexes exist.
3. **`TestPreflightFailsClearlyWhenRequiredSchemaMissing` (`backend/database`):**
   - TEST-D: Preflight fails with clear actionable error when tables or columns/indexes are missing.
4. **`TestNoUnsafeKnowledgeMutationInDatabaseSource` (`backend/database`):**
   - TEST-E & TEST-F: Source code verification confirming zero occurrences of `UPDATE knowledges ... agent_id = 1`.
5. **`TestCurrentRequiredTablesDoNotContainPrematureSaaSTables` (`backend/database`):**
   - TEST-G: Verifies `RequiredCurrentTables` does not contain future SaaS tables.
6. **`TestEnvBool` and `TestAutoMigrateEnabledDefaultFalse` (`backend/config`):**
   - Verifies case-insensitive parsing, trimming, truthy/falsy representations, and default `false`.

### Test Results
- `go test ./backend/config` -> **PASS**
- `go test ./backend/database` -> **PASS**
- `go test ./backend/...` -> **ALL PASS** (config, database, handlers, services)
- `go vet ./backend/...` -> **PASS** (zero warnings)
- `go build ./backend/...` -> **PASS** (clean compilation)
- `git diff --check` -> **PASS** (zero trailing whitespace)

---

## 11. Static Verification
Static audit of codebase search patterns:
- `AutoMigrate(`: 1 occurrence in `database.go`, strictly gated under `if autoMigrate`.
- `ALTER TABLE`: 3 occurrences in `database.go`, strictly gated inside `ensureCanonicalChatMessageIDs()` under `if autoMigrate`.
- `CREATE UNIQUE INDEX`: 1 occurrence in `database.go`, strictly gated inside `ensureCanonicalChatMessageIDs()` under `if autoMigrate`.
- `CREATE INDEX`: 0 occurrences.
- `DROP TABLE`: 0 occurrences.
- `DROP COLUMN`: 0 occurrences.
- `UPDATE knowledges`: 0 occurrences in application runtime.
- `agent_id = 1`: 0 occurrences in application runtime (only in tests and seeder documentation comments).
- Duplicate `AUTO_MIGRATE` configs: 0 duplicates. Single source of truth in `backend/config/config.go`.

---

## 12. Files Changed
1. `backend/config/config.go`: Enhanced `EnvBool` with whitespace trimming and case-insensitivity; added `AutoMigrateEnabled()`.
2. `backend/config/config_test.go`: Unit tests for `EnvBool` and `AutoMigrateEnabled`.
3. `backend/database/database.go`: AutoMigrate governance, read-only preflight, raw DDL gating, removed unsafe knowledge mutation.
4. `backend/database/automigrate_governance_test.go`: Governance and preflight validation unit tests.
5. `docs/PHASE-2B.3.1-AUTOMIGRATE-GOVERNANCE.md`: Checkpoint documentation.

---

## 13. Deferred Items for Later Checkpoints
The following items remain strictly deferred to subsequent Phase 2B.3 checkpoints:
- **Phase 2B.3.2:** Explicit SaaS database migration (`001_phase2b_saas_schema.sql` adding `tenant_members`, `plans`, `plan_features`, `subscriptions`, `usage_counters`, `audit_logs`).
- **Phase 2B.3.3:** SaaS domain models and GORM struct definitions.
- **Phase 2B.3.4:** EntitlementService and quota gating.
- **Phase 2B.3.5:** Multi-tenant auth middleware and tenant switching.
- **Phase 2B.3.6:** Tenant-wide knowledge resolution (`agent_id = NULL`).

---

## 14. Production Safety Statement
All implementation, testing, and validation for Phase 2B.3.1 were conducted **strictly locally**.
- **Production Database:** Untouched. Zero SQL commands or schema alterations executed.
- **Staging Database:** Untouched. Zero SQL commands or schema alterations executed.
- **Production Services:** Untouched. No restarts or systemd modifications.
- **WhatsApp Sessions:** Untouched. Active session (`wa-session-agent-3.db`) remains intact and undisturbed.
- **ChatLoop:** Untouched.
- **Canonical Phase 2B Design:** Fully preserved without contradiction.
