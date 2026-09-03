# Ruangkirim SaaS Subscription & Super Admin Roadmap

> **Status:** Planning baseline
> **Target branch:** `develop`
> **Primary preview:** `https://dev.ruangkirim.web.id`
> **Purpose:** Single implementation guide and checklist for converting Ruangkirim from a single-application WhatsApp platform into a subscription SaaS with trial limits, plan entitlements, billing lifecycle, and Super Admin management.

---

## 1. Product Goal

Ruangkirim will support multiple registered users/tenants with subscription-based access.

### User lifecycle

1. User registers an account.
2. User receives a **one-time 30-day Free Trial**.
3. Trial is limited to **1 WhatsApp sender** and a restricted feature set.
4. When the trial expires, premium/restricted functionality is blocked until the user has an active subscription.
5. Users can subscribe to **Pro** or higher tiers.
6. Entitlements are enforced by backend policy, not frontend visibility alone.
7. Super Admin can manage users, plans, subscriptions, feature access, limits, trials, and operational status.

### Core principle

> **Billing state and feature limits must be enforced server-side. The frontend is only a presentation layer for allowed/blocked features.**

---

# 2. Recommended Roadmap Placement

## Decision: implement SaaS foundation before the large Phase 2 frontend rebuild

The subscription system changes authentication, authorization, navigation, dashboard data, feature visibility, and backend data ownership. Therefore it should not be added only after the frontend is fully rebuilt.

Recommended order:

- **Phase 0:** SaaS architecture and audit
- **Phase 1:** Identity, tenant and entitlement foundation
- **Phase 2:** Plans, trial and subscription lifecycle
- **Phase 3:** Super Admin
- **Phase 4:** Billing/payment integration
- **Phase 5:** Customer SaaS frontend and Phase 2 UI refactor
- **Phase 6:** Production hardening and rollout

The current frontend rebuild work can continue only for shared UI foundations that do not lock in incorrect billing assumptions. Large dashboard rewrites should wait until the entitlement contract is defined.

---

# 3. Phase 0 — Architecture & Baseline Audit

## Goal

Freeze a safe implementation baseline before changing authentication or data models.

### Checklist

- [ ] Confirm current authentication flow and user model.
- [ ] Identify current backend database schema and persistence layer.
- [ ] Map all API routes.
- [ ] Identify every feature requiring plan restrictions.
- [ ] Identify every resource requiring a quota: sender, broadcast, contacts, agents, team members, etc.
- [ ] Define tenant ownership for all user-generated resources.
- [ ] Define Super Admin authorization separately from normal customer roles.
- [ ] Document trial state transitions.
- [ ] Document subscription state transitions.
- [ ] Define API error format for `subscription_required`, `feature_not_available`, and `limit_reached`.
- [ ] Create a migration and rollback strategy.
- [ ] Verify staging backup before schema changes.

### Deliverables

- [ ] `docs/SAAS-DOMAIN-MODEL.md`
- [ ] `docs/SAAS-ENTITLEMENT-MATRIX.md`
- [ ] `docs/SAAS-API-CONTRACT.md`
- [ ] database migration plan

### Exit criteria

- [ ] No implementation starts without an agreed tenant and entitlement model.
- [ ] Existing development environment remains deployable.

---

# 4. Phase 1 — Identity, Tenant & Authorization Foundation

## Goal

Introduce the ownership model required for a multi-user SaaS.

## Recommended core entities

### `users`

Authentication identity and global account data.

Suggested fields:

- `id`
- `email`
- `password_hash` or external identity reference
- `name`
- `role`
- `status`
- `email_verified_at`
- timestamps

### `tenants` / `workspaces`

Customer account boundary.

Suggested fields:

- `id`
- `owner_user_id`
- `name`
- `status`
- timestamps

### `tenant_members`

Membership and customer-side roles.

Suggested fields:

- `tenant_id`
- `user_id`
- `role`
- timestamps

### Roles

Initial recommendation:

- `super_admin`
- `owner`
- `admin`
- `member`

Do not mix customer roles with platform-level Super Admin privileges.

### Checklist

- [ ] Add tenant/workspace ownership model.
- [ ] Add tenant membership model if multi-user teams are supported.
- [ ] Attach tenant context to authenticated requests.
- [ ] Update resource queries to scope data by tenant.
- [ ] Add authorization middleware/service.
- [ ] Prevent cross-tenant resource access.
- [ ] Add platform-level `super_admin` authorization.
- [ ] Add audit logging for privileged Super Admin actions.
- [ ] Add tests for authorization boundaries.

### Exit criteria

- [ ] Every customer resource has an owner/tenant boundary.
- [ ] One tenant cannot read or mutate another tenant's resources.
- [ ] Super Admin access is explicit and auditable.

---

# 5. Phase 2 — Plans, Free Trial & Entitlements

## Goal

Create the commercial access engine before integrating payment.

## Initial plan model

