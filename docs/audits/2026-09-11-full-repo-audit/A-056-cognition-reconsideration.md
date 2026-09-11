# A-056 · `internal/cognition/reconsideration.go`

LOC: 169 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- A row is written once with its complete identity; no follow-up UPDATE exists solely to fill columns whose values were known before the INSERT.

## Findings
- **[LOW] F1. Redundant two-phase write of the reconsideration row** — `internal/cognition/reconsideration.go:119-127,149-151`. `triggerID` and `schedulerItemID` are computed at lines 94-95, before the INSERT, yet the INSERT omits both columns and a follow-up UPDATE (149-151) sets them. Both columns are nullable UNIQUE in `migrations/005_reconsiderations.sql`, so nothing forces the split; it is one extra statement and a window for the two steps to diverge. Fix: include `trigger_id` and `scheduler_item_id` in the INSERT and delete the UPDATE.

## Checked, not an issue
- P1: errors wrapped with `%w`; rows closed via defer; the discarded `DecodeDigest` detail at lines 48-51 is a deliberate uniform fail-closed message (the mismatch error is what consumers key on), not a swallow.
- P2: the correction snapshot is schema-validated AND digest-verified against the persisted `snapshot_sha256` before use (lines 33-51); correction data flows only into evidence/delta JSON, never into executable parameters; dedupe via the `UNIQUE (situation_id, superseded_version, invalidated_command_id)` key makes replay idempotent.
- P3: `saveEvaluation`/`insertItem` reuse is justified (reconsider items intentionally bypass trigger capacity/supersession); no unused symbols.
- P4: cognition schedules reasoning only — no Commands, no effectors; `e.clk.Now()` repetition is safe under the virtual clock.
- P5: canonical JSON for the delta evidence and correction digest; IDs derived by SHA-256 over stable material.
- P6: `reconsideration_test.go` covers admission and idempotency; `go test ./internal/cognition/` passes.
- P7: reconsideration/trigger/scheduler-item IDs are pure functions of (situation, superseded version, command); same inputs, same rows.
