# Migration reference

The runtime applies numbered SQLite migrations from
[`migrations/`](../../migrations/). The current tree contains 36 migrations.

## Migration families

The sequence establishes the initial event/Situation/episode/action records,
then adds trigger deltas, policy audits, lifecycle fencing, reconsiderations,
notifications, evidence ledgers, runtime ownership, quarantine/redrive,
cost/interlock controls, mode/shadow state, epoch control, calibration, and
episode rebinding, paired shadow comparisons, device authority,
reconciliation, and soak evidence, the device-reconciliation column names,
each Situation's latest material version, the removal of unused tables, and
indexes for the per-event hot path, event-time dispositions with per-source
partition clocks, and durable state for unopened Situations.

The current head is
[`036_unopened_situations.sql`](../../migrations/036_unopened_situations.sql),
which keeps the runtime state of Situations that have not opened yet;
[`035_event_time_dispositions.sql`](../../migrations/035_event_time_dispositions.sql)
records every late or clock-skewed event's disposition and adds per-source
clocks to partition checkpoints;
[`034_hot_path_indexes.sql`](../../migrations/034_hot_path_indexes.sql)
indexes the queries the runtime issues on every event or batch (the event-log
head and global page read, pending timers, pending intents and watch expiry);
[`033_drop_unused_tables.sql`](../../migrations/033_drop_unused_tables.sql)
drops `calibration_artifacts`, `replay_jobs` and `episode_events`, none of
which anything read or wrote;
[`032_situation_material_version.sql`](../../migrations/032_situation_material_version.sql)
records the latest material Situation version that intent freshness checks
against (ADR-018).

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
- Migration tests: [`internal/storage/internal/store/open_test.go`](../../internal/storage/internal/store/open_test.go), [`legacy_migration_test.go`](../../internal/storage/internal/store/legacy_migration_test.go)
- Persistence contract: [contracts/persistence.md](../contracts/persistence.md)

## Next reads

- [Recovery](../operations/recovery.md)
- [Durability](../architecture/durability.md)
- [Compatibility](../overview/compatibility.md)
