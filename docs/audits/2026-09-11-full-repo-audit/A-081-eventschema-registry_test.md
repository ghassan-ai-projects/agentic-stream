# A-081 · `internal/eventschema/registry_test.go`

LOC: 354 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T3/T5: no reimplementation of standard-library helpers that AGENTS.md mandates (cmp/maps/slices).

## Findings
- **[LOW] F1. Local `contains` reimplements `slices.Contains`** — `internal/eventschema/registry_test.go:283-290` (used at 156, 160, 209, 212). AGENTS.md Go standards say to prefer standard library helpers such as `slices`; Go 1.26 is the module target. Delete the helper and call `slices.Contains`.
- **[LOW] F2. Expected event type derived from the lookup key by index arithmetic** — `internal/eventschema/registry_test.go:165`. `definition.EventType != "pump."+tt.ref[len("pump."):len(tt.ref)-len("/1.0")]` recomputes the expectation from the input instead of stating it, unlike the bay table (lines 226-233) which pins explicit expected strings. A systematic quirk shared by the slicing and the data would pass unnoticed, and the arithmetic is fragile trivia. Replace with an explicit `eventType` column in the table like the bay test.

## Checked, not an issue
- T1: assertions check schema content (field types, enums, required lists, units) with messages naming the schema and field; no tautologies.
- T2: deterministic — no I/O beyond the embedded registry, no sleeps, no network.
- T3: table-driven with `t.Run` + `t.Parallel()` genuinely safe (read-only lookups); subtests independent.
- T4: covers enum emission, all 5 thermal provenance fields across 5 refs, 9 pump payloads, dissolved oxygen, 8 bay payloads, and the pinned 52-ref golden digest with per-field structural invariants — the digest pin is mandated by AGENTS.md, not over-coupling.
- T5: fixtures are table rows; no duplicated setup blocks.
