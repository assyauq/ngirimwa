# RUANGKIRIM â€” CANONICAL SAAS DOMAIN MODEL SPECIFICATION
**Project:** Ruangkirim
**Repository:** `mrifatsyauqi/ruangkirim`
**Document:** Final SaaS Domain Model
**Phase:** 2B.1.2 Final Micro-Refinement
**Status:** APPROVED DESIGN SPECIFICATION (ZERO IMPLEMENTATION / READ-ONLY)
**Date:** September 11, 2026

---

## 1. Executive Summary

This document establishes the canonical SaaS domain model for Ruangkirim, evolving the platform from an internal single-tenant company deployment into a structured multi-tenant subscription SaaS.

The architecture is designed to fulfill strict commercial and operational requirements:
1. **Multi-Tenancy:** Workspace isolation where each customer company operates within an independent Tenant boundary.
2. **Data & Session Integrity:** Designed to preserve existing production data (Tenant 1, User 1, Agent 3, 29,941 chat history records) without intentional downtime. WhatsApp session files must not be intentionally relocated or modified during Phase 2B migration (`/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`).
3. **SaaS Commercial Lifecycle:** Database-driven plans, entitlements, 30-day self-service trials, strict 1-sender trial caps, and usage tracking.
4. **Security Boundaries:** Cryptographic and database separation between platform administration (Super Admin) and tenant administration (Tenant Owner/Admin/CS).
5. **Operational Safety:** Staging-first migration, backup verification, and rollback procedures minimize operational risk.

---

## 2. Product SaaS Model

Ruangkirim is a multi-tenant WhatsApp AI Assistant and CRM platform for businesses in Indonesia.

### Commercial Tiering:
- **Free Trial:** 30-day duration, strictly limited to 1 active WhatsApp sender, core Inbox, and baseline AI capabilities.
- **Starter Plan:** Small teams requiring 1â€“2 WhatsApp senders, automated greetings, and basic CRM.
- **Pro Plan:** Growing e-commerce sellers requiring up to 5 WhatsApp senders, full AI auto-reply, vision checkout, and broadcasts.
- **Business / Enterprise:** Multi-agent rotation, custom AI knowledge bases, high-throughput broadcasts, and REST API access.

---

## 3. Tenant

The `Tenant` entity represents a customer workspace, business account, and commercial boundary. All operational agents, subscriptions, and customer data belong to a Tenant.

### Entity Attributes:
- `id` (bigint, unsigned, PK): Unique system identifier.
- `name` (varchar(128), NOT NULL): Business / company name.
- `slug` (varchar(64), NOT NULL, UNIQUE): URL-safe tenant identifier (e.g. `jt-batang`, `toko-berkah`).
- `status` (varchar(24), NOT NULL, DEFAULT 'active'): Workspace operational state (`trialing`, `active`, `past_due`, `suspended`, `cancelled`).
- `trial_ends_at` (datetime, NULL): Cached convenience timestamp reflecting trial expiration from the active subscription.
- `created_at` (datetime, NOT NULL)
- `updated_at` (datetime, NOT NULL)

### Tenant Operational State Machine:
```text
[ Registration ]
       â”‚
       â–¼
   trialing â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–º active â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–º past_due
       â”‚                   â–²                     â”‚
       â”‚ (Trial expires    â”‚ (Payment/Upgrade)   â”‚ (Grace period ends)
       â”‚  without upgrade) â”‚                     â–¼
       â–¼                   â”‚                 suspended
   suspended â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜                     â”‚
       â”‚                                         â–¼
       â–¼                                     cancelled
   cancelled
```

- `trialing`: Workspace operating under trial entitlements. Limited to 1 sender.
- `active`: Workspace operating with an active paid commercial subscription or grandfathered internal tier.
- `past_due`: Commercial subscription renewal is overdue; workspace operates under a temporary grace period.
- `suspended`: Workspace operational access is restricted (e.g., outbound message sending and AI replies are paused due to trial expiration or unpaid subscription; dashboard and historical data remain accessible in read-only mode).
- `cancelled`: Workspace explicitly terminated by customer request or platform administration.

*Note:* `"expired"` is strictly a **Subscription commercial state**, NOT a Tenant operational state. When a trial subscription expires without an upgrade, the Subscription transitions to `expired` while the Tenant transitions to `suspended`.

