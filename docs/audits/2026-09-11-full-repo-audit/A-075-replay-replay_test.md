# A-075 · `internal/replay/replay_test.go`

LOC: 519 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T5: no copy-paste masking a missing helper — FAIL: the two executors are near-identical duplicates.
- T4: covers behavior, not implementation trivia — FAIL: spec setup mutates golden YAML by exact-string replacement that can silently no-op.
- T3: table-driven with t.Run where natural — pass.
- T2: deterministic, no network/sleeps/races — pass.
- T1: real assertions, no tautologies — pass (effect-boundary counts, digest reproducibility, out-of-catalog rejection are all asserted).

## Findings
- **MED T1. Copy-paste test executors** — `internal/replay/replay_test.go:62-77` vs `:79-94`. `testShadowExecutor` and `testBaselineExecutor` are line-for-line identical except the executed method name and version string (state: `calls`, `manifest`, `snapshots`, `mutateInput`, `summary`). Any new instrumentation must be edited twice. Fix: one struct with an `execute` func field or embedded shared state.
- **MED T2. Golden-spec surgery with silent no-ops** — `internal/replay/replay_test.go:494-519`. `alwaysTriggerSpec` rewrites the golden YAML via exact-match `strings.Replace` for the `when`/`score`/`materialDelta` CEL blocks, debounce/cooldown lines, and `emit:`; only the `"cognition:\n"` marker is checked. If the base spec's wording drifts, every replacement silently no-ops and the "always trigger" premise is lost — shadow/recorded tests then fail with confusing assertions or silently exercise a weaker setup. Fix: assert each replacement actually changed the text (compare before/after or use `strings.Replace` return count and fail on 0).
- **LOW T3. Duplicate test coverage** — `internal/replay/replay_test.go:242-254` (`TestWorkerAwareModesRequireExplicitCapabilities`) asserts exactly the subset already covered by `TestReplayModesHaveNoCredentialOrEffectorBoundary` (`:162-192`) for the same three modes. Delete one.
- **LOW T4. `context.Background()` instead of `t.Context()`** — `internal/replay/replay_test.go` throughout (e.g. `:132-133,168,247,296`), against the repo's go-style guidance to use `t.Context()` in tests.

## Checked, not an issue
- T1: assertions are behavioral and load-bearing: capability-call accounting, zero intents/commands/outbox in shadow mode, comparison digest reproducibility across runs, malformed ledger/out-of-catalog/manifest rejections with error-code checks.
- T2: only `t.TempDir()`/temp files; no network, sleeps, or shared mutable state; parallelism not needed.
- T3: table-driven with `t.Run` in `TestGoldenTracesAreDeterministic` and mode tables.
- Edge coverage: existing DB and WAL sidecar rejection, deterministic-mode cognition isolation (`trigger_evaluations` count), input-mutation attempt on shadow executors.
