# A-088 · `internal/episodes/runner_test.go`

LOC: 311 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- F1: no tautological or mislabeled assertions.

## Findings
- **[LOW] F1. Vacuous `attempts` assertion masks a missing attempt-count check** — `internal/episodes/runner_test.go:298-301,308-309`. `SELECT COUNT(*), MAX(current_fence) FROM episodes` counts episode rows (one by PK on `episode_id`, and the row's existence is already proven by the query at line 292), so `attempts != 1` can never fail; the variable name suggests it verifies the retry produced exactly 2 attempts, which is never asserted anywhere. Fix: count `episode_attempts` for `episode_id = 'epi-retry'` (expect 2: one `failed`, one `produced`) and keep the episode-row query for `current_fence` only.

## Checked, not an issue
- T1: remaining assertions are real and observable (lifecycle transitions, failed-attempt count, fence advance 1→2, decision count, intent type/policy status); failure messages identify actual vs expected.
- T2: deterministic — in-process DB per test, fake executors, no network, no sleeps; `failOnceExecutor` state is safe because `RunOnce` calls are sequential.
- T3: tests are independent with isolated DBs; helpers (`insertSituationVersion`, `ticketSchema`, digests) reused from package fixtures rather than duplicated.
- T4: covers the admitted→concluded happy path end-to-end including decision provenance and validated intent, empty-queue no-op, and retry-with-next-fence — behavior, not line padding.
- T5: no copy-paste blocks; the inline episode fixture INSERT mirrors the seed shape used by `p8_freshness_test.go` (external package cannot share it) and does not drift in any asserted column.