### Grandfathered Baseline (Tenant 1):
Existing production Tenant (`id = 1`) is a **grandfathered internal business tenant**. It is active, exempt from the 30-day trial, exempt from normal payment expiry, independent of external payment gateways, and retained for existing production operations. It remains active until an explicit future commercial or administrative policy alters its status.

---

## 4. User

The `User` entity represents human authentication identity across the platform.

### Staged Email Compatibility Strategy:
To avoid breaking existing production accounts, user identity migration follows a three-phase plan:

1. **Phase A (Preservation & Backward Compatibility â€” Current Phase 2B):**
   - Preserve existing `users` table schema and authentication behavior.
   - `users.username` remains the primary unique login identifier (`username VARCHAR(64) UNIQUE NOT NULL`).
   - `users.email` remains nullable and non-unique at the database level (`email VARCHAR(255) NULL`).
   - Multi-tenant workspace association is decoupled from `users.tenant_id` and moved to `tenant_members`.
   - Existing single-tenant accounts log in with existing usernames without breakage.
2. **Phase B (Data Normalization & Audit):**
   - Background audit of all user records to verify that every active user has an authentic, verified email address.
   - Resolve any empty, duplicate, or placeholder emails before attempting schema constraint alterations.
3. **Phase C (Constraint Enforcement â€” Future Release):**
   - Apply `NOT NULL` and `UNIQUE` constraints to `users.email` only after Phase B validation proves zero data conflicts.

### Entity Attributes:
- `id` (bigint, unsigned, PK): System user ID.
- `username` (varchar(64), NOT NULL, UNIQUE): Login identifier.
- `email` (varchar(255), NULL): User email address (staged compatibility).
- `password` (varchar(255), NOT NULL): Bcrypt password hash.
- `name` (varchar(128), NOT NULL): Display name.
- `phone` (varchar(32), NULL): Contact phone number.
- `is_super_admin` (boolean, NOT NULL, DEFAULT false): Platform-level operator flag.
- `active` (boolean, NOT NULL, DEFAULT true): Account active status.
- `email_verified` (boolean, NOT NULL, DEFAULT false)
- `created_at` / `updated_at` (datetime)

---

## 5. Tenant Membership

The `TenantMember` entity links a `User` to a `Tenant`, defining the user's role and operational scope within that specific workspace.

### Entity Attributes:
- `id` (bigint, unsigned, PK)
- `tenant_id` (bigint, unsigned, NOT NULL, FK -> `tenants.id`)
- `user_id` (bigint, unsigned, NOT NULL, FK -> `users.id`)
- `role` (varchar(24), NOT NULL): Workspace role (`owner`, `admin`, `cs`).
- `status` (varchar(24), NOT NULL, DEFAULT 'active'): `active`, `invited`, `disabled`.
- `created_at` / `updated_at` (datetime)

### Uniqueness:
- `UNIQUE KEY uk_tenant_members_tenant_user (tenant_id, user_id)`: A user can belong to a given tenant at most once.

### Role Hierarchy:
1. `owner`: Full control over workspace, billing, plan changes, team management, and agent deletion. Cannot be deleted without transferring ownership.
2. `admin`: Can create/manage agents, configure knowledge base, view analytics, and manage CS users. Cannot modify billing or delete workspace.
3. `cs` (Customer Service): Operational role restricted by `CSRouteGuard`. Can only interact with conversations, contacts, and send messages on assigned agents (`user_agent_assignments`).

---

## 6. Authentication & Tenant Context

To avoid multi-tenant data leakage while supporting users who belong to multiple workspaces, the system uses a **Cryptographically Bound Active Tenant Context**.

### Authentication Flow:
1. **User Login (`POST /api/login`):** User submits credentials (`username` + `password`).
2. **Tenant Membership Discovery:** Backend queries `tenant_members` for active memberships associated with `user.ID`.
3. **Token Minting:**
   - JWT claims include:
     ```json
     {
       "user_id": 1,
       "active_tenant_id": 1,
       "role": "owner",
       "is_super_admin": false,
       "exp": 1789012345
     }
     ```
4. **Context Enforcement Middleware (`AuthMiddleware`):**
   - Parses JWT and extracts `user_id` and `active_tenant_id`.
   - Validates live DB record in `tenant_members` WHERE `user_id = ? AND tenant_id = ? AND status = 'active'`.
   - Injects into Gin context:
     - `c.Set("user_id", user.ID)`
     - `c.Set("tenant_id", active_tenant_id)`
     - `c.Set("role", membership.Role)`
     - `c.Set("is_super_admin", user.IsSuperAdmin)`
