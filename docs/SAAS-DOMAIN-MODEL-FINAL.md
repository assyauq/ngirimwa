# RUANGKIRIM â€” CANONICAL SAAS DOMAIN MODEL SPECIFICATION
**Project:** Ruangkirim
**Repository:** `mrifatsyauqi/ruangkirim`
**Document:** Final SaaS Domain Model
**Phase:** 2B.1
**Status:** APPROVED DESIGN SPECIFICATION (ZERO IMPLEMENTATION / READ-ONLY)
**Date:** September 11, 2026

---

## 1. Executive Summary

This document establishes the canonical SaaS domain model for Ruangkirim, evolving the platform from an internal single-tenant company deployment into a robust, multi-tenant subscription SaaS.

The architecture is designed to fulfill strict commercial and operational requirements:
1. **Multi-Tenancy:** True workspace isolation where each customer company operates within an independent Tenant boundary.
2. **Data & Session Integrity:** Total preservation of existing production data (Tenant 1, User 1, Agent 3, 29,941 chat history records) and live WhatsApp sessions (`/var/lib/ruangkirim/whatsapp/wa-session-agent-3.db`).
3. **SaaS Commercial Lifecycle:** Database-driven plans, entitlements, 30-day self-service trials, strict 1-sender trial caps, and usage tracking.
4. **Security Boundaries:** Strict cryptographic and database separation between platform administration (Super Admin) and tenant administration (Tenant Owner/Admin/CS).

---

## 2. Product SaaS Model

Ruangkirim is a multi-tenant WhatsApp AI Assistant and CRM platform for Indonesian businesses.

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
- `status` (varchar(24), NOT NULL, DEFAULT 'active'): Operational state.
- `trial_ends_at` (datetime, NULL): Timestamp when the trial period concludes.
- `created_at` (datetime, NOT NULL)
- `updated_at` (datetime, NOT NULL)

### Tenant State Machine:
```text
[ Registration ]
       â”‚
       â–¼
   trialing â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–º active â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â–º past_due
       â”‚                   â–²                     â”‚
       â”‚ (30 days expire)  â”‚ (Payment)           â”‚ (Grace period ends)
       â–¼                   â”‚                     â–¼
    expired â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜                 suspended
       â”‚                                         â”‚
       â–¼                                         â–¼
   cancelled â—„â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜
```

- `trialing`: Workspace in active 30-day trial mode. Limited to 1 sender.
- `active`: Workspace on an active paid subscription or grandfathered internal tier.
- `past_due`: Subscription renewal failed; entering 3-day grace period.
- `suspended`: Workspace blocked from sending outbound messages due to non-payment.
- `cancelled`: Workspace explicitly terminated by owner or superadmin. Data retained in read-only mode.

### Grandfathered Baseline:
Existing production Tenant (`id = 1`) is assigned `slug = 'default'`, `status = 'active'`, and `trial_ends_at = NULL`. It is not subject to trial expiration.

---

## 4. User

The `User` entity represents human authentication identity across the platform.

### Architectural Evolution:
- **Current Limitation:** `users.tenant_id` ties a user to at most one tenant, and `users.username` has a global unique constraint preventing common usernames (e.g. `cs1`, `admin`) across tenants.
- **Target Architecture:** Identity is anchored globally by `email` and `id`. Multi-tenant association is established via `tenant_members`.
- **Phase 2B Backward Compatibility:** To prevent breaking existing logins, `users.username` remains unique globally during Phase 2B, but application authentication decouples tenant context from `users.tenant_id`.

### Entity Attributes:
- `id` (bigint, unsigned, PK): System user ID.
- `email` (varchar(255), NOT NULL, UNIQUE): Canonical identity and communications anchor.
- `username` (varchar(64), NOT NULL, UNIQUE): Login username.
- `password` (varchar(255), NOT NULL): Bcrypt password hash.
- `name` (varchar(128), NOT NULL): Display name.
- `phone` (varchar(32), NULL): Contact phone number.
- `is_super_admin` (boolean, NOT NULL, DEFAULT false): Platform-level operator flag.
- `active` (boolean, NOT NULL, DEFAULT true): Account enable/disable switch.
- `email_verified` (boolean, NOT NULL, DEFAULT false)
- `created_at` / `updated_at` (datetime)

