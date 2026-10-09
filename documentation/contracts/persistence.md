# Persistence and migrations

The database stores evidence, state, work, permissions, and outcomes. These
records preserve identities and processing order, support recovery, and
explain why the runtime took each step.

## Authority

The ordered SQL files under [`migrations/`](../../migrations/) are the schema
authority executed by `internal/storage`. The migration head is the only description of the current schema.

## Durable families

| Family | Why it exists |
| --- | --- |
| Events/checkpoints | Replayable source evidence and deterministic progress |
| Situations/versions | Immutable published state and explanation |
| Scheduler/episodes/attempts | Admission, budgets, cancellation, fencing, recovery |
| Decisions/Intents/policy | Typed proposals and policy results |
| Commands/outbox/outcomes | Repeat-request identity, dispatch leases, and reconciliation of uncertain outcomes |
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
