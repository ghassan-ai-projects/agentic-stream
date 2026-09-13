# A-043 · `internal/engine/engine_events.go`

LOC: 213 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- One drive discipline for the state machine; any second loop is production-used or deleted.
- Watermark monotonicity is explicit; checkpoint and inbox commit atomically with applied state.
- No direct `sql.ErrNoRows` comparisons.
- Global scans do not degrade with log length.

## Findings
- **[MED] F1. Two divergent drive loops; the partition-scoped one has no production caller** — `internal/engine/engine_events.go:96-165` (`run`/`runBatch`/`readPartitionRecords`) vs `internal/engine/engine_events.go:14-94` (`runGlobal`/`applyGlobalRecord`). They implement different orderings (timers after all batches vs timers before every record) and different skip disciplines (checkpoint position vs per-record inbox check). Grep-verified: production live and replay both call `RunGlobal` (`internal/runtime/pipeline.go:371`, `internal/replay/replay.go:544`); `Engine.Run`/`RunWithHook` are exercised only by `engine_test.go`. This is one coherent state machine split into two behaviors that can drift; delete `run`/`runBatch` (driving tests through `RunGlobal`) or make `RunGlobal` the only entry point.
- **[LOW] F2. Direct sql.ErrNoRows comparison** — `internal/engine/engine_events.go:186`. `err != sql.ErrNoRows` instead of `errors.Is` (also at `internal/engine/engine_apply.go:75` for the same pattern).
- **[LOW] F3. runGlobal rescans the log from position 0 on every call** — `internal/engine/engine_events.go:15-39`. `lastPosition` starts at zero per call; every record is re-read and re-transactioned (no-op via `eventAlreadyApplied`) on each global run, so cost grows with total log length. Advance from the minimum durable `partition_checkpoints.last_position` (or a persisted global cursor) instead.

## Checked, not an issue
- P1: each record applies inside one `WithTx` (owner assert, inbox check, state, checkpoint, inbox insert); `checkpointWAL` tolerates `SQLITE_BUSY` as documented maintenance; contexts honored; errors wrapped `%w`.
- P2: watermark semantics explicit — `watermarkForRecord` computes `eventTime - maxOutOfOrderness` with a monotonic guard against the stored watermark; per-partition serialization holds in both loops.
- P4: file is the drive/loop layer of one state machine; helpers live in `engine_apply.go`/`engine_timers.go`/`engine_state.go` with downward dependencies only.
- P6: engine tests drive checkpoint idempotence, busy-retry, and restart via these loops (currently the only consumers of the partition loop — see F1).
