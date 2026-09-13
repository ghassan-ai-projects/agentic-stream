# A-067 · `internal/episodes/recovery.go`

LOC: 124 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported symbol has at least one production caller.
- No `RowsAffected`/iteration error is silently dropped.

## Findings
- **MED F1. `RecoverUnfinishedAttempts` is an exported wrapper with zero production callers** — `internal/episodes/recovery.go:22-29`. Repo-wide grep: production recovery goes through `RecoverUnfinishedAttemptsWithCost` (`internal/runtime/recovery.go:54`); the no-cost variant is called only by `internal/episodes/lifecycle_test.go:148,169`. A test-only convenience exported as production API — speculative surface that must be maintained and documented as if live. Fix: unexport it, or delete it and have the test call the `WithCost` variant with nil.
- **LOW F2. `RowsAffected` error swallowed in the cancelling branch** — `internal/episodes/recovery.go:105`. `if count, err := result.RowsAffected(); err == nil && count == 1` silently skips the `AbandonedEpisodes` counter on error (the attempt was already abandoned at 77-93, so the report can under-count while the mutation succeeded). P1 says no swallowed errors. Fix: handle the error and return it, or log via slog.

## Checked, not an issue
- P1: query/scan/iterate/close and every UPDATE error wrapped with `%w` and returned; rows closed exactly once effectively (deferred close is idempotent).
- P2: recovery is scoped to attempts owned by an older or missing epoch (`owner_epoch IS NULL OR <> current`), preserving fence monotonicity for the next `StartAttempt`; cancelling attempts abandon their episode while others re-queue, matching the documented restart contract.
- P7: requeued episodes retain their cost reservation; only permanently abandoned episodes settle at zero.
- P6: recovery covered by lifecycle restart tests (same-epoch and epoch-change cases).
