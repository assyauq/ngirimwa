# RUANGKIRIM â€” FINAL SAAS DATABASE SPECIFICATION
**Project:** Ruangkirim
**Repository:** `mrifatsyauqi/ruangkirim`
**Document:** Final SaaS Database & Migration Specification
**Phase:** 2B.1.1 Refinement
**Status:** APPROVED DESIGN SPECIFICATION (ZERO IMPLEMENTATION / READ-ONLY)
**Date:** September 11, 2026

---

## 1. Current Database Baseline

The live verified database baseline established during Phase 2B.0 across Production (`ruangkirim`) and Staging (`ruangkirim_staging`):

- **Total Tables:** Exactly **45 tables**.
- **Tenant Table (`tenants`):** 1 record (`id=1, name='Default'`). Columns: `id`, `name`, `created_at`, `updated_at`.
- **Users Table (`users`):** 1 production record (`id=1, username='superadmin', role='admin', is_super_admin=1, tenant_id=NULL`).
- **Agents Table (`agents`):** 1 production record (`id=3, tenant_id=1, name='Admin J&T Express Batang'`).
- **Large Business Tables:**
  - `chat_histories`: 29,941 rows (scoped indirectly via `agent_id = 3`).
  - `inbox_read_states`: 5,163 rows (scoped indirectly via `agent_id = 3`).
  - `contacts`: 532 rows (scoped indirectly via `agent_id = 3`).
- **Existing Foreign Keys:** Only 2 database-level foreign keys exist in the entire database:
  - `fk_ai_form_submissions_form`: `ai_form_submissions.form_id -> ai_forms.id`
  - `fk_product_orders_product`: `product_orders.product_id -> products.id`
- **SaaS Subscription Tables:** Exactly **0** tables exist currently.

---

## 2. New SaaS Tables

Phase 2B introduces 6 new core SaaS tables to manage multi-tenancy, memberships, subscriptions, and quotas:

### 1. `tenant_members`
**Purpose:** Links human users to customer tenants with specific workspace roles.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Membership ID |
| `tenant_id` | bigint unsigned | NO | None | FK -> `tenants(id)` ON DELETE CASCADE | Target workspace |
| `user_id` | bigint unsigned | NO | None | FK -> `users(id)` ON DELETE CASCADE | Associated user |
| `role` | varchar(24) | NO | 'cs' | None | `owner`, `admin`, `cs` |
| `status` | varchar(24) | NO | 'active' | None | `active`, `invited`, `disabled` |
| `created_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Creation timestamp |
| `updated_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Last update timestamp |

**Indexes & Constraints:**
- `UNIQUE KEY uk_tenant_members_tenant_user (tenant_id, user_id)`
- `KEY idx_tenant_members_user (user_id)`
- `KEY idx_tenant_members_role (tenant_id, role)`

---

### 2. `plans`
**Purpose:** Defines sellable commercial subscription packages and tiers.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Plan ID |
| `code` | varchar(32) | NO | None | UNIQUE | Identifier (`trial`, `starter`, `pro`, `business`) |
| `name` | varchar(64) | NO | None | None | Display name (e.g. "Free Trial", "Paket Pro") |
| `description` | text | YES | NULL | None | Plan marketing description |
| `price` | int unsigned | NO | 0 | None | Price in IDR (0 for trial) |
| `billing_interval` | varchar(16) | NO | 'monthly' | None | `monthly`, `yearly` |
| `is_active` | tinyint(1) | NO | 1 | None | Plan availability flag |
| `sort_order` | int | NO | 0 | None | UI display sequence |
| `created_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |
| `updated_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |

**Indexes & Constraints:**
- `UNIQUE KEY uk_plans_code (code)`
- `KEY idx_plans_active_sort (is_active, sort_order)`

---

