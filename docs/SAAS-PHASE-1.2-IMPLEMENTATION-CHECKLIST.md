# Ruangkirim SaaS Phase 1.2 — Implementation Checklist

## Objective
Introduce multi-tenant subscription enforcement in the backend while keeping the existing application operational.

## A. Authentication and tenant context
- [ ] Identify canonical user model
- [ ] Identify current super-admin role storage
- [ ] Resolve authenticated user on every protected request
- [ ] Resolve active tenant membership server-side
- [ ] Attach tenant context to request context
- [ ] Reject requests without active tenant membership
- [ ] Keep super-admin platform access separate from tenant access

## B. Subscription resolution
- [ ] Load current subscription by tenant
- [ ] Define current subscription selection rule
- [ ] Support trialing
- [ ] Support active
- [ ] Support expired
- [ ] Support cancelled
- [ ] Support past_due if payment provider requires it
- [ ] Add safe default for tenants without subscription

## C. Feature enforcement
Create centralized checks rather than scattering plan logic inside handlers.

- [ ] `RequireFeature(featureKey)`
- [ ] `RequireLimit(featureKey, requestedDelta)`
- [ ] `GetTenantEntitlements()`
- [ ] `GetTenantUsage()`

Initial enforcement targets:
- [ ] max_agents
- [ ] broadcast_enabled
- [ ] api_enabled
- [ ] team_members limit
- [ ] advanced automation/flow access
- [ ] analytics access

## D. Trial flow
- [ ] New registration creates user
- [ ] New registration creates tenant/workspace
- [ ] Create owner membership
- [ ] Assign trial plan
- [ ] Set 30-day trial end
- [ ] Enforce max 1 sender
- [ ] Prevent duplicate trial abuse using business rules
- [ ] Define expired trial UX/API response

## E. Existing tenant compatibility
- [ ] Existing production tenant bootstrapped
- [ ] Existing owner assigned
- [ ] Existing agents remain visible
- [ ] Existing WhatsApp session files remain untouched
- [ ] Existing API routes remain compatible unless intentionally versioned

## F. Super Admin backend
Required platform-only capabilities:
- [ ] List users
- [ ] List tenants
- [ ] View tenant details
- [ ] Suspend/activate tenant
- [ ] Manage plans
- [ ] Manage plan features
- [ ] View subscriptions
- [ ] Manually grant/change subscription
- [ ] View audit logs
- [ ] Protect every platform endpoint with super-admin middleware

## G. Super Admin frontend
Recommended route namespace:

`/super-admin/*`

Pages:
- [ ] Overview
- [ ] Users
- [ ] Tenants
- [ ] Subscriptions
- [ ] Plans
- [ ] Feature matrix
- [ ] Audit logs

## H. Tenant billing frontend
Recommended route namespace:

`/app/billing/*`

Pages:
- [ ] Current plan
- [ ] Usage and limits
- [ ] Upgrade plan
- [ ] Subscription status
- [ ] Billing history

## I. Test matrix
- [ ] User without token
- [ ] User with token but no tenant
- [ ] Trial tenant
- [ ] Active paid tenant
- [ ] Expired trial
- [ ] Suspended tenant
- [ ] Super admin
- [ ] Tenant admin
- [ ] Tenant member
- [ ] Agent limit reached
- [ ] Restricted feature access
- [ ] Legacy production data

## J. Deployment gates

### Develop
- [ ] Backend tests pass
- [ ] Frontend build passes
- [ ] Migration reviewed
- [ ] Dev deployment succeeds
- [ ] Health endpoint returns OK
- [ ] Manual SaaS smoke test passes

### Production
- [ ] Database backup verified
- [ ] Migration tested on staging copy
- [ ] Production deployment approved
- [ ] Health endpoint verified
- [ ] Existing WhatsApp agents verified
- [ ] Existing login verified
- [ ] Subscription checks verified

## Definition of done
The backend must be able to identify a request's tenant, resolve its active subscription and plan entitlements, enforce sender/feature limits centrally, and support a separate super-admin control plane.
