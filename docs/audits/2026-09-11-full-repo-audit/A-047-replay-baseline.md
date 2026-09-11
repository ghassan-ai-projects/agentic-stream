# A-047 · `internal/replay/baseline.go`

LOC: 189 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- No unused exported symbols (P3) — FAIL: `DeterministicBaseline`/`NewDeterministicBaseline` have zero non-test callers.
- No per-domain branches in Go code; domains are data (P4) — FAIL: phase→enum-value maps are hardcoded domain data.
- No duplicated expressions (P3) — FAIL: decision ID built twice.
- Errors wrapped %w, none swallowed (P1) — pass.
- Effect-free boundary, untrusted snapshot only decoded not executed (P2) — pass.
- Canonical JSON for decision/manifest digests (P5) — pass.

## Findings
- **MED F1. Dead exported API** — `internal/replay/baseline.go:22-49`. `NewDeterministicBaseline` and `DeterministicBaseline.ExecuteBaseline` are referenced only by `internal/replay/replay_test.go:461` (grep confirmed; no production caller exists, and the parent `RunMode` shadow machinery in replay.go is itself unwired). 189 LOC of production code exercised only by its own test. Fix: delete with the mode machinery (A-001 F1) or wire the shadow mode that consumes it.
- **MED F2. Domain data hardcoded in Go** — `internal/replay/baseline.go:152-156`. `baselineParameters` branches on `field == "state"` and `field == "mode"` with phase maps `{"over_ceiling":"alert","cooling":"watch"}` and `{"over_ceiling":"bounded_cooling","cooling":"hold"}` — predictive-maintenance domain values baked into Go, violating the repo rule "domains are data" (AGENTS.md forbidden changes; `registry_data.json`/intent-catalog pattern). A second domain would require editing this Go literal. Fix: derive baseline enum selection from spec-declared data (or delete with F1).
- **LOW F3. Duplicated decision-ID expression** — `internal/replay/baseline.go:79` and `:87` both compute `"dec_baseline_" + shortKey(input.EpisodeKey)`. Compute once and pass through.

## Checked, not an issue
- P1: all errors wrapped with context; nil-receiver and empty-catalog guards explicit.
- P2: no storage, credential, worker, or effector access; output validated downstream by `validateShadowOutput`; intent digest computed over a bounded schema-filling of spec-declared values only.
- P5: canonical JSON (`canonicaljson.Marshal`/`Digest`) for decision and manifest digests; deterministic output for identical input.