### 3. `plan_features`
**Purpose:** Configures granular feature flags and numeric resource limits per plan.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Feature rule ID |
| `plan_id` | bigint unsigned | NO | None | FK -> `plans(id)` ON DELETE CASCADE | Associated plan |
| `feature_key` | varchar(64) | NO | None | None | Feature identifier |
| `limit_type` | varchar(16) | NO | 'boolean' | None | `boolean` or `numeric` |
| `limit_value` | int | YES | NULL | None | Numeric quota (-1 = unlimited, NULL for boolean) |
| `enabled` | tinyint(1) | NO | 1 | None | Feature toggle |
| `created_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |
| `updated_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |

**Indexes & Constraints:**
- `UNIQUE KEY uk_plan_features_plan_key (plan_id, feature_key)`
- `KEY idx_plan_features_key (feature_key)`

---

### 4. `subscriptions`
**Purpose:** Authoritative commercial ledger recording subscription contracts, current state, and billing periods.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Subscription ID |
| `tenant_id` | bigint unsigned | NO | None | FK -> `tenants(id)` ON DELETE RESTRICT | Subscribed workspace |
| `plan_id` | bigint unsigned | NO | None | FK -> `plans(id)` ON DELETE RESTRICT | Active plan tier |
| `status` | varchar(24) | NO | 'trialing' | None | `trialing`, `active`, `past_due`, `cancelled`, `expired` |
| `is_current` | tinyint(1) | YES | NULL | None | Flag for current subscription (`1` or `NULL`) |
| `current_period_start`| datetime(3) | NO | None | None | Billing cycle start |
| `current_period_end` | datetime(3) | NO | None | None | Billing cycle expiration |
| `trial_ends_at` | datetime(3) | YES | NULL | None | Specific trial end timestamp |
| `cancel_at_period_end`| tinyint(1) | NO | 0 | None | Non-renewal flag |
| `payment_provider` | varchar(32) | YES | 'manual' | None | Gateway name |
| `external_reference` | varchar(128) | YES | NULL | None | External invoice/subscription ID |
| `created_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |
| `updated_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Timestamp |

**Indexes & Constraints:**
- `UNIQUE KEY uk_subscriptions_tenant_current (tenant_id, is_current)`: Guarantees at most ONE current subscription per tenant at the MySQL engine level (since MySQL allows multiple `NULL` values in unique indexes).
- `KEY idx_subscriptions_tenant_status (tenant_id, status)`
- `KEY idx_subscriptions_period_end (current_period_end)`

---