### Trial

- Duration: **30 days**
- Sender limit: **1 sender**
- Limited feature access
- One trial entitlement per account/tenant according to final policy
- Trial cannot be silently reset by repeated registration

### Pro

Paid entry tier.

### Higher tiers

Recommended placeholders until commercial limits are finalized:

- Pro
- Business
- Enterprise

Actual names and limits should be editable from Super Admin rather than hard-coded throughout the application.

## Recommended entities

### `plans`

- `id`
- `code`
- `name`
- `description`
- `is_active`
- `is_public`
- `sort_order`

### `plan_features`

Feature entitlements for each plan.

- `plan_id`
- `feature_key`
- `enabled`
- optional limit/value

### `subscriptions`

- `id`
- `tenant_id`
- `plan_id`
- `status`
- `started_at`
- `current_period_start`
- `current_period_end`
- `trial_ends_at`
- cancellation metadata
- timestamps

### `usage_counters` or usage service

For quota-controlled features.

Examples:

- active senders
- contacts
- broadcasts per period
- messages per period
- team members
- automation flows

## Suggested subscription statuses

- `trialing`
- `active`
- `past_due`
- `grace_period`
- `canceled`
- `expired`
- `suspended`

### Checklist

- [ ] Create plans data model.
- [ ] Create feature entitlement data model.
- [ ] Create subscription data model.
- [ ] Implement automatic 30-day trial at registration.
- [ ] Enforce one-sender trial limit.
- [ ] Create centralized `CanUseFeature()` / entitlement service.
- [ ] Create centralized quota checking.
- [ ] Block restricted API operations server-side.
- [ ] Return machine-readable restriction errors.
- [ ] Add frontend plan/upgrade state handling.
- [ ] Add subscription status endpoint.
- [ ] Add trial remaining-days endpoint/calculation.
- [ ] Add tests for expiry and limit boundaries.

### Exit criteria

- [ ] A trial user cannot exceed one sender through direct API calls.
- [ ] Expired trial users cannot bypass restrictions through frontend manipulation.
- [ ] Plan rules are not duplicated across many handlers.

---

# 6. Phase 3 — Super Admin Dashboard

## Goal

Provide platform-level management without exposing customer administration across tenants.

## Navigation areas

### Overview

- total users
- active tenants
- active subscriptions
- trialing accounts
- expiring trials
- monthly recurring revenue when billing is enabled
- recent system events

### User Management

- search/filter users
- account status
- verification status
- tenant ownership
- subscription summary
- suspend/reactivate
- controlled administrative actions

### Subscription Management

- current plan
- status
- trial end
- period end
- cancellation state
- manual plan changes with audit trail

### Plan Management

- create/edit/deactivate plans
- manage public availability
- manage limits
- manage enabled features
- ordering and pricing metadata

### Feature Management

- feature catalog
- plan-to-feature mapping
- quota configuration

### Sender Management

- sender count by tenant
- status
- connection health summary
- administrative suspension where justified

### Audit & Operations

- Super Admin audit log
- failed billing events
- webhook/event processing status
- system health links

### Checklist

- [ ] Add separate `/admin` route namespace.
- [ ] Add backend Super Admin middleware.
- [ ] Build overview dashboard.
- [ ] Build user management.
- [ ] Build tenant management.
- [ ] Build plan management.
- [ ] Build feature entitlement management.
- [ ] Build subscription management.
- [ ] Build audit log view.
- [ ] Add confirmation flows for destructive actions.
- [ ] Add pagination and search for large datasets.

### Exit criteria

- [ ] Normal users cannot access `/admin` APIs or pages.
- [ ] Privileged changes are auditable.

---

# 7. Phase 4 — Billing & Payment Integration

## Goal

Connect subscriptions to a payment provider after the internal subscription engine works.

## Principles

- Provider webhooks are the source of payment event processing.
- Never trust frontend payment success alone.
- Verify webhook signatures.
- Make webhook processing idempotent.
- Keep provider-specific logic behind a billing adapter.

## Recommended capabilities

- checkout/payment request creation
- invoice/payment records
- renewal processing
- webhook handling
- plan upgrade/downgrade
- cancellation
- grace period
- failed payment handling

### Checklist

- [ ] Select payment provider.
- [ ] Create billing provider adapter interface.
- [ ] Create payment/invoice persistence model.
- [ ] Implement checkout initiation.
- [ ] Implement verified webhook endpoint.
- [ ] Implement idempotency handling.
- [ ] Map provider events to internal subscription states.
- [ ] Test duplicate webhook delivery.
- [ ] Test failed payment.
- [ ] Test renewal.
- [ ] Test cancellation.

### Exit criteria

- [ ] Subscription access changes are based on verified backend events.
- [ ] Duplicate payment events cannot double-activate or corrupt a subscription.

---

# 8. Phase 5 — Customer SaaS Frontend & UI Refactor

## Goal

