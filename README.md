# Ruangkirim

Platform messaging and customer communication project derived from the stable ChatLoop application baseline, rebuilt as an independent product and runtime.

> **Status:** Project initialization / cloning phase

## Project Principles

- Ruangkirim has its own Git repository, VPS directory, environment, secrets, database, runtime state, services, queues, cache namespace, and WhatsApp session state.
- ChatLoop remains a reference/backup project during migration and is never modified as the Ruangkirim production runtime.
- Source code may be cloned from the approved ChatLoop baseline, but `.git`, production secrets, runtime data, sessions, logs, caches, and production database data must not be copied.
- Branding migration must distinguish user-facing identity from third-party dependencies and technical identifiers that must remain unchanged.

## Planned Runtime

```text
GitHub:       mrifatsyauqi/ruangkirim
VPS:          /var/www/ruangkirim
Service:      ruangkirim.service / RuangkirimService
Session:      ruangkirim_session
Database:     fresh Ruangkirim database
```

## Documentation

- [Cloning & Migration](docs/CLONING.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Branding](docs/BRANDING.md)
- [Database](docs/DATABASE.md)
- [Authentication](docs/AUTHENTICATION.md)
- [WhatsApp](docs/WHATSAPP.md)
- [Services & Runtime](docs/SERVICES.md)
- [Deployment](docs/DEPLOYMENT.md)
- [Security](docs/SECURITY.md)

## Development Workflow

```text
Audit → Change → Build/Test → Diff Review → Commit → Push
```

Never use a global blind replacement for branding, services, database names, or session identifiers.
