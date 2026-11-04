# Migration reference

The runtime applies numbered SQLite migrations from
[`migrations/`](../../migrations/). The current tree contains 43 migrations.

## Migration families

The sequence establishes the initial event/Situation/episode/action records,
then adds trigger deltas, policy audits, lifecycle fencing, reconsiderations,
notifications, evidence ledgers, runtime ownership, quarantine/redrive,
cost/interlock controls, mode/shadow state, epoch control, calibration, and
episode rebinding, paired shadow comparisons, device authority,
reconciliation, and soak evidence, the device-reconciliation column names,
each Situation's latest material version, the removal of unused tables, and
indexes for the per-event hot path, event-time dispositions with per-source
partition clocks, durable state for unopened Situations, set-aside apply
failures, the normalized target of a command binding, and the playbook's
rule-promotion evidence.

The current head is
[`043_event_log_tenant_time.sql`](../../migrations/043_event_log_tenant_time.sql),
an index on the tenant's event time so rule case verification reads the log's
latest event time without scanning the log;
[`042_rule_proposals.sql`](../../migrations/042_rule_proposals.sql) adds
`rule_drafts` and `rule_proposals` (draft calls to the worker and the proposals
a human approves; nothing in them authorizes an action);
[`041_rule_evaluation_cap.sql`](../../migrations/041_rule_evaluation_cap.sql)
records whether a rule's match fell inside its approved cap (`in_cap`);
[`040_case_verifications.sql`](../../migrations/040_case_verifications.sql)
adds `case_verifications`, one record per case whose decision dispatched an
action, the only evidence rule proposals count;
[`039_playbook.sql`](../../migrations/039_playbook.sql) adds `rule_fires`,
`rule_evaluations` and `rule_circuits`, which record what playbook rules did and
authorize nothing;
[`038_command_binding_normalized_target.sql`](../../migrations/038_command_binding_normalized_target.sql)
records the target a command named when the effector bound it to a different
device output, so reconciliation evidence is compared like with like;
[`037_apply_failures.sql`](../../migrations/037_apply_failures.sql)
records events set aside because applying them fails every time;
[`036_unopened_situations.sql`](../../migrations/036_unopened_situations.sql)
keeps the runtime state of Situations that have not opened yet;
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
