# Code and boundary audit

| Candidate | Production evidence | Decision |
| --- | --- | --- |
| `Engine`, `NewEngine`, `NewStreamEngine`, `WithRuntimeOwner` | Only runtime composition and replay constructed them; tests used all three | Replace with `Service` and `New(ctx, Config)`; `Cognition` selects the former constructor, ownership is required |
| Exported `Run`, `RunDueTimers` | Tests only | Private to app; tests use `export_test.go` that take the run lock |
| Exported `ConsumerName` | Used only inside the package | Private to store |
| `loadOperatorStateForPartition` and `loadOperatorState` | One-line wrappers over `readOperatorState` | One `LoadOperatorState` with an empty entity for the whole partition |
| Duplicated rollback restore in apply and timer paths | Same reset-and-restore twice | One `restoreAfterRollback` |
| Duplicated per-entity situation-state save in apply and timer paths | Same SQL path twice | One `saveCurrentSituationState` |
| Restore codec beside SQL | Pure integrity rules | Moved to `domain.StoredSituation.Restore` |
| `Engine.lineageID` receiver method | Never used its receiver | `domain.LineageID` |

The engine owns `event_inbox`, `partition_checkpoints`, `operator_state`,
`situations`, `situation_versions`, `lineage_sets` and `timers`, all now pinned to
`internal/engine/internal/store`. It reads no foreign table. Cognition receives
the caller's transaction through a store-defined interface, so app never names a
SQL transaction. Operator-state arming iterates a Go map, so heartbeat timers are
armed in random order; each timer is independent and keyed by a stable hash, so
this is unchanged and harmless.
