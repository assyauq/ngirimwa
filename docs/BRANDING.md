# Ruangkirim — Branding Migration

## Canonical Identity

User-facing brand:

```text
Ruangkirim
```

Technical lowercase identifier:

```text
ruangkirim
```

## Migration Targets

Audit the source for:

```text
ChatLoop
chatloop
CHATLOOP
Ngertikode
ngertikode
chatloopService
chatloop_session
```

Potential Ruangkirim targets include:

```text
Ruangkirim
ruangkirim
RuangkirimService
ruangkirim_session
```

These are **targets, not blind search-and-replace instructions**.

## Classification Before Replacement

Every match must be classified as one of:

- user-facing text
- application identifier
- configuration key/value
- database object
- service/process name
- session/storage path
- documentation
- test fixture
- third-party dependency/reference

Only appropriate categories are renamed.

## Frontend Branding Checklist

- [ ] document title
- [ ] favicon
- [ ] logos
- [ ] login page
- [ ] navigation
- [ ] dashboard
- [ ] footer
- [ ] empty states
- [ ] error pages
- [ ] loading states
- [ ] notifications
- [ ] manifest/app metadata
- [ ] emails/templates if present

## Backend Branding Checklist

- [ ] application name
- [ ] API response messages
- [ ] mail sender/display name
- [ ] notification sender
- [ ] logs where product identity is appropriate
- [ ] service labels
- [ ] runtime directories
- [ ] documentation

## Verification

Run a repository-wide search after migration and manually review every remaining ChatLoop/Ngertikode occurrence. The goal is zero unintended user-facing legacy branding, not necessarily zero occurrences in third-party code or historical documentation.