### 5. `usage_counters`
**Purpose:** Tracks resource consumption per tenant across billing periods.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Counter ID |
| `tenant_id` | bigint unsigned | NO | None | FK -> `tenants(id)` ON DELETE CASCADE | Associated workspace |
| `metric_key` | varchar(64) | NO | None | None | Metric name (`monthly_messages`, etc.) |
| `period_start` | date | NO | None | None | Start date of measurement period |
| `period_end` | date | NO | None | None | End date of measurement period |
| `used_value` | int unsigned | NO | 0 | None | Accumulated consumption |
| `updated_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Last increment timestamp |

**Indexes & Constraints:**
- `UNIQUE KEY uk_usage_counters_tenant_metric_period (tenant_id, metric_key, period_start, period_end)`
- `KEY idx_usage_counters_lookup (tenant_id, metric_key)`

---

### 6. `audit_logs`
**Purpose:** Immutable platform and workspace audit trail for security and governance.

| Column | Type | Nullable | Default | Constraints | Description |
|---|---|---|---|---|---|
| `id` | bigint unsigned | NO | auto_increment | PRIMARY KEY | Audit ID |
| `tenant_id` | bigint unsigned | YES | NULL | FK -> `tenants(id)` ON DELETE SET NULL | Workspace context (NULL for platform events) |
| `actor_user_id` | bigint unsigned | YES | NULL | FK -> `users(id)` ON DELETE SET NULL | User who performed action |
| `action` | varchar(64) | NO | None | None | Action identifier (e.g. `agent.created`) |
| `resource_type` | varchar(64) | NO | None | None | Entity type (`tenant`, `subscription`, `agent`) |
| `resource_id` | varchar(64) | NO | None | None | Target entity ID |
| `metadata_json` | text | YES | NULL | None | Serialized JSON event details |
| `ip_address` | varchar(45) | YES | NULL | None | Client IP |
| `created_at` | datetime(3) | YES | CURRENT_TIMESTAMP(3) | None | Event timestamp |

**Indexes & Constraints:**
- `KEY idx_audit_logs_tenant_created (tenant_id, created_at)`
- `KEY idx_audit_logs_actor (actor_user_id)`
- `KEY idx_audit_logs_resource (resource_type, resource_id)`

---

## 3. Existing Table Changes

All changes to existing tables are strictly **additive**:

### 1. Table `tenants`
- Add Column: `slug` (`varchar(64) NULL`) -> Backfilled to `'default'` for `id=1` -> Set to `NOT NULL` with `UNIQUE KEY uk_tenants_slug (slug)`.
- Add Column: `status` (`varchar(24) NOT NULL DEFAULT 'active'`) -> Operational state (`trialing`, `active`, `past_due`, `suspended`, `cancelled`).
- Add Column: `trial_ends_at` (`datetime(3) NULL`) -> Read-cache field reflecting `subscriptions.trial_ends_at`. Initialized to `NULL` for existing `id=1`.
- Add Index: `KEY idx_tenants_status (status)`.

### 2. Table `users` (Staged Email Strategy)
- `users.username` remains `VARCHAR(64) UNIQUE NOT NULL` for backward-compatible authentication.
- `users.email` remains `VARCHAR(255) NULL` during Phase 2B.
- Staged execution:
  - **Phase A (Current):** Preserve existing users and authentication compatibility.
  - **Phase B:** Backfill and normalize missing/placeholder emails where required.
  - **Phase C (Future):** Apply `NOT NULL` and `UNIQUE` constraints only after Phase B validation confirms zero duplicates and zero nulls.

### 3. Table `agents`
- Maintain existing `tenant_id` (`bigint unsigned NOT NULL`).
- Add FK Constraint after backfill: `fk_agents_tenant`: `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT`.

### 4. Table `knowledges`
- Add Column: `tenant_id` (`bigint unsigned NULL`) -> Backfilled via `agents.tenant_id` -> Set to `NOT NULL`.
- Make `agent_id` nullable (`bigint unsigned NULL DEFAULT NULL`) to support tenant-wide knowledge bases.
- Add Index: `KEY idx_knowledges_tenant (tenant_id)`.

### 5. Table `contacts`
- Add Column: `tenant_id` (`bigint unsigned NULL`) -> Backfilled via `agents.tenant_id`.
- Add Index: `KEY idx_contacts_tenant_number (tenant_id, number)`.

---

## 4. Complete 45-Table Final Ownership Matrix

| # | Table Name | Live Rows | Final Classification | Final Ownership Path | Direct tenant_id? | Phase 2B Schema Action |
|---|---|---|---|---|---|---|
| 1 | `agents` | 1 | DIRECT TENANT OWNED | `tenant_id -> tenants.id` | YES | Retain. Add foreign key to `tenants(id)`. |
| 2 | `ai_form_sessions` | 1 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Direct tenant scoping already present. |
| 3 | `ai_form_submissions` | 1 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Direct tenant scoping already present. |
| 4 | `ai_forms` | 2 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Direct tenant scoping already present. |
| 5 | `ai_turns` | 19 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 6 | `app_settings` | 4 | PLATFORM GLOBAL | Platform key-value store | NO | Retain as platform-wide configuration. |
| 7 | `auto_replies` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 8 | `broadcast_recipients`| 0 | INDIRECT TENANT OWNED | `broadcast_id -> broadcasts.id`| NO | Scoped via `broadcast_id`. Retain. |
| 9 | `broadcasts` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Already tenant-aware. |
| 10 | `chat_histories` | 29,941 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | **CRITICAL: DO NOT alter.** Retain indirect scoping via `agent_id`. |
| 11 | `chat_labels` | 1 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 12 | `closing_forms` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 13 | `closing_records` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 14 | `contact_consents` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 15 | `contacts` | 532 | HYBRID SCOPED | `agent_id` + `tenant_id` | **ADD (NULL)** | Add `tenant_id` column; backfill from `agents`. |
| 16 | `conversation_memories`| 2 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 17 | `crawl_jobs` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 18 | `crawl_pages` | 0 | INDIRECT TENANT OWNED | `job_id -> crawl_jobs.id` | NO | Scoped via `job_id`. Retain. |
| 19 | `cs_activity_logs` | 44 | DIRECT TENANT OWNED | `tenant_id`, `user_id` | YES | Retain. Direct tenant scoping already present. |
| 20 | `flow_sessions` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 21 | `flows` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 22 | `follow_up_enrollments`| 0 | DIRECT TENANT OWNED | `tenant_id`, `follow_up_id` | YES | Retain. Already tenant-aware. |
| 23 | `follow_up_steps` | 0 | INDIRECT TENANT OWNED | `follow_up_id -> follow_ups.id`| NO | Scoped via `follow_up_id`. Retain. |
| 24 | `follow_ups` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Already tenant-aware. |
| 25 | `group_guard_configs` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES (NULL) | Enforce `tenant_id NOT NULL`. |
| 26 | `group_moderation_logs`| 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 27 | `handoffs` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 28 | `inbox_read_states` | 5,163 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | **Retain indirect scoping.** Avoid table locks. |
| 29 | `knowledges` | 13 | DIRECT TENANT OWNED | `tenant_id` (optional `agent_id`)| **ADD** | Add `tenant_id`; make `agent_id` nullable. |
| 30 | `labels` | 11 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 31 | `login_throttles` | 2 | DERIVED / SYSTEM | System rate limiting | NO | Retain as platform-global table. |
| 32 | `meta_conversion_events`| 0 | DIRECT TENANT OWNED | `tenant_id` | YES (NULL) | Enforce `tenant_id NOT NULL`. |
| 33 | `opt_outs` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 34 | `otp_codes` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 35 | `product_checkout_sessions`| 0 | DIRECT TENANT OWNED | `tenant_id`, `product_id` | YES | Retain. Already tenant-aware. |
| 36 | `product_orders` | 0 | DIRECT TENANT OWNED | `tenant_id`, `product_id` | YES | Retain. Already tenant-aware. |
| 37 | `products` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Already tenant-aware. |
| 38 | `scheduled_messages` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Already tenant-aware. |
| 39 | `scheduled_statuses` | 0 | DIRECT TENANT OWNED | `tenant_id`, `agent_id` | YES | Retain. Already tenant-aware. |
| 40 | `settings` | 0 | DEPRECATED | Obsolete single-agent table | NO | Mark for future DROP; do not query. |
| 41 | `shipping_cities` | 0 | PLATFORM GLOBAL | RajaOngkir master data | NO | Retain as platform-global cache. |
| 42 | `templates` | 0 | INDIRECT TENANT OWNED | `agent_id -> agents.id` | NO | Retain indirect scoping via `agent_id`. |
| 43 | `tenants` | 1 | PLATFORM ROOT | Workspace root | N/A | Add `slug`, `status`, `trial_ends_at`. |
| 44 | `user_agent_assignments`| 0 | DIRECT TENANT OWNED | `tenant_id`, `user_id` | YES | Retain. Already tenant-aware. |
| 45 | `users` | 1 | USER GLOBAL / TENANT ASSOC | Multi-tenant via `tenant_members`| YES (NULL) | Deprecate direct scalar `tenant_id`. |

---

## 5. Foreign Key Strategy

To maintain high throughput on WhatsApp message arrival and avoid cascading row locks:

1. **New SaaS Tables:** Full foreign key constraints are applied (`ON DELETE RESTRICT` for subscriptions, `ON DELETE CASCADE` for plan features and members).
2. **Existing Tables:** Foreign keys are NOT added retroactively to legacy high-volume tables (`chat_histories`, `inbox_read_states`) to prevent replication lag and write lock contention.
3. **Agent Constraint:** `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT` will be added to `agents` after backfill verification.

---

## 6. Unique Constraint Strategy

To guarantee multi-tenant safety and commercial integrity:
- `tenants.slug`: `UNIQUE KEY uk_tenants_slug (slug)`
- `tenant_members`: `UNIQUE KEY uk_tenant_members_tenant_user (tenant_id, user_id)`
- `plans.code`: `UNIQUE KEY uk_plans_code (code)`
- `plan_features`: `UNIQUE KEY uk_plan_features_plan_key (plan_id, feature_key)`
- `subscriptions`: `UNIQUE KEY uk_subscriptions_tenant_current (tenant_id, is_current)`
- `usage_counters`: `UNIQUE KEY uk_usage_counters (tenant_id, metric_key, period_start, period_end)`
- `users.username`: `UNIQUE KEY uk_users_username (username)` (retained for backward compatibility)
- `users.email`: Constraint deferred to Phase C.

---

## 7. Index Strategy

Indexes designed for multi-tenant query acceleration:
- `agents(tenant_id, id)`
- `knowledges(tenant_id, agent_id)`
- `contacts(tenant_id, number)`
- `subscriptions(tenant_id, is_current)`
- `subscriptions(tenant_id, status)`
- `audit_logs(tenant_id, created_at)`
- `chat_histories(agent_id, created_at)` (Preserves high-speed cursor pagination without requiring `tenant_id`).

---

## 8. Trial Data Model

When a new tenant signs up:
1. `INSERT INTO tenants (name, slug, status, trial_ends_at) VALUES (?, ?, 'trialing', NOW() + INTERVAL 30 DAY)`
2. `INSERT INTO subscriptions (tenant_id, plan_id, status, is_current, current_period_start, current_period_end, trial_ends_at) VALUES (tenant_id, trial_plan_id, 'trialing', 1, NOW(), NOW() + INTERVAL 30 DAY, NOW() + INTERVAL 30 DAY)`
3. Plan feature `max_active_agents = 1` is linked via the `trial` plan.

---

## 9. Subscription Data Model

Authoritative query for active subscription:
```sql
SELECT s.*, p.code as plan_code, p.name as plan_name
FROM subscriptions s
JOIN plans p ON s.plan_id = p.id
WHERE s.tenant_id = ?
  AND s.is_current = 1
