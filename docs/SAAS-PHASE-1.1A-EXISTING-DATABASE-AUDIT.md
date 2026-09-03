# Ruangkirim SaaS Phase 1.1A — Existing Database Audit

## Status
- [x] Read-only source audit completed
- [x] Existing tenant model identified
- [x] Existing user tenant linkage identified
- [x] Agent tenant ownership identified
- [x] Feature-gating baseline identified
- [ ] Migration implementation started

## Audit conclusion
Ruangkirim is **not a blank single-tenant database**. The current codebase already contains an early multi-tenant foundation, but the model is intentionally limited to an internal-company edition and subscription enforcement is currently a no-op.

The Phase 1 SaaS work should therefore **extend the existing tenant architecture** instead of replacing it.

## 1. Existing tenant architecture

### Tenant
The current `models.Tenant` contains only:
- `ID`
- `Name`
- `CreatedAt`
- `UpdatedAt`

Current source comments describe it as one internal company and assume a single default tenant.

### User
The existing user model already contains:
- `TenantID *uint`
- `IsSuperAdmin bool`
- `Active bool`
- role information

Authentication middleware already resolves `tenant_id` into the request context when the user has a tenant.

### Agent
`models.Agent` already has:
- `TenantID uint`
- index
- `not null`

This is the correct primary ownership boundary for WhatsApp senders.

## 2. Existing tenant-aware domain models

The source already contains explicit `TenantID` ownership in multiple domains, including:

- Agent
- Broadcast
- Scheduled messages/status
- Follow-up flows
- Products/orders
- AI Forms
- Group Guard
- Meta conversion events

Several records also carry both `TenantID` and `AgentID`, which is the preferred pattern for resources that belong to an agent but must also support efficient tenant isolation.

## 3. Agent-owned models that should not be blindly modified

Several legacy models are naturally scoped by `AgentID`, for example:

- ChatHistory
- Contact
- ConversationMemory
- AITurn
- Handoff
- Knowledge
- InboxReadState

For these models, adding `TenantID` everywhere is not automatically required. Tenant ownership can be derived through the Agent relationship.

**Rule:** do not denormalize `TenantID` into agent-owned tables unless required for query performance, isolation enforcement, reporting, or operational reasons.

## 4. Current migration behavior

The application uses GORM `AutoMigrate` during database initialization and already includes:

- `models.User`
- `models.Agent`
- `models.Tenant`
- all major operational domain models

Startup also performs schema preflight and data backfill routines.

### SaaS implication
The first SaaS schema implementation should remain additive and safe for `AutoMigrate`.

Avoid destructive schema changes during the first rollout.

## 5. Current default tenant behavior

The database startup code seeds a default tenant and creates default operational data using `TenantID: 1`.

This is valuable for migration compatibility but must not become a permanent SaaS authorization fallback for ordinary users.

Recommended future behavior:

- Existing production data remains attached to bootstrap/default tenant.
- New registrations create their own tenant.
- Super admin is a platform identity and should not rely on `tenant_id = 1` as its authorization model.
- Cross-tenant operations require explicit platform-level authorization.

## 6. Current feature-gating baseline

The codebase already contains feature-gating helper functions, but they currently return `true` unconditionally.

This is a useful integration point for SaaS enforcement.

Current direction:
- `tenantPlanAllows(tenantID, feature)`
- `agentPlanAllows(agentID, feature)`

These helpers should be replaced by centralized entitlement resolution rather than duplicating plan checks across handlers.

## 7. Recommended SaaS schema evolution

### Extend existing Tenant
Add SaaS lifecycle fields instead of creating a second tenant concept:

- `Slug`
- `Status`
- optional display/billing metadata if needed later

Do not immediately store all subscription state directly in `Tenant`.

### Add TenantMembership
The current `User.TenantID` can remain as a compatibility shortcut during migration, but the target SaaS architecture should introduce membership records:

- tenant_id
- user_id
- role
- status
- timestamps

### Add Plan
Sellable subscription package definition.

### Add PlanFeature
Feature enablement and limits.

### Add Subscription
Subscription lifecycle and billing period state.

### Add SubscriptionEvent
Immutable history.

### Add UsageCounter
Usage-limited features.

### Add AuditLog
Platform and administrative actions.

## 8. Critical migration decision

The first SaaS implementation should use the following order:

1. Preserve `Tenant` table and extend it additively.
2. Preserve `User.TenantID` compatibility.
3. Introduce `TenantMembership` as the target authorization relationship.
4. Add plans and plan features.
5. Add subscriptions.
6. Seed a trial plan.
7. Seed the existing/default tenant as an active bootstrap tenant.
8. Backfill membership for existing users.
9. Replace unconditional feature gates with entitlement services.
10. Only after validation, tighten tenant authorization paths.

## 9. Trial policy mapping

The requested product policy maps cleanly to the existing architecture:

### Trial
- Duration: 30 days
- Maximum agents/senders: 1
- Restricted features controlled by `PlanFeature`
- No destructive deletion when expired

### Paid plans
- Entitlements resolved from active subscription
- Agent limit enforced before creating/linking an agent
- Feature access enforced server-side

## 10. Phase 1.1A checklist

### Existing model review
- [x] Tenant model reviewed
- [x] User tenant linkage reviewed
- [x] Super admin flag reviewed
- [x] Agent tenant ownership reviewed
- [x] Tenant-aware models sampled
- [x] Agent-owned legacy models identified

### Architecture decisions
- [x] Reuse existing Tenant table
- [x] Keep Agent as sender ownership boundary
- [x] Avoid blind TenantID duplication
- [x] Use additive migration strategy
- [x] Reuse feature-gating integration points

### Next implementation phase
- [ ] Design exact GORM models for SaaS entities
- [ ] Extend Tenant model safely
- [ ] Add TenantMembership
- [ ] Add Plan
- [ ] Add PlanFeature
- [ ] Add Subscription
- [ ] Add SubscriptionEvent
- [ ] Add UsageCounter
- [ ] Add AuditLog
- [ ] Register models in AutoMigrate
- [ ] Implement bootstrap/backfill routines
- [ ] Add entitlement service
- [ ] Add trial agent-limit enforcement

## Exit decision
The codebase is ready to proceed to **Phase 1.1B — SaaS Model & Additive Migration Implementation**.
