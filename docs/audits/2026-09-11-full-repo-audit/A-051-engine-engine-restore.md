# A-051 · `internal/engine/engine_restore.go`

LOC: 180 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Restore reproduces state identical to the pre-restart in-memory engine (P7) and refuses anything it cannot reproduce.
- The `_event_time` fact-time convention is implemented in exactly one place.
- Persisted state integrity is verified (codec, digest, identity) before use.

## Findings
- **[LOW] F1. `_event_time` fact convention implemented independently of its writer** — `internal/engine/engine_restore.go:156-172` vs `internal/situations/situations.go:265-272,380`. The reducer writes `facts[<field>+"_event_time"]` and strips those keys when snapshotting; restore guesses the same suffix to rehydrate `time.Time` values. Two hand-rolled copies of one implicit cross-package contract: if the writer's key scheme changes, restore silently skips rehydration (non-string values are skipped, not rejected) and reducer behavior diverges after restart while digests still verify. Move the rehydrate/strip helpers into `internal/situations` (next to the writer) and call them from restore.

## Checked, not an issue
- P1: errors wrapped `%w`; `rows.Close`/`rows.Err` handled; contexts honored.
- P2: restore refuses legacy/unknown codecs (`requires rebuild`, unsupported codec), incomplete state, digest mismatch, and identity mismatch (situation_id/occurrence/partition/version cross-checked against columns) — fails closed rather than approximating.
- P3: no dead code; `evidenceSet`/`parseSituationTime` are single-purpose and used.
- P4: query joins `situations` with `situation_versions` on `current_version` matching migrations 001/008/009; downward dependencies only.
- P6: `TestEngineRestoresSituationStateAcrossRestart` pins identity stability, version advance, codec refusal, and reducer-state persistence across restart.
- P7: deterministic `ORDER BY partition_id, entity_type, entity_id`; digest verified via `canonicaljson.Digest(DomainSituationState)` against `state_sha256`; restore output feeds the same reducer code paths as live processing.
