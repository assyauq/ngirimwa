# Ruangkirim — Security & Secrets

## Core Rule

Ruangkirim is a fresh runtime. No ChatLoop production secret or private runtime state should be copied into the new project.

## Never Commit

- `.env`
- production credentials
- API keys
- OAuth client secrets
- private keys
- database passwords
- WhatsApp session credentials
- access tokens
- signing/encryption secrets
- production database dumps
- private customer/message data

## Fresh Secrets

Generate/configure new values where the application requires them:

- application encryption key
- session secret
- JWT/signing secret
- webhook secret
- OAuth credentials
- external API credentials
- WhatsApp credentials

## Environment Policy

Commit only safe templates such as `.env.example` with placeholder values. Never put real production secrets in documentation, shell history committed to scripts, or source files.

## Runtime Isolation

A security boundary is not achieved by branding alone. Verify that Ruangkirim cannot accidentally access or mutate:

- ChatLoop production database
- ChatLoop session directory
- ChatLoop queue
- ChatLoop cache namespace
- ChatLoop private uploads
- ChatLoop production credentials

## Verification

Search for accidental secrets before pushing:

```bash
git status --short
git diff --cached
```

Review suspicious files manually. Use repository secret scanning where available.

## Incident Rule

If a ChatLoop secret is accidentally copied into Ruangkirim, remove it from the working tree **and rotate the credential**. Removing it from the latest commit alone is not sufficient if the secret has already been exposed to Git history.

## Least Privilege

Use separate database users/credentials and service permissions where practical. Ruangkirim services should have only the filesystem, network, and database access they require.