---

## 5. Tenant Membership

The `TenantMember` entity represents the relationship between a `User` and a `Tenant`, defining the user's role and permissions within that specific workspace.

### Entity Attributes:
- `id` (bigint, unsigned, PK)
- `tenant_id` (bigint, unsigned, NOT NULL, FK -> `tenants.id`)
- `user_id` (bigint, unsigned, NOT NULL, FK -> `users.id`)
- `role` (varchar(24), NOT NULL): Workspace role (`owner`, `admin`, `cs`).
- `status` (varchar(24), NOT NULL, DEFAULT 'active'): `active`, `invited`, `disabled`.
- `created_at` / `updated_at` (datetime)

### Uniqueness:
- `UNIQUE KEY uk_tenant_user (tenant_id, user_id)`: A user can belong to a given tenant at most once.

### Role Hierarchy:
1. `owner`: Full control over workspace, billing, plan changes, team management, and agent deletion. Cannot be deleted without transferring ownership.
2. `admin`: Can create/manage agents, configure knowledge base, view analytics, and manage CS users. Cannot modify billing or delete workspace.
3. `cs` (Customer Service): Operational role restricted by `CSRouteGuard`. Can only interact with conversations, contacts, and send messages on assigned agents (`user_agent_assignments`).

---

## 6. Authentication & Tenant Context

To avoid multi-tenant data leakage while supporting users who belong to multiple workspaces, the system uses a **Cryptographically Bound Active Tenant Context**.

### Authentication Flow:
1. **User Login (`POST /api/login`):** User submits credentials (`username`/`email` + `password`).
2. **Tenant Membership Discovery:** Backend queries `tenant_members` for all active memberships associated with `user.ID`.
3. **Token Minting:**
   - If user has 1 tenant, that tenant is selected automatically.
   - If user has multiple tenants, the last accessed tenant or specified tenant is selected.
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

**Rule:** The client CANNOT manipulate `tenant_id` via HTTP headers or body; it is strictly derived from the verified JWT and live database membership check.

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

### Entity Attributes:
- `id` (bigint, unsigned, PK)
- `tenant_id` (bigint, unsigned, NOT NULL, FK -> `tenants.id`)
- `plan_id` (bigint, unsigned, NOT NULL, FK -> `plans.id`)
- `status` (varchar(24), NOT NULL): `trialing`, `active`, `past_due`, `cancelled`, `expired`.
- `current_period_start` (datetime, NOT NULL)
- `current_period_end` (datetime, NOT NULL)
- `trial_ends_at` (datetime, NULL)
- `cancel_at_period_end` (boolean, NOT NULL, DEFAULT false)
- `payment_provider` (varchar(32), NULL): `manual`, `tripay`, `midtrans`, `xendit`.
- `external_reference` (varchar(128), NULL): External payment/subscription identifier.
- `created_at` / `updated_at` (datetime)

### Active Subscription Invariant:
A tenant can have at most **one** subscription in `trialing` or `active` state at any given moment.

---

## 12. Trial

### Specifications:
1. **Creation:** Upon self-service registration, a Tenant is created with `status = 'trialing'` and `trial_ends_at = NOW() + 30 days`.
2. **Initial Subscription:** A subscription is created linking Tenant to the `trial` plan (`status = 'trialing'`).
3. **Expiration:** When `NOW() > trial_ends_at`:
   - Subscription transitions to `expired`.
   - Tenant transitions to `expired`.
4. **Behavior on Expiration:**
   - **Login & Dashboard:** Remains accessible (READ-ONLY).
   - **Existing Data:** 100% preserved; no customer, chat, or knowledge data is deleted.
   - **WhatsApp Session:** Remains linked but background message processing & AI replies are paused.
   - **Outbound Sending:** Blocked (UI displays upgrade paywall modal).
   - **New Agent Creation:** Blocked.

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
- `period_start` (date, NOT NULL): Start of billing cycle (e.g. `2026-09-01`).
- `period_end` (date, NOT NULL): End of billing cycle (e.g. `2026-09-30`).
- `used_value` (int, NOT NULL, DEFAULT 0): Current count.
- `updated_at` (datetime, NOT NULL)