5. **Tenant Switch Endpoint (`POST /api/auth/switch-tenant`):**
   - User requests switch to `target_tenant_id`.
   - Backend verifies membership in `target_tenant_id`.
   - Issues fresh JWT minted with the new `active_tenant_id`.

**Rule:** The client cannot manipulate `tenant_id` via HTTP headers or body; it is strictly derived from the verified JWT and live database membership check.

---

## 7. Super Admin

Platform Administration is strictly separated from Tenant Administration.

### Authorization Boundary:
- `is_super_admin = true` grants access ONLY to `/api/superadmin/*` routes.
- Super Admin does **NOT** automatically have membership in customer tenants.
- Super Admin does **NOT** appear in `tenant_members`.
- Super Admin actions (tenant suspension, plan changes, manual quota overrides) are logged to `audit_logs` with `actor_user_id` and platform metadata.

---

## 8. Agent / WhatsApp Sender

The `Agent` entity represents a virtual assistant persona bound to exactly one physical WhatsApp phone number.

### Invariants:
1. **Strict Ownership:** Every Agent belongs to exactly one Tenant (`agents.tenant_id`).
2. **Global Identity:** `agents.id` remains an auto-incrementing global integer.
3. **Session File Mapping:** WhatsApp session database is deterministically named `/var/lib/ruangkirim/whatsapp/wa-session-agent-{id}.db`.
4. **Active Sender Definition:** An agent is counted as an "active sender" against plan quotas if:
   - It is not marked as soft-deleted/archived.
   - It is configured with WhatsApp connectivity (`device_j_id != ''` OR status is `connected`/`connecting`/`paired`).

---

## 9. Plans

Plans define sellable subscription tiers and commercial capabilities.

### Entity Attributes:
- `id` (bigint, unsigned, PK)
- `code` (varchar(32), NOT NULL, UNIQUE): `trial`, `starter`, `pro`, `business`.
- `name` (varchar(64), NOT NULL): e.g. "Free Trial", "Starter", "Pro", "Business".
- `description` (text)
- `price` (int, NOT NULL): Price in IDR (e.g. 0, 149000, 299000, 599000).
- `billing_interval` (varchar(16), NOT NULL): `monthly`, `yearly`.
- `is_active` (boolean, NOT NULL, DEFAULT true): Plan availability flag.
- `sort_order` (int, NOT NULL, DEFAULT 0)
- `created_at` / `updated_at` (datetime)

---

## 10. Plan Features

Defines the explicit permissions and numeric quotas granted by each plan.

### Entity Attributes:
- `id` (bigint, unsigned, PK)
- `plan_id` (bigint, unsigned, NOT NULL, FK -> `plans.id`)
- `feature_key` (varchar(64), NOT NULL): Identifier of the feature or limit.
- `limit_type` (varchar(16), NOT NULL): `boolean` or `numeric`.
- `limit_value` (int, NULL): Value for numeric limits (-1 for unlimited, NULL for boolean).
- `enabled` (boolean, NOT NULL, DEFAULT true)
- `created_at` / `updated_at` (datetime)

### Standard Feature Keys:
| Feature Key | Type | Trial | Starter | Pro | Business |
|---|---|---|---|---|---|
| `max_active_agents` | Numeric | 1 | 2 | 5 | 15 |
| `max_team_members` | Numeric | 2 | 3 | 10 | 50 |
| `monthly_messages` | Numeric | 1,000 | 5,000 | 25,000 | 100,000 |
| `monthly_ai_turns` | Numeric | 500 | 2,500 | 10,000 | 50,000 |
| `ai_autoreply` | Boolean | true | true | true | true |
| `broadcast_enabled`| Boolean | false | true | true | true |
| `api_access` | Boolean | false | false | true | true |
| `webhooks` | Boolean | false | false | true | true |
| `crm_lead_stages` | Boolean | true | true | true | true |

---

## 11. Subscription

Represents a tenant's historical and currently active commercial entitlement period.

