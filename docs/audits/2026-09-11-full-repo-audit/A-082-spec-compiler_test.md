# A-082 · `internal/spec/compiler_test.go`

LOC: 323 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T1: negative-path assertions verify the diagnostic, not merely that *some* error occurred.

## Findings
- **[MED] F1. Range-validation failures asserted as bare `err != nil`** — `internal/spec/compiler_test.go:144-149`. In `TestCompileWatchConfidenceFloorSchemaValidation`, the "negative" and "over one" cases pass on any compile error. The test's YAML mutation is built by string-splicing `watch_confidence_floor: <value>` under `actions:`; if that splice produces malformed YAML (indent drift) or the field is rejected for a schema reason unrelated to the range check, the test still passes while the actual [-0.1, 1.1] range validation is broken. Assert the error mentions `watch_confidence_floor` (or the specific range diagnostic), mirroring the targeted substring checks the rest of this file already uses (e.g., lines 163, 191, 204).

## Checked, not an issue
- T1: positive paths assert compiled values (`WatchConfidenceFloor == 0.7`, input/intent counts, canonical JSON non-empty); all other error paths pin the diagnostic substring (`not_declared`, "does not match", "unknown operator output", "cel:", "duplicate key", "duplicate window name").
- T2: deterministic — embedded schema + local YAML strings; the three example-file compiles are repo fixtures, not network or time-dependent.
- T3: table-driven with `t.Run` (`TestEffectiveWatchConfidenceFloor`, `TestCompileWatchConfidenceFloorSchemaValidation`); subtests independent (fresh `CompileBytes` each).
- T4: covers digest stability under equivalent YAML, digest sensitivity to prompt content, unit mismatch, undeclared payload fields, latest aggregate, invalid CEL, duplicate YAML keys, and duplicate window names — real edge cases for a compiler, no padding.
- T5: single `minimalSpecYAML()` fixture mutated per test; no duplicated YAML blocks.
