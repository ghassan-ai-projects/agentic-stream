# Persistence and migrations

The persistence contract is the durable relational state needed to preserve
identity, ordering, lifecycle, policy, action, and explanation semantics.

## Authority

The ordered SQL files under [`migrations/`](../../migrations/) are the schema
authority executed by `internal/storage`. The design SQL at
[`docs/design/contracts/storage-schema-v1.sql`](../../docs/design/contracts/storage-schema-v1.sql)
is a reviewed baseline and may contain design context that is not a literal
description of the current migration head.

## Durable families

| Family | Why it exists |
| --- | --- |
| Events/checkpoints | Replayable source evidence and deterministic progress |
| Situations/versions | Immutable published state and explanation |
| Scheduler/episodes/attempts | Admission, budgets, cancellation, fencing, recovery |
| Decisions/Intents/policy | Typed proposals and governed disposition |
| Commands/outbox/outcomes | Effect idempotency, leasing, unknown reconciliation |
| Evidence ledger | Scoped read-call identity and crash recovery |
| Owner/epoch/interlock | Single active writer and action readiness |
| Notifications | Cursor-resumable downstream observations |

## Migration rules

- Apply migrations in numeric order through the storage opener.
- Add a migration for a schema meaning change; do not silently edit the meaning
  of a shipped column.
- Add tests for upgrade mapping, idempotence, and recovery behavior.
- Back up before upgrades and rehearse restore in an isolated environment.
- Do not treat a fresh empty database as a successful restore.

## Next reads

- [Durability](../architecture/durability.md)
- [Migration reference](../reference/migrations.md)
- [Recovery](../operations/recovery.md)
