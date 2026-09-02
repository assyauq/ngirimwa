# Ruangkirim — Phase 2 Frontend Rebuild Plan

## Purpose

This document is the development baseline for rebuilding the Ruangkirim frontend without changing backend contracts or removing existing product capabilities.

## Approved Direction

- Modern SaaS dashboard
- Professional and clean
- Clear hierarchy and generous spacing
- Soft surfaces and subtle borders
- Minimal navigation
- Responsive desktop and mobile
- Controlled Ruangkirim brand color usage

Do not introduce neubrutalism, heavy black borders, hard offset shadows, random colors, or decorative motion unrelated to usability.

## Mandatory Rules

1. Preserve API request/response contracts unless explicitly approved.
2. Preserve authentication, permissions, and business behavior.
3. Audit a module before replacing its UI.
4. Reuse shared UI primitives.
5. Do not remove legacy UI until its replacement is verified.
6. Keep UI, feature logic, and page composition separated.

## Architecture

```text
frontend/src/
├── components/
│   ├── layout/
│   ├── ui/
│   └── shared/
├── features/
├── hooks/
├── services/
├── styles/
└── pages/
```

## Phase Order

1. Design system foundation
2. Application shell
3. Authentication
4. Dashboard
5. Inbox
6. Contacts
7. Messaging operations
8. Automation
9. Products and integrations
10. Team/status/remaining modules
11. Full QA and responsive audit

## Phase 2.1 Definition of Done

- Design tokens are centralized.
- Global and typography styles are available.
- Shared primitives exist for button, input, card, badge, loading, empty, and error states.
- Foundation can be adopted progressively without breaking existing pages.
- Build and typecheck pass before merging.

## AI Agent Workflow

```text
READ → AUDIT → PLAN → IMPLEMENT → VERIFY → COMMIT → REPORT
```

Every implementation report must include modified files, preserved functionality, and build/test results.
