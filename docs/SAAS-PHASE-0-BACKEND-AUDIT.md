# Ruangkirim SaaS Phase 0 — Backend Architecture Audit

> **Status:** Baseline audit
> **Branch:** `develop`
> **Scope:** Read-only architecture assessment before SaaS schema implementation

## 1. Current Baseline

The backend is a Go application using Gin, GORM, and MySQL. Database startup is centralized in `backend/database/database.go` and uses environment variables `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, and `DB_NAME`.

The current startup migration uses `AutoMigrate` for a large set of existing application models. This means SaaS changes must be introduced carefully so existing production and development data are not accidentally reinterpreted.

## 2. Authentication Baseline

Current API routing already has authenticated routes and explicit role guards, including:

- `AuthMiddleware()`
- `CSRouteGuard()`
- `RequireSuperAdmin()`
- `RequireTenantAdmin()`

This is a useful foundation, but SaaS authorization must distinguish three separate concepts:

1. authenticated identity;
2. customer workspace/tenant membership;
3. platform-level Super Admin authority.

These concepts must not be merged into one generic role check.

## 3. Current Tenant Readiness

The current `Tenant` model exists, but its own source comment describes it as a single internal company installation. Therefore the current model is **not yet a complete multi-tenant SaaS boundary**.

The current `Agent` model already contains `TenantID`, which is a strong migration starting point. However, many application resources are primarily scoped by `AgentID`. The Phase 1 design must establish that every agent belongs to exactly one SaaS tenant and that access to every agent-scoped resource is validated against the current tenant.

## 4. Resource Ownership Findings

The application is largely organized around the WhatsApp `Agent` entity. Many domain resources are linked through `AgentID`, including examples such as chats, contacts, knowledge, flows, templates, follow-ups, products, and other feature data.

Recommended ownership chain:

`User -> Tenant membership -> Tenant -> Agent -> Agent-scoped resources`

The backend must not trust an `agent_id` from a URL by itself. Every request must verify that the requested agent belongs to the authenticated tenant.

## 5. SaaS Migration Risks

### High priority

- Existing tenant semantics are single-installation oriented.
- Existing records may not all have an explicit tenant ownership column.
- WhatsApp session files are runtime state and must not be treated as SaaS database migrations.
- `AutoMigrate` is convenient for additive schema changes but is not a complete migration strategy for destructive or data-backfill changes.

### Required safety controls

- backup before data migration;
- explicit data backfill for tenant ownership;
- staging validation before production;
- rollback procedure for each destructive change;
- automated cross-tenant authorization tests.

## 6. Recommended Phase 1 Data Direction

### Platform identity

- `users`
- `tenants`
- `tenant_members`

### Commercial domain

- `plans`
- `plan_features`
- `subscriptions`
- optional `subscription_events`
- optional `usage_counters`

### Platform administration

- `audit_logs`

## 7. Trial Policy

Initial product rule:

- first registration creates one trial entitlement;
- trial duration is 30 days;
- trial allows one active WhatsApp sender;
- trial restrictions must be enforced server-side;
- repeated registration must not reset trial eligibility.

The final anti-abuse identity policy must be implemented explicitly. Email uniqueness alone is not sufficient to guarantee one trial per real-world person.

## 8. Phase 0 Decisions Required Before Coding

- [ ] Define whether one user may belong to multiple tenants.
- [ ] Define whether a tenant may have multiple owners.
- [ ] Define whether sender limit means created agents or connected active senders.
- [ ] Finalize Trial feature matrix.
- [ ] Finalize initial Pro / Business / Enterprise limits.
- [ ] Select the future billing provider.
- [ ] Decide billing period rules.
- [ ] Define existing-user migration policy.

## 9. Recommended Next Implementation

Proceed with **Phase 1.1 — SaaS Domain Model Specification**, followed by an explicit additive database migration plan. Do not yet change frontend screens or payment integration.