LIMIT 1;
```

Transactional transition to new plan:
```sql
START TRANSACTION;
-- Demote previous subscription
UPDATE subscriptions
SET is_current = NULL, status = 'expired', updated_at = NOW()
WHERE tenant_id = ? AND is_current = 1;

-- Insert new subscription with is_current = 1
INSERT INTO subscriptions (
    tenant_id, plan_id, status, is_current,
    current_period_start, current_period_end, trial_ends_at,
    created_at, updated_at
) VALUES (?, ?, 'active', 1, NOW(), NOW() + INTERVAL 30 DAY, NULL, NOW(), NOW());
COMMIT;
```

---

## 10. Usage Data Model

Atomic quota consumption query:
```sql
INSERT INTO usage_counters (tenant_id, metric_key, period_start, period_end, used_value, updated_at)
VALUES (?, ?, ?, ?, 1, NOW())
ON DUPLICATE KEY UPDATE
    used_value = used_value + 1,
    updated_at = NOW();
```

---

## 11. Backfill Strategy

For existing production data:

1. **Step 1: Tenant 1 Update**
   ```sql
   UPDATE tenants
   SET slug = 'default', status = 'active', trial_ends_at = NULL
   WHERE id = 1;
   ```
2. **Step 2: Seed Plans**
   Insert standard plans (`trial`, `starter`, `pro`, `business`) and assign feature records.
3. **Step 3: Tenant 1 Subscription**
   ```sql
   INSERT INTO subscriptions (tenant_id, plan_id, status, is_current, current_period_start, current_period_end, trial_ends_at)
   SELECT 1, id, 'active', 1, NOW(), NOW() + INTERVAL 10 YEAR, NULL
   FROM plans WHERE code = 'business' LIMIT 1;
   ```
4. **Step 4: User 1 Membership**
   ```sql
   INSERT INTO tenant_members (tenant_id, user_id, role, status, created_at, updated_at)
   VALUES (1, 1, 'owner', 'active', NOW(), NOW());
   ```
5. **Step 5: Knowledge Backfill**
   ```sql
   UPDATE knowledges k
   JOIN agents a ON k.agent_id = a.id
   SET k.tenant_id = a.tenant_id
   WHERE k.tenant_id IS NULL OR k.tenant_id = 0;
   ```
6. **Step 6: Contacts Backfill**
   ```sql
   UPDATE contacts c
   JOIN agents a ON c.agent_id = a.id
   SET c.tenant_id = a.tenant_id
   WHERE c.tenant_id IS NULL OR c.tenant_id = 0;
   ```

---

## 12. Tenant 1 Migration

Tenant 1 (`Default`) is the live company workspace operating WhatsApp Agent 3 (`Admin J&T Express Batang`). It is grandfathered as an active business-tier tenant with permanent validity, ensuring Agent 3 encounters zero operational interruption.

---

## 13. Large Table Strategy (`chat_histories`)

Table `chat_histories` contains **29,941 records**.
- **Constraint:** Direct `ALTER TABLE chat_histories ADD COLUMN tenant_id` in production carries lock-contention risks.
- **Decision:** Do NOT add `tenant_id` to `chat_histories` in Phase 2B.
- **Security Assessment:** Handlers always access chat histories via `resolveAgent(c)`:
  1. `agent_id` is validated to belong to caller's `tenant_id`.
  2. Query is executed: `SELECT * FROM chat_histories WHERE agent_id = ? AND sender = ?`.
  3. No route permits querying `chat_histories` across arbitrary IDs without agent validation.

---

## 14. AutoMigrate Governance

### Final Policy:
1. **Production Boot Safety:** GORM `AutoMigrate` must not execute unmanaged DDL on production startup.
2. **Implementation:** Introduce environment flag `AUTO_MIGRATE=false` in production.
3. **Execution Model:** Schema changes in production must be applied via explicit, idempotent SQL migration scripts executed before binary deployment.
4. **Staging Verification:** All SQL scripts must be executed and verified on Staging (`ruangkirim_staging`) prior to production execution.

---

## 15. Migration Dependency Order

The migration sequence follows an exact 17-step operational process designed to minimize operational risk:

```text
01. AutoMigrate governance/preflight
    Rationale: Ensures application cannot attempt conflicting DDL during deployment.