### Source of Truth Hierarchy:
1. **Commercial Source of Truth:** `subscriptions` is the authoritative commercial record containing plan IDs, commercial states, billing periods, and current status.
2. **Workspace Operational State:** `tenants.status` reflects the current operational access of the workspace (`trialing`, `active`, `past_due`, `suspended`, `cancelled`).
3. **Convenience Read Cache:** `tenants.trial_ends_at` is a cached convenience field reflecting `subscriptions.trial_ends_at` to allow fast indexing without joins.
4. **Authoritative Service Boundary:** `SubscriptionService` is the sole service authorized to mutate subscription state and synchronize `tenants.status` and `tenants.trial_ends_at` within a single atomic database transaction.

### Subscription Commercial States:
- `trialing`: Active trial period.
- `active`: Active paid commercial period.
- `past_due`: Renewal payment overdue; grace period active.
- `expired`: Trial or subscription period concluded without renewal.
- `cancelled`: Subscription terminated.

### Current Subscription Invariant:
- **Database Level:** The constraint `UNIQUE KEY uk_subscriptions_tenant_current (tenant_id, is_current)` guarantees that there is **AT MOST ONE** current subscription (`is_current = 1`) per tenant.
  - *MySQL Engine Behavior:* In MySQL (InnoDB), `NULL` values are treated as distinct in unique indexes. Multiple historical subscriptions can have `is_current = NULL`, but the engine rejects any second row with `is_current = 1` for the same `tenant_id`.
- **Application Level:** Every provisioned tenant is expected to have **EXACTLY ONE** current subscription during normal operation, except during a controlled transactional subscription transition.

### Transactional Transition Pattern:
```sql
START TRANSACTION;
-- 1. Demote previous active subscription to historical
UPDATE subscriptions
SET is_current = NULL, status = 'expired', updated_at = NOW()
WHERE tenant_id = ? AND is_current = 1;

-- 2. Insert new current subscription
INSERT INTO subscriptions (
    tenant_id, plan_id, status, is_current,
    current_period_start, current_period_end, trial_ends_at,
    created_at, updated_at
) VALUES (?, ?, 'active', 1, NOW(), NOW() + INTERVAL 30 DAY, NULL, NOW(), NOW());

-- 3. Synchronize operational tenant state
UPDATE tenants
SET status = 'active', trial_ends_at = NULL, updated_at = NOW()
WHERE id = ?;
COMMIT;
```

---

## 12. Trial

### Trial State Transitions:
1. **Provisioning:** On registration, Tenant is created with `status = 'trialing'`, `trial_ends_at = NOW() + 30 days`.
2. **Initial Subscription:** A subscription row is inserted with `plan_id = trial_plan_id`, `status = 'trialing'`, `is_current = 1`.
3. **Expiration Handling (Phase 2B Pragmatic Model):**
   - **Request-Time Entitlement Evaluation:** When an authenticated request arrives, `EntitlementService` checks if `status == 'trialing' AND NOW() > trial_ends_at`. If expired:
     - Subscription transitions from `trialing` to `expired` (`is_current = 1` retained until upgraded or cancelled).
     - Tenant transitions from `trialing` to `suspended`.
   - **Scheduled Reconciliation Sweeper:** A periodic background job (`ReconcileSubscriptions`) checks for expired trials daily and flags them without waiting for user traffic.
   - **Behavior on Expiration:**
     - Dashboard remains accessible in read-only mode.
     - Customer data, chat histories, and knowledge items are preserved.
     - Outbound message sending and AI replies are paused.
     - Creation of new agents is blocked.

---

## 13. Sender Limits

### Trial Cap:
- During trial, a tenant is allowed **maximum 1 active WhatsApp sender**.

### Enforcement Entry Points (Centralized):
1. `POST /api/agents` (Create Agent)
2. `POST /api/agents/:id/wa/connect` (Connect WhatsApp)
3. `POST /api/agents/:id/wa/connect-pairing` (Pairing Code Connect)
4. Public API `/v1/*` Agent provisioning

If an agent creation or connection attempt exceeds the plan's `max_active_agents`, the request is rejected with HTTP 403:
`{"error": "Batas kuota nomor WhatsApp tercapai. Silakan upgrade paket Anda."}`

---

## 14. Usage

Lightweight, predictable quota tracking per billing period without heavyweight event streaming.

