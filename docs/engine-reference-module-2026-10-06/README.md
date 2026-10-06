# Engine reference-module migration

`internal/engine` is the deterministic stream engine: it reads the event log per partition, drives operators and timers, folds features into Situations, persists versioned Situations, and hands each new version to cognition in the same transaction. One `Engine` type currently mixes the orchestration, every SQL statement for six tables, the restore codec for persisted Situation state, timer matching rules and watermark arithmetic. This migration gives it the reference layers while keeping the deterministic ordering, clock reads and transaction boundaries byte-for-byte.

```text
internal/engine (configured facade, layer 10)
  └── internal/app (run, apply, timer and restore use cases, layer 9)
        ├── internal/domain (watermarks, situation state and version rules, timer and heartbeat rules, layer 5)
        └── internal/store (opaque transactions and engine SQL, layer 6)
```

Merge decision: engine stays its own module; it is the owner of `event_inbox`, `partition_checkpoints`, `operator_state`, `situations`, `situation_versions`, `lineage_sets` and `timers`, and the reasoning layers must not reach effect code. There is no `wire` package: the persisted-state decode is a few pure checks and belongs in domain.
