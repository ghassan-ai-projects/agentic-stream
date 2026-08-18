# Migration reference

The runtime applies numbered SQLite migrations from
[`migrations/`](../../migrations/). The current tree contains 28 migrations.

## Migration families

The sequence establishes the initial event/Situation/episode/action records,
then adds trigger deltas, policy audits, lifecycle fencing, reconsiderations,
notifications, evidence ledgers, runtime ownership, quarantine/redrive,
cost/interlock controls, mode/shadow state, epoch control, calibration, and
episode rebinding.

The filenames are the precise history. Read the SQL and storage tests before
depending on a column or status value.

## Upgrade posture

- Back up the database before upgrades.
- Apply migrations through the runtime storage opener; do not run an arbitrary
  subset.
- Test upgrade and restart behavior on a copy.
- Never edit a shipped migration to change its historical meaning.
- Treat failed or partial migration as a readiness failure, not a reason to
  start with a new empty database.

## Source evidence

- Migration runner: [`migrations/migrations.go`](../../migrations/migrations.go)
- Storage tests: [`internal/storage/storage_test.go`](../../internal/storage/storage_test.go)
- Persistence contract: [contracts/persistence.md](../contracts/persistence.md)

## Next reads

- [Recovery](../operations/recovery.md)
- [Durability](../architecture/durability.md)
- [Compatibility](../overview/compatibility.md)