### Entity Attributes (`usage_counters`):
- `id` (bigint, unsigned, PK)
- `tenant_id` (bigint, unsigned, NOT NULL, FK -> `tenants.id`)
- `metric_key` (varchar(64), NOT NULL): e.g. `monthly_messages`, `monthly_ai_turns`.
- `period_start` (date, NOT NULL): Start date of billing cycle (e.g. `2026-09-01`).
- `period_end` (date, NOT NULL): End date of billing cycle (e.g. `2026-09-30`).
- `used_value` (int unsigned, NOT NULL, DEFAULT 0): Accumulated count.
- `updated_at` (datetime, NOT NULL)

### Uniqueness:
- `UNIQUE KEY uk_usage_counters_tenant_metric_period (tenant_id, metric_key, period_start, period_end)`

---

## 15. Entitlement Service

The central backend authority replacing hardcoded `tenantPlanAllows()` stubs.

### Core Interface:
```go
type EntitlementService interface {
    // Check if a boolean feature is enabled for tenant
    CanUseFeature(tenantID uint, featureKey string) (bool, error)

    // Check if tenant has remaining quota for a numeric metric
    CanConsumeQuota(tenantID uint, metricKey string, amount int) (bool, error)

    // Increment quota usage atomically
    ConsumeQuota(tenantID uint, metricKey string, amount int) error

    // Return max active sender limit for tenant
    GetActiveSenderLimit(tenantID uint) (int, error)

    // Get full subscription & entitlement summary
    GetTenantSubscriptionState(tenantID uint) (*SubscriptionState, error)
}
```

---

## 16. Knowledge Ownership

### Architecture Decision:
- **Decision:** Dual-Scope Knowledge Base (`Tenant` primary scope, optional `Agent` override).
- **Rationale:** Businesses typically share a central product catalog and FAQ across all customer service lines, with optional per-agent customizations.
- **Model:** Table `knowledges` receives `tenant_id` (NOT NULL). `agent_id` becomes optional (`NULL` for tenant-wide shared knowledge).
- **Prerequisite Safety Requirement:** Before modifying `agent_id` to nullable in Phase 2B.2, an application code audit must be performed to identify all Go handlers and RAG queries assuming non-null `agent_id`, categorizing findings into (A) NULL-safe, (B) Requires adjustment, and (C) Assumes agent ownership. All (C) assumptions must be resolved before migration execution.

---

## 17. Contact Ownership

### Architecture Decision:
- **Decision:** Retain Agent-Scoped Contacts for Phase 2B, with additive `tenant_id` indexing.
- **Rationale:** `contacts.agent_id` remains the primary operational ownership boundary. `tenant_id` is an additive denormalized scope/index derived strictly from the owning agent. Authorization must not rely on `tenant_id` alone; queries must maintain consistency between `agent_id` and `tenant_id`. Phase 2B.2 must audit contact queries to ensure no cross-tenant leakage is introduced by the column.

---

## 18. Broadcast / Follow-up / Flow Ownership

- `broadcasts`: Direct `tenant_id` owned. Multi-agent rotation validates that every agent in `agent_ids` belongs to the same `tenant_id`.
- `broadcast_recipients`: Scoped via `broadcast_id -> broadcasts.tenant_id`.
- `follow_ups`: Direct `tenant_id` owned.
- `flows`: Scoped via `agent_id -> agents.tenant_id`.

---

## 19. API Keys / Webhooks

- API keys and Webhook configs remain bound to `models.Agent`.
- Public REST API `/api/v1/*` resolves `agent` via `api_key`.
- `APIKeyMiddleware` verifies:
  1. `agent.APIKey` matches.
  2. Agent's owning tenant has an active subscription allowing `api_access`.
  3. Sets `tenant_id` in context to ensure downstream operations respect tenant boundaries.

---

## 20. Settings

- **Platform Settings (`app_settings`):** Global AI provider keys (OpenRouter, Gemini) and system presets managed exclusively by Super Admin.
- **Tenant Settings:** Reserved for business profile, branding, and notification configurations.
- **Agent Settings (`agents`):** Persona, tone, greeting message, and business hours remain on `Agent`.
- **Legacy `settings` table:** Obsolete; marked for formal deprecation.

---

## 21. Audit Logs

Audit trail for administrative, security, and subscription events.

