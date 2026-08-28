# Ruangkirim — Architecture

## Target Architecture

```text
                         Internet
                            │
                         Nginx
                            │
                     Ruangkirim Web/API
                      ┌─────┴─────┐
                      │           │
                  Frontend     Backend
                                  │
             ┌────────────┬──────┼────────────┐
             │            │      │            │
          Database      Redis   Queue      WhatsApp
             │            │      │            │
        fresh DB     RK namespace  workers  RK session
```

The exact components depend on the audited ChatLoop baseline; this document defines the isolation requirements rather than assuming implementation details that have not yet been verified.

## Isolation Model

```text
ChatLoop                         Ruangkirim
────────                         ──────────
/var/www/chatloop               /var/www/ruangkirim
ChatLoop DB                      Ruangkirim DB
chatloop service                 ruangkirim service
chatloop session                 ruangkirim_session
ChatLoop cache/queue             Ruangkirim cache/queue
ChatLoop secrets                 Ruangkirim secrets
```

No production state should be shared accidentally.

## Layers

### Presentation

React/Vite frontend, UI assets, routing, authentication screens, dashboard and API client.

### Application

Backend routes, controllers/services, validation, authentication, business logic and integrations.

### Data

Fresh Ruangkirim database, migrations, indexes and required seed/system data.

### Runtime

Systemd/service processes, queue workers, scheduler, cache, storage, logs and WhatsApp session.

### Infrastructure

Nginx, DNS, TLS, filesystem permissions and process supervision.

## Design Rules

1. Prefer configuration over hard-coded environment-specific values.
2. Never use ChatLoop production credentials in Ruangkirim.
3. Never use the ChatLoop production database as the Ruangkirim database.
4. Do not globally rename arbitrary identifiers without classification.
5. Keep reproducible builds and migrations under version control.
6. Keep secrets and runtime data outside Git.
