# A-085 · `internal/episodes/p8_freshness_test.go`

LOC: 348 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- F1: no duplicated test helper across files in the package family (`p8BlockingExecutor` vs `blockingExecutor`).

## Findings
- **[LOW] F1. Duplicated blocking-executor helper across internal/external test packages** — `internal/episodes/p8_freshness_test.go:267-275`. `p8BlockingExecutor` re-implements `blockingExecutor` (`internal/episodes/cancellation_test.go:98`, package `episodes`) because this file is package `episodes_test`. The comment acknowledges the copy; nothing keeps the two in sync (behavior, `Name()`, panic-on-double-close semantics). Fix: move the blocking executor into the external test package (`episodes_test`) and have `cancellation_test.go` consume it, or add a small shared exported test helper — one definition, one behavior to maintain.

## Checked, not an issue
- T1: assertions are real and observable (lifecycle rows, attempt status, decision count, byte-compare of persisted `budget` across dispatch); failure messages identify the failing field.
- T2: deterministic — no network; `slowExecutor`'s `time.Sleep(50ms)` simulates workload against a 1ms budget (50x margin, not synchronization); kill test uses bounded channel waits (1s/2s) with failure-diagnosed `t.Fatal`; no goroutine races (`started`/`result` channels establish ordering).
- T3: table logic and subtests independent; helpers take `t` and use `t.Helper()`; DB fixtures isolated per test via `t.TempDir()`.
- T4: covers the P8 gate in both directions (stale refuses, fresh proceeds), deadline immutability, deadline refusal, and hostile-worker kill — high-value behavior, not padding.
- T5: `jsonEqual` defined once; `ticketSchema`/`insertSituationVersion` reused from package fixtures, not redefined.