02. Schema preflight
    Rationale: Verifies table counts, active connections, and engine status before touching schema.
03. Backup + backup verification
    Rationale: Creates a cold mysqldump snapshot and verifies file integrity and size.
04. Create new SaaS tables
    Rationale: Creates non-blocking empty tables (plans, plan_features, tenant_members, subscriptions, usage_counters, audit_logs).
05. Add additive columns
    Rationale: Adds nullable columns (slug, status, trial_ends_at to tenants; tenant_id to knowledges and contacts).
06. Backfill data
    Rationale: Populates tenant_id on knowledges/contacts and initializes Tenant 1 slug/status.
07. Add constraints/indexes after validation
    Rationale: Applies UNIQUE and NOT NULL constraints only after data backfill is verified clean.
08. Seed plans/features
    Rationale: Populates commercial plan catalog and feature limits.
09. Create Tenant 1 grandfathered subscription
    Rationale: Attaches active permanent subscription to Tenant 1 with is_current = 1.
10. Create Tenant 1 membership
    Rationale: Links User 1 (superadmin) as owner in tenant_members.
11. Validate data integrity
    Rationale: Executes verification queries ensuring zero orphan records and correct table counts.
12. Application compatibility testing
    Rationale: Runs local backend test suite against migrated database schema.
13. Staging migration
    Rationale: Executes steps 01-11 on dev.ruangkirim.web.id staging environment.
