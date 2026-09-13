# A-033 · `internal/engine/engine_timers.go`

LOC: 286 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Feature application and situation-state persistence are implemented once, not duplicated from the event path.
- Timer firing, situation persistence, and timer acknowledgement commit atomically (exactly-once).
- A tripped invariant error identifies what tripped it.
- Failure restore is not a copy-paste of the event-path restore.

## Findings
- **[MED] F1. saveTimerFeature/saveTimerSituationStates duplicate the event path's applyFeatures/saveAffectedSituationStates** — `internal/engine/engine_timers.go:224-261` vs `internal/engine/engine_apply.go:81-119`. Both implement `ApplyFeature -> saveSituationVersion -> cogEngine.Process` and the "snapshot CurrentState and saveSituationRuntimeState per affected entity" loop. They have already diverged cosmetically (timer version dedups entities via an `updated` map; event version dedups via the `affected` map) and must be edited in lockstep — the exact divergence risk the engine_*.go split is supposed to avoid. Extract shared helpers on `Engine` used by both paths.
- **[MED] F2. Post-rollback restore logic duplicated** — `internal/engine/engine_timers.go:280-286` vs `internal/engine/engine_apply.go:62-67`. `sitEngine.Reset()` + `restoreSituations(...)` + double-error wrapping exists twice in the same state machine. One `restoreAfterWriteFailure(ctx, db, sitEngine, cause)` helper should exist.
- **[LOW] F3. Invariant trip-wire error is not actionable** — `internal/engine/engine_timers.go:163-165`. `due timer has no matching operator state: matched %d of %d` aborts the partition transaction and forces a full in-memory restore, but does not name the timer, operator, or state key that failed to match, and the strict fail-hard response (rather than suppressing the stale timer like `activeTimerIDs` does for boot-inactive ones) would wedge the timer loop if the invariant ever breaks. Include the timer identity in the error and justify fail-hard vs suppress in a comment.

## Checked, not an issue
- P1: load, apply, save, and `acknowledgeTimers` share one `WithTx`; owner asserted inside the transaction; the pre-transaction watermark read is safe (monotonic watermark, acks are the authoritative skip); placeholders generated from timer count with a `//nolint:gosec` justification.
- P2: timer firing is serial per partition under `Engine.mu`; expected-event fencing (`matchesExpectedEvent`) plus boot-activity suppression (`activeTimerIDs`) prevents stale timers from emitting features.
- P3: `ApplyTimer` returns the input state pointer, so discarding `newState` is correct, not lost state.
- P7: timer ack (`status = 'fired'`) is transactional with the effects, so replays and re-runs cannot double-fire (pinned by `TestEngineFiresDurableProcessingTimerExactlyOnce`, including restart and idempotent re-fire).
- P6: timer tests cover fire-once, boot-fenced retirement, and situation completeness effects.