Continue the frontend rebuild with the SaaS domain model already available.

## Customer-facing additions

- [ ] subscription status banner
- [ ] trial countdown
- [ ] upgrade modal/page
- [ ] current plan page
- [ ] billing history
- [ ] quota usage indicators
- [ ] feature lock states
- [ ] sender-limit feedback
- [ ] plan comparison page

## Existing Phase 2 frontend refactor

The current large components should be refactored progressively after the API contracts are stable. High-priority files identified in the existing audit include:

- `src/pages/Dashboard.tsx`
- `src/components/InboxPanel.tsx`
- `src/hooks.ts`
- `src/components/ApiPanel.tsx`
- `src/components/ProductPanel.tsx`
- `src/components/BroadcastPanel.tsx`

Recommended refactor rules:

- [ ] Do not combine billing logic with presentation components.
- [ ] Use reusable domain hooks/services for subscription state.
- [ ] Keep backend entitlement checks authoritative.
- [ ] Split oversized dashboard modules by feature domain.
- [ ] Reuse shared UI components and design tokens.
- [ ] Keep customer and Super Admin layouts separate.

### Exit criteria

- [ ] UI accurately represents backend subscription state.
- [ ] Locked features cannot be used by bypassing the UI.
- [ ] Dashboard refactor does not regress current functionality.

---

# 9. Phase 6 — Hardening, Migration & Launch

## Checklist

### Security

- [ ] Authorization tests.
- [ ] Cross-tenant isolation tests.
- [ ] Admin privilege tests.
- [ ] Payment webhook signature tests.
- [ ] Rate limiting where required.
- [ ] Sensitive audit logging review.

### Data

- [ ] Backup before migration.
- [ ] Migration rollback plan.
- [ ] Existing user migration strategy.
- [ ] Existing sender ownership migration.
- [ ] Existing WhatsApp session data migration strategy.

### Operations

- [ ] `/health` remains available.
- [ ] staging deployment verified.
- [ ] production deployment verified.
- [ ] rollback procedure documented.
- [ ] monitoring and logs reviewed.

### Launch

- [ ] internal admin testing
- [ ] trial registration testing
- [ ] trial expiration testing
- [ ] plan upgrade testing
- [ ] payment webhook testing
- [ ] production smoke test

---

# 10. Feature Entitlement Matrix (Initial Draft)

| Feature | Trial | Pro | Business | Enterprise |
|---|---|---|---|---|
| WhatsApp senders | 1 | configurable | configurable | configurable |
| Dashboard | limited | yes | yes | yes |
| Broadcast | limited | yes | higher limits | custom |
| Automation | limited | yes | yes | yes |
| Contacts | limited | configurable | higher limits | custom |
| Team members | 1 / limited | configurable | higher limits | custom |
| Advanced analytics | no / limited | optional | yes | yes |
| Priority support | no | standard | priority | dedicated |

> Exact limits should be finalized as product/business decisions and stored in plan configuration rather than scattered constants.

---

# 11. Implementation Sequence for AI Agents

AI agents working on this repository should follow this order:

1. Read this roadmap and existing architecture documents.
2. Audit current database/auth implementation before changing code.
3. Create a dedicated implementation branch from `develop` when the work is large.
4. Implement one phase at a time.
5. Run backend tests and frontend build after each meaningful change.
6. Commit only source/docs changes; never commit `.env`, binaries, or runtime databases.
7. Push to `develop` only after the phase checkpoint passes.
8. GitHub Actions deploys `develop` to `dev.ruangkirim.web.id`.
9. Review the live development environment before merging/promoting to `main`.
10. Never treat a successful deployment as proof that authorization and billing rules are correct; run functional checks.

---

# 12. Current Repository Safety Rules

The repository already uses these runtime separation principles:

- Production: `main` -> `/var/www/ruangkirim` -> port `3031`
- Development: `develop` -> `/var/www/ruangkirim-staging` -> port `3032`
- Development deployment workflow: `.github/workflows/deploy-dev.yml`
- Production deployment workflow: `.github/workflows/deploy-production.yml`
- Runtime databases and binaries must remain ignored by Git.

For the current WhatsApp runtime database:

- `wa-assistant.db`
- `wa-assistant.db-wal`
- `wa-assistant.db-shm`

must remain untracked runtime artifacts.

---

# 13. Immediate Next Step

## Recommended next implementation task

**Phase 0: perform a backend architecture audit specifically for authentication, user identity, current database schema, resource ownership, and tenant readiness.**

Do **not** start payment integration or redesign all frontend screens before this audit is complete.

### Phase 0 completion checklist

- [ ] Inspect `backend/` architecture.
- [ ] Identify authentication storage and token flow.
- [ ] Identify current database connection and schema.
- [ ] Identify all API route groups.
- [ ] Identify resource ownership gaps.
- [ ] Propose tenant and subscription migrations.
- [ ] Produce a separate implementation specification before modifying production data.