### Entity Attributes (`audit_logs`):
- `id` (bigint, unsigned, PK)
- `tenant_id` (bigint, unsigned, NULL): Tenant context (NULL for system/platform actions).
- `actor_user_id` (bigint, unsigned, NULL): User performing the action.
- `action` (varchar(64), NOT NULL): e.g. `tenant.created`, `plan.changed`, `agent.deleted`.
- `resource_type` (varchar(64), NOT NULL): `tenant`, `subscription`, `agent`, `user`.
- `resource_id` (varchar(64), NOT NULL)
- `metadata_json` (text, NULL)
- `ip_address` (varchar(45), NULL)
- `created_at` (datetime, NOT NULL)

---

## 22. Billing Readiness

Phase 2B focuses strictly on the internal subscription and entitlement state machine. Payment gateway integrations (Tripay, Midtrans, Stripe), automated recurring billing, invoices, and payment webhooks are decoupled and deferred to future billing phases.

---

## 23. Security Invariants

1. **Isolation Invariant:** A request authenticated under `tenant_id = X` shall never observe, mutate, or delete records belonging to `tenant_id = Y`.
2. **Path Traversal Invariant:** When accessing a sub-resource via `/agents/:id/resource/:rid`, the backend must assert:
   `agent.tenant_id == ctx.tenant_id AND resource.agent_id == agent.id`.
3. **Session Invariant:** WhatsApp engine instances `services.WA(agentID)` are partitioned by global `agentID`. No cross-tenant session lookup is possible.
4. **Super Admin Invariant:** Platform administrators cannot modify tenant business data without explicit audit logging.

---

## 24. Canonical ER Diagram

```mermaid
erDiagram
    TENANTS ||--o{ TENANT_MEMBERS : has
    USERS ||--o{ TENANT_MEMBERS : joins
    TENANTS ||--o{ SUBSCRIPTIONS : holds
    PLANS ||--o{ SUBSCRIPTIONS : provides
    PLANS ||--o{ PLAN_FEATURES : specifies
    TENANTS ||--o{ USAGE_COUNTERS : records
    TENANTS ||--o{ AGENTS : owns
    TENANTS ||--o{ AUDIT_LOGS : logs
    USERS ||--o{ AUDIT_LOGS : performs

    TENANTS {
        bigint id PK
        varchar name
        varchar slug UK
        varchar status
        datetime trial_ends_at
        datetime created_at
        datetime updated_at
    }

    USERS {
        bigint id PK
        varchar username UK
        varchar email
        varchar password
        varchar name
        boolean is_super_admin
        boolean active
    }

    TENANT_MEMBERS {
        bigint id PK
        bigint tenant_id FK
        bigint user_id FK
        varchar role
        varchar status
        datetime created_at
    }

    PLANS {
        bigint id PK
        varchar code UK
        varchar name
        int price
        varchar billing_interval
        boolean is_active
    }

    PLAN_FEATURES {
        bigint id PK
        bigint plan_id FK
        varchar feature_key
        varchar limit_type
        int limit_value
        boolean enabled
    }

    SUBSCRIPTIONS {
        bigint id PK
        bigint tenant_id FK
        bigint plan_id FK
        varchar status
        tinyint is_current UK
        datetime current_period_start
        datetime current_period_end
        datetime trial_ends_at
    }

    USAGE_COUNTERS {
        bigint id PK
        bigint tenant_id FK
        varchar metric_key
        date period_start
        date period_end
        int used_value
    }

    AGENTS {
        bigint id PK
        bigint tenant_id FK
        varchar name
        varchar number
        boolean ai_enabled
    }

    AUDIT_LOGS {
        bigint id PK
        bigint tenant_id FK
        bigint actor_user_id FK
        varchar action
        varchar resource_type
        varchar resource_id
    }
```

---

## 25. Resource Ownership Diagram

