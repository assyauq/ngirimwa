# Ruangkirim — Services & Runtime

## Objective

Create independent runtime processes for Ruangkirim without disrupting ChatLoop.

## Service Naming

Legacy example:

```text
chatloopService
chatloop.service
```

Target examples:

```text
RuangkirimService
ruangkirim.service
```

The exact service name must follow the audited implementation and systemd conventions.

## Runtime Isolation

- application process
- queue workers
- scheduler/cron
- Redis/cache namespace
- filesystem runtime directory
- logs
- WhatsApp session
- environment file

## Systemd Migration

Before changing anything:

```bash
systemctl cat chatloop.service
systemctl status chatloop.service
```

Create a new Ruangkirim unit rather than renaming/removing the production ChatLoop unit during the migration phase.

Verify:

```bash
systemctl daemon-reload
systemctl enable --now ruangkirim.service
systemctl is-active ruangkirim.service
```

Use the actual service name only after it has been established in the project configuration.

## Queue Workers

Audit queue connection and worker configuration. Ruangkirim workers must not consume ChatLoop production jobs.

## Scheduler

If the source uses a scheduler, configure a separate Ruangkirim schedule. Verify that scheduled tasks target the fresh Ruangkirim database and runtime.

## Logs

Logs should be separately identifiable and should not expose credentials, tokens, message contents unnecessarily, or other sensitive data.

## Health Verification

- [ ] service active
- [ ] process running
- [ ] port/socket correct
- [ ] database reachable
- [ ] cache reachable
- [ ] queue worker active if required
- [ ] scheduler active if required
- [ ] WhatsApp subsystem active if required
