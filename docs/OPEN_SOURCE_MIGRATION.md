# Open Source Migration Notes

## Scope

This branch removes the proprietary licensing/activation subsystem while preserving application business features and runtime responsibilities.

## Removed

- Runtime startup license verification.
- Periodic license heartbeat and license-triggered shutdown.
- Machine fingerprinting and persisted license machine ID.
- License reset CLI path.
- License watermark/owner injection.
- Proprietary EULA and source-license template.
- License-specific environment variables.
- Member/LMS release packaging semantics.

## Preserved

- WhatsApp connectivity and agent lifecycle.
- AI, embeddings, knowledge base, crawler and persona flows.
- Inbox, CRM, broadcast, scheduler and follow-up flows.
- Public REST API and webhooks.
- JWT/bcrypt authentication and tenant/role authorization.
- MySQL/GORM persistence.
- Frontend React/Vite serving and development workflow.
- Application startup workers and cleanup jobs unrelated to licensing.

## Validation checklist

Before merging this branch into `main`:

```bash
gofmt -w backend

go test ./...

npm --prefix frontend ci
npm --prefix frontend run build
```

For production validation, build a fresh backend binary from this branch, deploy to a staging directory or staging VPS, confirm startup without any license environment variables, verify the dashboard login, connect a WhatsApp agent, test inbound/outbound messaging, AI reply, inbox, broadcast, media, scheduling, and public API, then promote the tested build.

Do not deploy directly over the live production binary until the build and smoke tests pass.
