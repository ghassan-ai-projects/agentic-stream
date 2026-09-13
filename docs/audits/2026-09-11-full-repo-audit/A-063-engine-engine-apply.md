# A-063 · `internal/engine/engine_apply.go`

LOC: 147 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Idempotency: an event applies at most once, enforced transactionally with the state it produces.
- Retry is safe: only transaction-safe callbacks are retried, and rollback restores in-memory state.
- No direct `sql.ErrNoRows` comparisons; no logic duplicated from the timer path (tracked in A-033).

## Findings
- **[LOW] F1. Direct sql.ErrNoRows comparison** — `internal/engine/engine_apply.go:75`. `err != sql.ErrNoRows` instead of `errors.Is`; same deviation also in `internal/engine/engine_events.go:186` and `internal/eventlog/quarantine.go:47`.
- **[LOW] F2. saveAffectedSituationStates re-parses composite keys that applyFeatures just built** — `internal/engine/engine_apply.go:81-119`. `applyFeatures` packs `EntityType + "\x00" + EntityID` into a map and `saveAffectedSituationStates` immediately splits it back with `strings.SplitN`; passing structured keys (or deduplicating inside `applyFeatures`) would remove the encode/decode round trip through a magic separator. Duplication with the timer path's equivalent loop is filed as A-033 F1.

## Checked, not an issue
- P1: `RetrySQLiteBusy` wraps a transaction-safe callback (`WithTx` rolls back before returning, satisfying its contract); state, checkpoint, and inbox commit in one transaction; post-rollback `sitEngine.Reset()` + restore keeps memory equal to disk; errors wrapped `%w`; contexts honored.
- P2: `assertOwner` fences every write transaction to the live lease; `eventAlreadyApplied` inbox check plus unconditional inbox insert (PK-backed) gives at-most-once per consumer; operator state is loaded fresh from the DB per event, so no stale in-memory operator state exists to invalidate.
- P3: no dead code in this file; every helper has a caller.
- P4: dependency direction downward (eventlog, operators, storage); thin orchestration over `sitEngine`/`cogEngine` matches the documented flow.
- P7: commit writes `partition_checkpoints` and `event_inbox` atomically with state, so replay and live produce identical state for the same log (pinned by engine tests: checkpoint idempotence, restart restore, busy-retry apply).
