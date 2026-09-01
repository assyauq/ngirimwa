# Ruangkirim — Database Isolation

## Principle

Ruangkirim must use a **fresh database**. Do not clone ChatLoop production records into it.

```text
ChatLoop DB → reference schema/logic only
                 ↓
          fresh Ruangkirim DB
```

## Allowed to Reuse

- migration structure
- schema design
- indexes
- constraints
- seed definitions required for a clean installation
- data model/business rules

## Must Not Be Copied

- production users
- passwords or password hashes unless intentionally seeded as test data
- production sessions
- WhatsApp credentials
- production messages
- customer data
- production API tokens
- private uploaded data

## Fresh Install Contract

The project must support:

```text
empty database
    ↓
configure environment
    ↓
run migrations
    ↓
run required seeders
    ↓
create/bootstrap administrator
    ↓
login
```

## Naming

Use an explicitly configured Ruangkirim database name. Do not infer or silently reuse the ChatLoop database name.

## Verification

Before production:

- [ ] database connection points to Ruangkirim DB
- [ ] migrations run successfully from zero
- [ ] required seeders run successfully
- [ ] application can read/write
- [ ] no production ChatLoop records are present
- [ ] backup strategy exists
- [ ] credentials are stored only in environment/secret management
