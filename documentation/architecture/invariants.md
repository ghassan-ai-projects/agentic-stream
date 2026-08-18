# Product invariants

These are release-blocking properties, not aspirations. A feature that weakens
one of them requires a design decision and new evidence before it can be
accepted.

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. A Situation version is immutable after publication.
4. Deterministic state changes are serial per virtual partition.
5. Every episode is bound to one immutable Situation snapshot and finite budget.
6. A model can read evidence and propose typed Intents; it cannot execute
   effects.
7. Policy revalidates every Intent against current state immediately before
   dispatch.
8. Cross-boundary work uses stable identities, inbox/outbox records, and
   idempotency.
9. Replay never performs external effects unless an explicit, separate
   simulation mode is selected.
10. Every admitted, deferred, coalesced, rejected, canceled, and expired
    cognitive opportunity is explainable from durable records.

## How to use the invariants

When changing a boundary, identify which invariant it touches, name the durable
record or test that proves it, and include the failure path. The focused tests
are organized by package, while the design rationale is in
[`docs/design/TECHNICAL_DESIGN.md`](../../docs/design/TECHNICAL_DESIGN.md).

## Invariant-to-system map

| Invariants | Primary implementation areas |
| --- | --- |
| 1, 6, 7 | `internal/decisions`, `internal/policy`, `internal/actions`, worker/evidence boundary |
| 2, 3, 4 | `internal/eventlog`, `internal/engine`, `internal/operators`, `internal/situations` |
| 5, 10 | `internal/cognition`, `internal/episodes`, `internal/notify`, `internal/storage` |
| 8 | `internal/ids`, `internal/storage`, `internal/evidence`, `internal/actions`, `internal/notify` |
| 9 | `internal/replay`, `internal/runtime`, replay isolation tests |

## Review questions

- Can untrusted content cross the boundary as an executable parameter?
- Can out-of-order or duplicate input change a published version silently?
- Can a stale worker result become accepted after cancellation or restart?
- Can a policy decision become stale between validation and effect acceptance?
- Can a crash cause duplicate or unacknowledged external work without a durable
  reconciliation state?
- Can replay construct or reach a real effector?
- Can an operator explain why an opportunity was admitted or refused?

## Next reads

- [Security model](security-model.md)
- [Replay and shadow design](../design/replay-and-shadow.md)
- [Quality gates](../governance/quality.md)
