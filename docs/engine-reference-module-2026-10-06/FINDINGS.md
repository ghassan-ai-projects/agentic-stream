# Engine migration findings

## Mixed responsibilities

- [`engine_apply.go`](../../internal/engine/engine_apply.go) sequences record application and also owns the checkpoint/inbox SQL and the owner assertion.
- [`engine_state.go`](../../internal/engine/engine_state.go) mixes operator-state SQL (scope predicate, delete, insert) with its JSON/digest codec.
- [`engine_versions.go`](../../internal/engine/engine_versions.go) mixes version-write derivation (occurrence ID, first event time, lineage identity) with three upserts.
- [`engine_restore.go`](../../internal/engine/engine_restore.go) mixes the restore query with the persisted-state integrity rules (codec version, digest, identity, fact-time parsing).
- [`engine_timers.go`](../../internal/engine/engine_timers.go), [`engine_timer_firing.go`](../../internal/engine/engine_timer_firing.go) and [`engine_heartbeat.go`](../../internal/engine/engine_heartbeat.go) mix timer matching and heartbeat due-time rules with timer SQL.
- [`engine_events.go`](../../internal/engine/engine_events.go) mixes batch orchestration with watermark arithmetic and checkpoint reads.

## Optional safety dependency

`WithRuntimeOwner` attaches the ownership fence after construction and is skipped when omitted ("deterministic replay leaves it unset"). A forgotten call silently drops the fence on every stream mutation. `New(Config)` must require the check; replay passes an explicit, named replay-ownership value.

## Public surface

| Symbol | Production use | Decision |
| --- | --- | --- |
| `Engine`, `NewEngine`, `NewStreamEngine`, `WithRuntimeOwner` | runtime composition, replay session | Replace with `Service`, `New(ctx, Config)` (`Cognition` flag selects the old constructor) |
| `RunGlobal` | runtime pipeline, replay | Keep |
| `Run`, `RunDueTimers` | tests only | Private to app; tests use `export_test.go` |
| `ConsumerName` | internal | Private to store/domain |

## Cross-module access

The engine reads and writes only its own tables. `spec.SaveDeployment` writes through spec's API; cognition receives the caller's transaction through an interface the store satisfies structurally. Reads of the event log go through the `eventlog` facade.

## Smaller defects

- The deterministic ID generator is shared by operators, situations and cognition by construction order; this order must be preserved.
- `loadOperatorStateForPartition` and `loadOperatorState` are one-line wrappers over `readOperatorState`.
- `applyRecordTransaction` rebuilds in-memory Situations after a rollback; the same restore is duplicated in the timer path.
- `Checkpoint` busy handling is classified in rules code.
