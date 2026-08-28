# Ruangkirim — Authentication

## Scope

Authentication functionality should be preserved from the approved source baseline while all Ruangkirim runtime secrets are newly generated/configured.

## Components to Audit

- login
- logout
- session/cookie handling
- token handling if present
- password reset
- email verification
- remember-me behavior
- rate limiting
- CSRF protection where applicable
- OAuth/Google authentication if enabled
- authorization/roles

## Fresh Project Requirements

- [ ] new application encryption/key material where applicable
- [ ] new session secret where applicable
- [ ] no ChatLoop production authentication data
- [ ] no production OAuth secrets committed to Git
- [ ] fresh administrator bootstrap process

## Verification

Test at minimum:

1. Valid login succeeds.
2. Invalid credentials are rejected.
3. Logout invalidates the authenticated state.
4. Protected routes reject unauthenticated requests.
5. Password reset/verification flows work if enabled.
6. Rate limiting works if enabled.
7. Frontend receives and handles authentication errors correctly.

## Security Rule

Do not weaken authentication merely to make the cloning phase easier. Temporary debugging credentials must never become production credentials.
