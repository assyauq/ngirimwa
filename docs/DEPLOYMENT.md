# Ruangkirim — Deployment Guide

## Deployment Flow

```text
Local/Development
      ↓
GitHub main
      ↓
VPS /var/www/ruangkirim
      ↓
Dependencies
      ↓
Build
      ↓
Database migration (controlled)
      ↓
Restart services
      ↓
Health checks
```

## VPS Directory

```text
/var/www/ruangkirim
```

Keep `/var/www/chatloop` intact during migration.

## Initial VPS Setup

The exact runtime commands must be generated from the audited project stack. Do not assume PHP, Node, Python, Redis, PostgreSQL/MySQL, or systemd configuration until the source is audited.

General sequence:

```bash
cd /var/www
# create/prepare ruangkirim directory
# clone GitHub repository
cd /var/www/ruangkirim
# install dependencies according to project stack
# create environment configuration
# run migrations against fresh Ruangkirim database
# build frontend if required
# configure service(s)
```

## Nginx

Create an independent server block for Ruangkirim. It must point to `/var/www/ruangkirim` and must not overwrite the ChatLoop server block during the validation phase.

## Environment

Production `.env` files are created on the VPS and are not committed.

Required categories include:

- application URL/name
- encryption/session secrets
- database credentials
- cache/queue settings
- external API credentials
- WhatsApp credentials/configuration
- OAuth credentials if enabled

## Deployment Verification

```bash
git status --short
git log -1 --oneline
# build/test commands according to stack
# service status
# application health check
```

## Rollback

Every production deployment must record the deployed Git commit. Roll back to the previous known-good commit if health checks fail, then restart the relevant service(s) and verify application health.

## Production Cutover Checklist

- [ ] Git commit verified
- [ ] environment verified
- [ ] database target verified
- [ ] migrations verified
- [ ] frontend build verified
- [ ] backend health verified
- [ ] services verified
- [ ] queues verified
- [ ] WhatsApp verified
- [ ] Nginx verified
- [ ] TLS verified
- [ ] domain verified
- [ ] logs reviewed
- [ ] rollback point recorded
