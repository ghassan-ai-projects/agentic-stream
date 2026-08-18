# Design

These pages are public summaries of the current Agentic Stream design. The
full technical record remains in [`docs/design/`](../../docs/design/), while
the implementation and tests decide what can be claimed as shipped.

## Reading order

1. [Architecture overview](../architecture/overview.md)
2. [Stream processing](stream-processing.md)
3. [Cognition](cognition.md)
4. [Decisions and actions](decisions-and-actions.md)
5. [Replay and shadow](replay-and-shadow.md)
6. [Observability](observability.md)
7. [Deployment model](deployment-model.md)
8. [ADRs and decisions](../adr/README.md)

## Design pages

- [Stream processing](stream-processing.md) — event time, watermarks, windows,
  operators, corrections, and Situations.
- [Cognition](cognition.md) — deterministic admission, bounded episodes,
  freshness, cancellation, and reconsideration.
- [Decisions and actions](decisions-and-actions.md) — typed proposals, policy,
  approvals, outbox, effects, and reconciliation.
- [Replay and shadow](replay-and-shadow.md) — effect-safe replay modes and
  evaluation boundaries.
- [Observability](observability.md) — traces, metrics, durable notifications,
  and explainability.
- [Deployment model](deployment-model.md) — single-node runtime, worker
  process, and explicit scale-out deferrals.

## Drift rule

The design archive contains historical plans and detailed contracts. A public
summary must label design-only surfaces as planned or deferred. When a summary
and current code disagree, update the summary or record the discrepancy; do not
silently present the design target as the running API.

## Next reads

- [Current status](../overview/status.md)
- [Contracts](../contracts/README.md)
- [Quality bar](../governance/quality.md)
