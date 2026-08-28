# Ruangkirim — Operations Checklist

## Before Clone

- [ ] ChatLoop is healthy
- [ ] stable commit identified
- [ ] working tree reviewed
- [ ] backups available
- [ ] repository access verified

## After Clone

- [ ] new VPS directory exists
- [ ] `.git` is newly initialized
- [ ] no production `.env` copied
- [ ] no production session copied
- [ ] no production database copied
- [ ] dependencies install cleanly
- [ ] build passes

## Branding

- [ ] logo
- [ ] title
- [ ] favicon
- [ ] login
- [ ] navigation
- [ ] dashboard
- [ ] footer
- [ ] system messages
- [ ] email/notification identity

## Database

- [ ] fresh database
- [ ] migrations pass
- [ ] seeds pass
- [ ] application reads/writes
- [ ] no ChatLoop production data

## Runtime

- [ ] service active
- [ ] worker active
- [ ] scheduler active if required
- [ ] Redis/cache isolated
- [ ] queue isolated
- [ ] storage isolated
- [ ] logs isolated
- [ ] WhatsApp session isolated

## Production

- [ ] Nginx independent
- [ ] domain correct
- [ ] TLS correct
- [ ] health endpoint/application verified
- [ ] login verified
- [ ] core workflow verified
- [ ] WhatsApp verified
- [ ] rollback commit recorded

## Final Audit

```bash
git status --short
git log -1 --oneline
git diff --check
```

Also perform a repository-wide review for unintended legacy branding and leaked secrets.
