# Durability and recovery

The runtime stores identities, state, and progress so it can recover after a
restart. Those records preserve processing history and help distinguish
completed work from work that still needs attention.

## Storage boundary

The runtime uses SQLite in write-ahead log (WAL) mode through `modernc.org/sqlite`. Database open
applies the numbered migrations under [`migrations/`](../../migrations/). The
current repository contains 34 migrations; migration order is append-only.

Important durable record families include:

- event log rows, connector checkpoints, quarantine rows, and gap records;
- event schemas and deployment/spec digests;
- Situation current state and immutable versions;
- scheduler items and reconsideration records;
- episodes, attempts, fences, decisions, intents, approvals, commands,
  outbox leases, outcomes, and verifications;
- evidence-call ledgers, runtime owner epochs, notifications, and telemetry
  support records.

The exact columns and indexes are in the migration files. The public [persistence
contract](../contracts/persistence.md) explains which families are stable
boundaries and which remain implementation detail.

## Ownership and fencing

One runtime owner holds a durable lease and an epoch. Mutations that require
ownership assert the current epoch. On restart, recovery abandons work owned by
the prior epoch where appropriate and prevents the old owner from renewing or
committing later state.

Episode attempts carry an `attempt_id` and monotonic `fence`. A late result from
an abandoned or superseded attempt is rejected even if it refers to the same
Situation snapshot. The same snapshot does not make output from an obsolete attempt acceptable.

## Crash boundaries

The runtime treats these boundaries differently:

| Boundary | Recovery behavior |
| --- | --- |
| Event append | Duplicate event identity is ignored only when the payload digest matches |
| Stream checkpoint | Processing resumes from durable progress and deterministic state |
| Episode dispatch | Attempt identity and fence reject stale worker output |
| Evidence call | Reservation/completion ledger prevents unsafe duplicate reads |
| Command outbox | Leases let dispatch resume; the idempotency key identifies repeat delivery |
| External timeout | Outcome becomes `unknown` and requires reconciliation rather than blind retry |
| Notification cursor | Subscriber resumes from `Last-Event-ID` or explicit cursor, subject to retention |

## Replay isolation

The default replay creates a fresh database, processes the trace, and computes
the Situation-version history hash. It is constructed without credentials or
effectors. Recorded and shadow modes require explicit
capabilities that cannot perform production effects and keep `EffectsAllowed` false.

## Recovery operator expectations

Operators must back up the SQLite database, protect its permissions, rehearse
restore, and treat a missing or corrupted database as a recovery event rather
than allowing a new empty database to masquerade as continuity. See the
[recovery runbook](../operations/recovery.md).

## Next reads

- [Persistence contract](../contracts/persistence.md)
- [Recovery runbook](../operations/recovery.md)
- [Replay and shadow design](../design/replay-and-shadow.md)