14. Staging SaaS isolation tests
    Rationale: Validates multi-tenant isolation, 1-sender limit, and trial expiry on staging.
15. Production approval
    Rationale: Requires explicit formal user authorization before live deployment.
16. Production migration
    Rationale: Executes steps 01-11 on production database during off-peak window.
17. Production verification
    Rationale: Confirms production health, table count (51), and active Agent 3 WhatsApp connectivity.
```

---

## 16. Rollback Strategy

1. **Database Rollback:** Because all changes are additive (new tables, new nullable columns), rolling back application code does not require rolling back the database.
2. **Cold Snapshot:** A pre-migration dump `backup_pre_phase2b.sql` is retained on VPS.
3. **Application Downgrade:** If issues arise, swapping back the Phase 2A binary (`ed2a0fb`) restores previous behavior, ignoring the new SaaS tables.
4. **WhatsApp Safety:** WhatsApp session files must not be relocated or modified.

---

## 17. Staging Migration Requirements

Before touching production:
- Complete migration run on `ruangkirim_staging`.
- Verify table count increases from 45 to 51.
- Run automated API tests verifying multi-tenant isolation, 1-sender limit enforcement, and trial expiration logic.

---

## 18. Production Migration Requirements

- Maintenance window: Off-peak (22:00â€“00:00 WIB).
- Verified pre-migration mysqldump.
- Service restarted using systemd binary replacement designed to avoid intentional downtime.
- WhatsApp Agent 3 session (`/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`) verified active and connected.

---

## 19. Data Integrity Validation Queries

Post-migration verification queries:

```sql
-- 1. Verify Tenant 1 exists and is active
SELECT count(*) FROM tenants WHERE id = 1 AND slug = 'default' AND status = 'active'; -- Expected: 1