### Uniqueness:
- `UNIQUE KEY uk_tenant_metric_period (tenant_id, metric_key, period_start, period_end)`

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
- **Rationale:** Most businesses have 1 canonical FAQ, catalog, and company profile shared across all CS agents. However, specific agents may require customized prompts or specialized knowledge.
- **Model:** Table `knowledges` receives `tenant_id` (NOT NULL). `agent_id` becomes optional/nullable.
  - If `agent_id IS NULL`: Knowledge is tenant-global (accessible by all agents of that tenant).
  - If `agent_id IS NOT NULL`: Knowledge is specific to that particular agent.

---

## 17. Contact Ownership

### Architecture Decision:
- **Decision:** Retain Agent-Scoped Contacts for Phase 2B, with additive `tenant_id` index.
- **Rationale:** Rewriting the contact/CRM system into a shared multi-agent address book requires complex contact-merging logic, deduplication, and thread ownership transfer. For Phase 2B isolation, scoping via `agent_id` (where agent belongs to tenant) provides complete security with zero regression risk.

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
  3. Sets `tenant_id` in context to ensure all downstream operations respect tenant boundaries.

---

## 20. Settings

- **Platform Settings (`app_settings`):** Global AI provider keys (OpenRouter, Gemini) and system presets managed exclusively by Super Admin.
- **Tenant Settings:** Future table for business profile, branding, and billing notifications.
- **Agent Settings (`agents`):** Tone, greeting messages, business hours, and persona prompts remain on the `Agent` entity.
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

The subscription model is abstracted from third-party payment gateways (Tripay, Midtrans, Stripe). Future payment integration requires only a webhook handler mapping gateway events to subscription state transitions (`trialing -> active`, `active -> past_due`).

---

## 23. Security Invariants

1. **Isolation Invariant:** A request authenticated under `tenant_id = X` shall **never** observe, mutate, or delete records belonging to `tenant_id = Y`.
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
        varchar email UK
        varchar username UK
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
- **Reason:** Guarantees zero cross-tenant spoofing.
- **Alternatives Considered:** Passing `X-Tenant-ID` header. Rejected as unsafe without per-request DB validation.
- **Impact:** Eliminates an entire class of authorization bypass vulnerabilities.

### ADR-03: Plan & Subscription Ownership
- **Decision:** Subscriptions are owned by Tenants, not individual Users.
- **Reason:** A commercial subscription pays for the business workspace and its senders, regardless of which team member manages it.
- **Impact:** If the owner leaves or changes, the workspace and senders remain uninterrupted.

### ADR-04: Trial Model Architecture
- **Decision:** 30-day trial recorded in `tenants.trial_ends_at` and backed by a `trialing` subscription row.
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
- **Impact:** 100% production safety and zero migration downtime.

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
- **Reason:** Uncontrolled AutoMigrate on startup is a P0 crash hazard for production.
- **Impact:** Predictable, reversible deployments.

### ADR-11: Existing Tenant 1 Migration
- **Decision:** Grandfather Tenant 1 as active internal company tenant (`slug = 'default'`, `status = 'active'`).
- **Reason:** Preserves existing live business operations without forcing artificial trial or payment states.
- **Impact:** Seamless continuity for live Agent 3 WhatsApp operations.

### ADR-12: WhatsApp Session Preservation
- **Decision:** Never alter, relocate, or re-namespace WhatsApp SQLite session files on disk (`wa-session-agent-{id}.db`).
- **Reason:** Preserves active WhatsApp authentication keys and prevents user re-login/QR requirements.
- **Impact:** 100% session persistence across the migration.

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

## 29. Final Domain Model Verdict

**CANONICAL DOMAIN MODEL: APPROVED FOR SPECIFICATION**
The proposed architecture fully satisfies all multi-tenant SaaS requirements, eliminates historical documentation contradictions, strictly protects live production operations, and provides a clear, safe path for Phase 2B database migration.
