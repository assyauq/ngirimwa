# Ruangkirim SaaS Phase 1.1 — Database & Migration Specification

## Status
- [x] Architecture baseline documented
- [ ] Database migration implemented
- [ ] Production migration executed

## Goal
Transform the current single-installation data model into a safe multi-tenant SaaS foundation without rewriting unrelated WhatsApp functionality.

## Core entities

### 1. tenants
Represents a customer workspace/account.

Fields:
- id
- name
- slug (unique)
- status: active | suspended | cancelled
- trial_ends_at
- created_at
- updated_at

### 2. tenant_members
Links users to tenants.

Fields:
- id
- tenant_id (FK)
- user_id (FK)
- role: owner | admin | member
- status: active | invited | disabled
- created_at
- updated_at

Unique index: `(tenant_id, user_id)`.

### 3. plans
Defines sellable subscription packages.

Fields:
- id
- code (unique, e.g. trial, pro, business, enterprise)
- name
- description
- billing_interval: monthly | yearly
- price
- currency
- is_active
- sort_order
- created_at
- updated_at

### 4. plan_features
Defines feature flags and limits per plan.

Fields:
- id
- plan_id (FK)
- feature_key
- enabled
- limit_value (nullable)
- created_at
- updated_at

Examples:
- max_agents
- max_team_members
- broadcast_enabled
- api_enabled
- flow_enabled
- crm_enabled
- ai_enabled
- analytics_enabled

### 5. subscriptions
Current and historical subscription records.

Fields:
- id
- tenant_id (FK)
- plan_id (FK)
- status: trialing | active | past_due | cancelled | expired
- started_at
- trial_ends_at
- current_period_start
- current_period_end
- cancelled_at
- provider
- provider_subscription_id
- created_at
- updated_at

### 6. subscription_events
Immutable subscription lifecycle history.

Fields:
- id
- subscription_id (FK)
- event_type
- payload_json
- occurred_at
- created_at

Examples:
- trial_started
- trial_expired
- payment_success
- payment_failed
- subscription_upgraded
- subscription_cancelled

### 7. usage_counters
Optional aggregate counters for usage-limited plans.

Fields:
- id
- tenant_id
- feature_key
- period_start
- period_end
- used_value
- updated_at

Unique index: `(tenant_id, feature_key, period_start, period_end)`.

### 8. audit_logs
Platform-level audit trail.

Fields:
- id
- actor_user_id (nullable)
- tenant_id (nullable)
- action
- resource_type
- resource_id
- metadata_json
- ip_address (optional)
- created_at

## Trial policy
Default first-release policy:

- One trial per customer/workspace.
- Trial duration: 30 days.
- Trial plan has `max_agents = 1`.
- Restricted features are denied centrally by feature/limit guards.
- Trial expiration must not delete customer data.
- On expiration, workspace becomes read-only/restricted according to policy until a paid subscription is active.

## Tenant migration strategy

### Step A — Preserve current data
Do not drop or rename existing tables destructively.

### Step B — Bootstrap existing installation
Create one bootstrap tenant representing the existing production owner/workspace.

Recommended logical values:
- slug: `default`
- status: `active`

### Step C — Assign legacy records
Existing tenant-owned data must be assigned to the bootstrap tenant.

Priority review targets:
- agents
- users
- settings
- team users
- contacts
- broadcasts
- templates
- products
- knowledge
- flows
- follow-ups
- other agent-owned resources

### Step D — Enforce new ownership for new data
New customer data must always derive `tenant_id` from authenticated membership/server-side context, not from an arbitrary request body field.

## Migration order

1. Add SaaS tables.
2. Add indexes and foreign keys where safe.
3. Seed trial/pro/business/enterprise plans.
4. Create bootstrap tenant.
5. Create bootstrap subscription for existing tenant.
6. Backfill tenant ownership where required.
7. Add server-side tenant resolution middleware/context.
8. Add subscription status guard.
9. Add feature and quota guard.
10. Validate with staging before production.

## Rollback principles

- Prefer additive migrations.
- Never delete production data in the same migration that introduces SaaS tables.
- Backfill in idempotent steps where possible.
- Test migration against a production-like database backup before live execution.

## Implementation checklist

- [ ] Inventory existing GORM models
- [ ] Identify all existing TenantID fields
- [ ] Identify models requiring TenantID
- [ ] Design migration file(s)
- [ ] Implement SaaS models
- [ ] Seed initial plans
- [ ] Implement bootstrap tenant migration
- [ ] Implement subscription seed
- [ ] Add indexes
- [ ] Run backend tests
- [ ] Build frontend
- [ ] Deploy to dev/staging
- [ ] Verify dev.ruangkirim.web.id
- [ ] Production migration review

## Exit criteria
Phase 1.1 is complete when the schema can represent multiple independent customer tenants, each tenant can have membership and subscription state, and trial/plan limits can be enforced by backend guards without breaking legacy WhatsApp data.