-- 2. Verify User 1 is linked to Tenant 1
SELECT count(*) FROM tenant_members WHERE tenant_id = 1 AND user_id = 1 AND role = 'owner'; -- Expected: 1

-- 3. Verify Agent 3 remains intact
SELECT count(*) FROM agents WHERE id = 3 AND tenant_id = 1; -- Expected: 1

-- 4. Verify no orphaned agents exist
SELECT count(*) FROM agents WHERE tenant_id IS NULL OR tenant_id = 0; -- Expected: 0

-- 5. Verify no orphaned knowledges exist
SELECT count(*) FROM knowledges WHERE tenant_id IS NULL OR tenant_id = 0; -- Expected: 0

-- 6. Verify table count
SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE(); -- Expected: 51
```

---

## 20. Security Validation

- Attempting `GET /api/agents` with a token for Tenant B returns ONLY Tenant B agents.
- Creating a second agent under a `trial` subscription returns HTTP 403.
- Modifying a knowledge item belonging to Tenant A using a Tenant B session returns HTTP 404.

---

## 21. Final Schema Verdict

**FINAL DATABASE SPECIFICATION: APPROVED FOR IMPLEMENTATION PLANNING**
The schema specification provides an empirically verified, production-safe database blueprint that transforms Ruangkirim into a multi-tenant SaaS platform while preserving live WhatsApp operations.