```text
=============================================================================
                           RESOURCE OWNERSHIP MODEL
=============================================================================

[ PLATFORM LAYER ]
   â”œâ”€â”€ app_settings (Global AI config & API keys)
   â”œâ”€â”€ shipping_cities (RajaOngkir global cache)
   â”œâ”€â”€ login_throttles (IP/user login rate limits)
   â””â”€â”€ plans & plan_features (Commercial package definitions)

[ TENANT LAYER ] (Directly owned by Tenant ID)
   â”œâ”€â”€ tenants (Workspace record)
   â”œâ”€â”€ tenant_members (User-Tenant relationship)
   â”œâ”€â”€ subscriptions & usage_counters (Billing & quotas)
   â”œâ”€â”€ audit_logs (Workspace audit history)
   â”œâ”€â”€ products, product_orders, product_checkout_sessions
   â”œâ”€â”€ broadcasts & follow_ups
   â”œâ”€â”€ ai_forms, ai_form_sessions, ai_form_submissions
   â”œâ”€â”€ scheduled_messages & scheduled_statuses
   â””â”€â”€ cs_activity_logs & user_agent_assignments

[ AGENT LAYER ] (Owned by Agent ID -> Agent owned by Tenant ID)
   â”œâ”€â”€ agents (WhatsApp virtual assistant profile)
   â”œâ”€â”€ WhatsApp SQLite Session (/var/lib/ruangkirim/whatsapp/wa-session-agent-{id}.db)
   â”œâ”€â”€ chat_histories (29,941 rows - High performance isolation)
   â”œâ”€â”€ inbox_read_states (5,163 conversation cursor states)
   â”œâ”€â”€ contacts (532 customer phone records)
   â”œâ”€â”€ conversation_memories, ai_turns, handoffs
   â”œâ”€â”€ auto_replies, flows, flow_sessions
   â”œâ”€â”€ templates, labels, chat_labels
   â””â”€â”€ crawl_jobs, crawl_pages
=============================================================================
```

---

## 26. Architecture Decision Records (ADRs)

### ADR-01: Tenant Membership Model
- **Decision:** Introduce `tenant_members` join table representing many-to-many relationship between Users and Tenants.
- **Reason:** Real-world businesses require multi-user teams (Owner, Admin, CS) and users who may consult for or own multiple businesses.
- **Alternatives Considered:** Keeping single scalar `users.tenant_id`. Rejected because it permanently blocks multi-workspace collaboration.
- **Trade-offs:** Requires token refresh/switch endpoint when changing workspaces.
- **Impact:** Clean authorization foundation for team expansion.

### ADR-02: Tenant Context Resolution
- **Decision:** Derive active `tenant_id` strictly from JWT claims verified against live database membership; do not trust client query parameters or request body for tenant identification.
- **Reason:** Designed to prevent cross-tenant spoofing.
- **Alternatives Considered:** Passing `X-Tenant-ID` header. Rejected as unsafe without per-request DB validation.
- **Impact:** Eliminates authorization bypass vulnerabilities.

### ADR-03: Plan & Subscription Ownership
- **Decision:** Subscriptions are owned by Tenants, not individual Users.
- **Reason:** A commercial subscription pays for the business workspace and its senders, regardless of which team member manages it.
- **Impact:** If the owner leaves or changes, the workspace and senders remain uninterrupted.

### ADR-04: Trial Model Architecture
- **Decision:** 30-day trial recorded in `subscriptions.trial_ends_at` with `is_current = 1`, and cached on `tenants.trial_ends_at`.
- **Reason:** Dual-recording allows fast workspace status checks on `tenants` while maintaining strict subscription audit trails.
- **Impact:** Clean trial enforcement and straightforward upgrade path.

### ADR-05: Sender Limit Enforcement
- **Decision:** Enforce sender limits centrally in Go service layer at agent creation and WhatsApp connection.
- **Reason:** Prevents bypassing limits via direct API calls or script automation.
- **Impact:** Trial accounts strictly limited to 1 active WhatsApp sender.

### ADR-06: Usage Model
- **Decision:** Synchronous periodic counter table (`usage_counters`) using `(tenant_id, metric_key, period_start, period_end)`.
- **Reason:** Simplest, most reliable implementation without requiring external Redis or Kafka streaming infrastructure.
- **Impact:** Zero additional operational dependencies for VPS hosting.

### ADR-07: Indirect Agent Ownership for Large Tables
- **Decision:** Retain indirect tenant scoping for `chat_histories` (29,941 rows) and `inbox_read_states` (5,163 rows) via `agent_id -> agents.tenant_id`.
- **Reason:** Modifying 30,000+ rows during startup or synchronous migration risks severe table locks, timeouts, and production downtime. Handlers already enforce `resolveAgent()`.
- **Impact:** High operational safety and avoidance of intentional downtime.

### ADR-08: Knowledge Base Scope
- **Decision:** Transition `knowledges` to direct Tenant ownership with an optional `agent_id` override.
- **Reason:** Allows multiple WhatsApp agents in the same tenant to share common business knowledge.
- **Impact:** Eliminates duplicate knowledge entry for multi-agent setups.

