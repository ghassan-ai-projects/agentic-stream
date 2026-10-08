# Migration reference

The runtime applies numbered SQLite migrations from
[`migrations/`](../../migrations/). The current tree contains 31 migrations.

## Migration families

The sequence establishes the initial event/Situation/episode/action records,
then adds trigger deltas, policy audits, lifecycle fencing, reconsiderations,
notifications, evidence ledgers, runtime ownership, quarantine/redrive,
cost/interlock controls, mode/shadow state, epoch control, calibration, and
episode rebinding, paired shadow comparisons, device authority,
reconciliation, and soak evidence, the device-reconciliation column names,
and each Situation's latest material version.

The current head is
[`032_situation_material_version.sql`](../../migrations/032_situation_material_version.sql),
which records the latest material Situation version that intent freshness
checks against (ADR-018);
[`031_device_reconciliation_language.sql`](../../migrations/031_device_reconciliation_language.sql)
aligns `device_reconciliation` columns with the device-authority vocabulary.

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
