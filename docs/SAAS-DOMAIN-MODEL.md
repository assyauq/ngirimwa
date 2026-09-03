# Ruangkirim SaaS Domain Model

> **Status:** Proposed Phase 1 contract
> **Branch:** `develop`

## 1. Separation of Responsibilities

### User
Authentication identity.

### Tenant
Customer account/workspace and commercial ownership boundary.

### TenantMember
Relationship between a user and a tenant.

### Agent
WhatsApp sender/application agent owned by exactly one tenant.

### Plan
Commercial product definition.

### PlanFeature
Feature and quota configuration for a plan.

### Subscription
Current or historical commercial access period for a tenant.

### UsageCounter
Measured usage where a quota is required.

## 2. Relationship Model

```text
User
 └── TenantMember ──> Tenant
                        ├── Agents
                        │    └── Agent-scoped application resources
                        └── Subscription
                              └── Plan
                                    └── PlanFeatures
```

Platform administration is separate:

```text
User(role=super_admin)
   └── platform administration access
```

## 3. Tenant Roles

- `owner`
- `admin`
- `member`

Platform role:

- `super_admin`

A customer-side `admin` must not automatically become a platform Super Admin.

## 4. Subscription States

Recommended initial state machine:

```text
trialing -> active
trialing -> expired
active -> past_due
past_due -> grace_period
past_due -> suspended
active -> canceled
canceled -> expired
```

Manual administrative changes must create audit events.

## 5. Entitlement Evaluation

All restricted operations must resolve through a centralized service.

Conceptual API:

```text
CanUseFeature(tenantID, featureKey) -> allowed / denied
CanConsumeQuota(tenantID, quotaKey, amount) -> allowed / limit reached
GetSubscriptionState(tenantID) -> current commercial state
```

Handlers must not duplicate plan logic.

## 6. Sender Limit Definition

Recommended quota key:

`active_senders`

Trial value:

`1`

The implementation should check the limit transactionally when creating or activating an agent/sender, so concurrent requests cannot exceed the plan limit.

## 7. Data Ownership Rule

Every request for an agent-scoped route must satisfy:

```text
requested agent belongs to current tenant
AND
current user is authorized in that tenant
```

This rule applies even when the route uses a valid existing `agent_id`.

## 8. Initial Migration Strategy

Use additive changes first:

1. extend tenant model;
2. create membership table;
3. attach existing users to the default tenant;
4. backfill existing agents to that tenant;
5. add subscription and plan tables;
6. seed Trial plan;
7. add entitlement service in observe/log mode if needed;
8. enable enforcement after data validation.

No WhatsApp runtime database/session file is part of this relational SaaS migration.
