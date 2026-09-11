# A-070 · `internal/cognition/engine_test.go`

LOC: 1316 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every test builds its fixture through one shared helper/table instead of 14 copy-pasted setup blocks.
- `buildFeatures` `set_union` strategy (engine.go:288-293) has at least one assertion in the cognition package.
- The score == threshold boundary (engine.go:262 admits on equality) is pinned by a test.
- The capacity fixture does not hardcode the unexported `globalCapacity` value.

## Findings
- **[MED] F1. Fourteen near-identical ~55-line setup blocks** — `internal/cognition/engine_test.go:71-113,145-191,223-267,302-347,379-424,477-520,576-623,663-707,758-801,922-965,1001-1044,1075-1118,1153-1196,1227-1302`. Each test repeats storage.Open + the full `spec.CompiledSpec` literal + `SaveDeployment` + `NewEngine` + a `situations.Version` literal + the `WithTx(insert, Process)` block; only the trigger fields (`Debounce`/`Cooldown`/`MaterialDelta`/`Threshold`) and facts differ. ~700 of 1316 LOC is boilerplate; a new trigger field means 14 edits. Extract `newCognitionFixture(t, trigger spec.Trigger, facts map[string]any) (*storage.DB, *Engine)` and drive the uniform scenarios from one table.
- **[MED] F2. `set_union` reducer strategy untested** — `internal/cognition/engine_test.go` (whole file) vs `internal/cognition/engine.go:288-293`. Every test uses `latest_event_time`; the evidence-based `set_union` branch (including its nil-evidence default) has zero coverage in the package. Add one test admitting a trigger on `features.<evidence_field>`.
- **[LOW] F3. Capacity fixture silently couples to unexported constant** — `internal/cognition/engine_test.go:609` vs `internal/cognition/scheduler.go:59`. `fillPendingSchedulerItems(..., 100, ...)` hardcodes 100 to match `globalCapacity = 100`; raising the constant breaks the test with no hint why. Seed `globalCapacity - 1` via an exported test hook or assert the observed capacity boundary relative to the constant.
- **[LOW] F4. Score==threshold boundary unpinned** — engine.go:262 uses `score < tr.Threshold`, so equality admits; tests cover only strictly-below (`:1018`) and strictly-above (`:239`). One equality case locks the comparison direction against a future `<=` regression.
- **[LOW] F5. Not table-driven; `context.Background()` throughout** — the 14 tests vary one dimension (trigger config) yet are separate top-level functions; all use `context.Background()` where repo style prefers `t.Context()`. Subsumed by fixing F1.

## Checked, not an issue
- T1: assertions read durable outcomes (`trigger_evaluations.outcome`, `scheduler_items.not_before`/`status` counts) with got-values in every fatal message; no tautologies found.
- T2: fully deterministic — virtual clock for debounce/cooldown/combined tests, no sleeps, no network, no goroutines.
- T3: subtests independent (each uses its own `t.TempDir()` DB).
- T4: core decision paths (ignore/nil-feature/admit/debounce/cooldown/coalesce/capacity-defer/material-delta/delta-math/policy-digest/upsert) are behaviorally covered.
- T5: `insertSituationVersion` is shared by all tests in the file; no intra-file copy-paste beyond F1's setup blocks.
