# Ruangkirim — WhatsApp Subsystem

## Objective

The WhatsApp runtime must be independent from ChatLoop. In particular, session state must never be accidentally reused.

## Naming Target

Legacy concept:

```text
chatloop_session
```

Ruangkirim target:

```text
ruangkirim_session
```

Before changing it, determine whether the identifier is a database object, filesystem path, Redis key, application variable, or service configuration.

## Isolation Requirements

- [ ] new session directory/state
- [ ] new credentials where required
- [ ] new environment configuration
- [ ] independent service process
- [ ] independent logs
- [ ] independent queue/cache namespace
- [ ] no production ChatLoop session copied

## Functional Tests

- [ ] service starts
- [ ] WhatsApp session initializes
- [ ] authentication/pairing flow works
- [ ] connection state is reported correctly
- [ ] outbound message works
- [ ] inbound message works if supported
- [ ] reconnect behavior works
- [ ] session survives expected restart behavior
- [ ] logout/reset behavior works

## Operational Rule

Never point Ruangkirim at the ChatLoop session directory as a shortcut. If migration of a WhatsApp account is intentionally required later, it must be handled as a separate controlled migration with explicit backup and rollback procedures.

## Future Architecture Changes

The WhatsApp implementation may evolve independently after cloning. Such changes should be documented as separate architectural changes rather than mixed into the initial branding clone.
