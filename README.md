# Kirimwa

Kirimwa is a self-hosted WhatsApp business platform combining AI auto-reply, a multi-agent WhatsApp inbox, CRM, broadcasts, scheduling, knowledge base, website crawling, products/orders, REST API, webhooks, and tenant/user management.

## Features

### AI auto-reply
- Automatic AI replies for WhatsApp messages.
- OpenRouter integration for chat, persona, extraction, and embeddings.
- Knowledge base with manual, generated, and imported entries.
- Website crawler with page selection and training.
- Semantic search and configurable models.
- Tone options: friendly, formal, casual, persuasive.
- Human handoff when needed.

### Broadcast / blast
- Bulk text and media messages.
- Scheduled broadcasts.
- Group broadcasts.
- `{nama}` personalization.
- Recipient selection from chats, contacts, groups, and labels.
- WhatsApp number checks.
- Opt-out and consent tracking.
- Randomized delay, humanized typing, pause/retry, and restart recovery safeguards.

### Group management
- Link/number/keyword/flood moderation.
- Delete-message and optional auto-kick actions.
- Moderation audit log.

### CRM & inbox
- Contacts, labels, stages, conversation history, analytics, handoffs, follow-ups, products/orders, and closing records.
- Multi-agent inbox with operational CS permissions.

### Integrations
- REST API at `/api/v1`.
- Per-agent API keys and webhooks.
- Optional Meta Conversions API and Google Sheets integrations.

## Stack

- Go backend
- Gin HTTP server
- React + TypeScript + Vite frontend
- MySQL via GORM
- WhatsMeow for WhatsApp connectivity
- JWT + bcrypt authentication
- OpenRouter-compatible AI integration

## Local development

### Prerequisites

- Go
- Node.js LTS
- MySQL 8.x

### Setup

```bash
npm run setup
npm run setup:env
```

Configure `.env` with your local database and strong application secrets. Never commit `.env`.

### Run

```bash
npm run dev
```

Development servers:

- Frontend Vite: `http://127.0.0.1:5173`
- Backend API: `http://127.0.0.1:3030`

## Project documentation

- `docs/PROJECT-MAP.md` — architecture and development map.
- `docs/INSTALL-LOCAL.md` — local installation guide.
- `docs/LICENSE-AUDIT-REPORT.md` — historical technical audit of the removed proprietary licensing subsystem.

## Security

- Passwords use bcrypt.
- JWT is used for authenticated sessions.
- Login throttling/lockout is enforced server-side.
- Tenant and role permissions are enforced by backend middleware.
- Production CORS should use explicit allowed origins.
- Secrets belong in `.env` or an appropriate secret manager, never in Git.

## License

This project is intended to be distributed under the MIT License. See `LICENSE`.

The project is self-hosted and users are responsible for complying with applicable laws and third-party service terms, including WhatsApp/Meta policies and privacy requirements.
