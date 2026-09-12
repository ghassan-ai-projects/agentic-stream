# A-030 · `internal/engine/engine_state.go`

LOC: 288 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Stable identities are unambiguous: distinct evidence sets cannot produce the same lineage identity.
- Entity-scoped operator-state SQL is defined once, not duplicated.
- Persisted counters mean what they say (`state_version` advances).
- Silent zero-row updates cannot age durable state.

## Findings
- **[HIGH] F1. lineageID hashes concatenated evidence IDs without a separator** — `internal/engine/engine_state.go:278-284`. `sha256(id1+id2+...)` maps `["ab","c"]` and `["a","bc"]` to the same `lineage_id` and the same `sha256`, so the second `lineage_sets` insert is silently dropped by `ON CONFLICT DO NOTHING` (`internal/engine/engine_state.go:213-223`) and a situation version whose evidence differs references the OTHER version's `references_json`. Event IDs are caller-supplied free text (`contractsv1.ValidateEnvelope` only requires non-empty), so crafted ingress IDs can corrupt which evidence explains a situation version — an explainability-invariant violation. Fix: join with a delimiter that cannot appear in IDs (the codebase already uses `"\x00"` for composite keys) or length-prefix each ID; add a collision test.
- **[MED] F2. Entity-prefix matching predicate duplicated between read and delete** — `internal/engine/engine_state.go:71-77` and `internal/engine/engine_state.go:123-134`. The `state_key = ? OR (length > length AND substr(...) = ? || char(31))` predicate is copy-pasted; if one copy changes, saves would delete state that reads still load (or vice versa), silently dropping or resurrecting operator state. Extract one predicate builder; also comment that `char(31)` (unit separator) is the composite-key delimiter.
- **[LOW] F3. state_version conflict branch is unreachable; column is permanently 1** — `internal/engine/engine_state.go:142-154`. `saveOperatorState` always deletes the entity scope before re-inserting, so `ON CONFLICT ... state_version = excluded.state_version + 1` never fires and `state_version` stays 1 despite the schema (and name) implying mutation counting. Either increment against the existing row without the delete, or stop pretending the column versions.
- **[LOW] F4. saveSituationRuntimeState ignores RowsAffected** — `internal/engine/engine_state.go:22-31`. The `WHERE ... current_version = ?` guard exists precisely to detect a version race, but a zero-row update is accepted silently, leaving `situations.state_json` older than the in-memory engine until the next versioned save. Check and surface the mismatch.

## Resolution (2026-09-11) — FIXED
- **F1 (HIGH)** fixed: `lineageID` now length-prefixes each event ID (8-byte big-endian) before hashing, so the encoding is injective — distinct evidence sets cannot share a `lineage_id`. Added `lineage_internal_test.go::TestLineageIDNoConcatenationCollision` covering boundary shifts, empty-vs-joined, differing counts, and an embedded delimiter, plus a stability check.
- **F2** fixed: extracted `entityScopePredicate(entityID)` used by both `operatorStateRows` and `deleteOperatorState`; the char(31) delimiter rationale is documented on the helper.
- **F3** fixed: the dead `ON CONFLICT ... state_version = excluded.state_version + 1` branch is removed. Because `saveOperatorState` deletes the entity scope before inserting, the INSERT never conflicts; a conflict now surfaces as an error instead of silently upserting a counter that never advanced.
- **F4** fixed: `saveSituationRuntimeState` now checks `RowsAffected` and returns an error when the `current_version` guard matches zero rows (version divergence), instead of silently leaving `state_json` stale.
- Verified: `go build ./...` and `go test ./internal/engine/` pass.

## Checked, not an issue
- P1: all writes occur inside caller transactions; errors wrapped `%w`; contexts honored; no data races (engine serialized by `Engine.mu`, DB fenced by runtime owner).
- P2: operator state and situation state are per `deployment_id, tenant_id, partition_id` — serial per virtual partition holds; digests verified on restore (`engine_restore.go`).
- P4: all columns match migrations 001/008/009 (`state_codec_version`, `state_json`, `state_sha256`, `tracestate` on `situation_versions`); dependencies point downward (operators, situations, canonicaljson).
- P6: engine tests cover restart restore, digest-pinned state, and version advance; `scanOperatorState`/digest helpers exercised via those paths.