### ADR-09: Contact Ownership
- **Decision:** Retain agent-scoped contacts for Phase 2B; defer unified contact deduplication to future CRM phase.
- **Reason:** Minimizes Phase 2B scope and eliminates risk to active customer interactions.
- **Impact:** Safe, incremental rollout.

### ADR-10: AutoMigrate Governance
- **Decision:** Govern GORM `AutoMigrate` so production schema changes are strictly controlled, additive, and executed only through pre-tested scripts.
- **Reason:** Uncontrolled AutoMigrate on startup is a crash hazard for production.
- **Impact:** Predictable, reversible deployments.

### ADR-11: Existing Tenant 1 Migration
- **Decision:** Grandfather Tenant 1 as active internal company tenant (`slug = 'default'`, `status = 'active'`).
- **Reason:** Preserves existing live business operations without forcing artificial trial or payment states.
- **Impact:** Continuity for live Agent 3 WhatsApp operations.

### ADR-12: WhatsApp Session Preservation
- **Decision:** WhatsApp session files must not be intentionally relocated or modified during Phase 2B migration (`wa-session-agent-{id}.db`).
- **Reason:** Preserves active WhatsApp authentication keys and prevents user re-login/QR requirements.
- **Impact:** Session persistence across the migration.

---

## 27. Phase 2B Scope (Must Have)
- Finalize domain model and database specification.
- Govern AutoMigrate and establish controlled migration ordering.
- Add `slug`, `status`, and `trial_ends_at` to `tenants`.
- Create `tenant_members`, `plans`, `plan_features`, `subscriptions`, and `usage_counters`.
- Seed standard plans (`trial`, `starter`, `pro`, `business`) and Tenant 1 grandfathered subscription.
- Replace `tenantPlanAllows()` stubs with centralized `EntitlementService`.
- Enforce maximum 1 sender limit on trial tenants.
- Protect all routes with verified multi-tenant context.

---

## 28. Future Scope (Out of Scope for Phase 2B)
- Automated payment gateway integration (Tripay / Midtrans).
- Invoice generation and automated tax calculation.
- Coupon, affiliate, and discount code engine.
- Shared multi-agent CRM contact deduplication.
- Super Admin tenant impersonation ("login as tenant").
- Single Sign-On (Google / SAML OAuth).

---

## 29. Phase 2B.2 Implementation Prerequisites

Before executing Phase 2B.2 implementation, the following prerequisite planning audits must be completed:
1. **AutoMigrate Runtime Audit:** Inspect startup initialization in Go source code to verify exactly where AutoMigrate is called and implement environment-flagged governance (`AUTO_MIGRATE=false`).
2. **Complete Existing Schema Preflight:** Verify all 45 tables in Staging and Production databases before running DDL.
3. **Knowledge `agent_id` NULL Compatibility Audit:** Audit all Go handlers, queries, and RAG search code for implicit non-null assumptions on `knowledges.agent_id`.
4. **Contact `tenant_id` Query & Mutation Audit:** Review all contact creation, update, and read queries to ensure the addition of `tenant_id` does not weaken agent-level isolation.
5. **Tenant 1 Grandfathering Validation:** Verify Tenant 1 seed queries attach the active grandfathered subscription with `is_current = 1`.
6. **Subscription Current-Record Invariant Validation:** Verify MySQL unique index behavior for `(tenant_id, is_current)` under concurrent write simulation.
7. **Trial/Subscription State Transition Test Design:** Draft test cases for trial expiration, request-time evaluation, and tenant operational suspension.
8. **Tenant Isolation Test Design:** Draft automated tests verifying cross-tenant data access is rejected with HTTP 404/403.
9. **Sender Quota Test Design:** Draft test cases asserting that creating or connecting a second sender under trial returns HTTP 403.
10. **Backup and Rollback Verification Plan:** Document pre-migration mysqldump procedures and binary downgrade steps.

---

## 30. Final Domain Model Verdict

**CANONICAL DOMAIN MODEL: APPROVED FOR SPECIFICATION**
The refined architecture satisfies all multi-tenant SaaS requirements, guarantees backward compatibility for existing user logins, establishes strong database-level subscription constraints, and provides a safe path for Phase 2B database migration.
