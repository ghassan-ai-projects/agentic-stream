# Operations

These pages separate code-supported runtime behavior from deployment
responsibilities and deferred release evidence. Read the [limitations]
before exposing the process to a network or external effector.

[limitations]: ../overview/limitations.md

## Operator paths

- [Runtime](runtime.md) — start, readiness, serving, drain, and kill.
- [Recovery](recovery.md) — owner epochs, restart, quarantine, redrive,
  outbox, and reconciliation.
- [Observability](observability.md) — metrics, traces, SSE, cursors, and
  notification failure behavior.
- [Security hardening](security-hardening.md) — deployment checklist, secrets,
  sockets, proxy, storage, and external effects.

## Status vocabulary

Every procedure is labeled:

- **Implemented:** present in the current code and focused tests.
- **Tested:** implemented with a relevant automated test.
- **Deployment responsibility:** the runtime exposes a boundary, but the
  operator must configure or rehearse it.
- **Deferred:** design or future release work; do not treat it as available.

## Source evidence

The longer working runbook is [`docs/runbooks/runtime-operations.md`](../../docs/runbooks/runtime-operations.md).
It is an engineering archive, so this public set calls out current code
boundaries explicitly.

## Next reads

- [Install](../getting-started/install.md)
- [Current status](../overview/status.md)
- [Release evidence](../governance/release.md)
