# A-018 · `internal/episodes/lifecycle.go`

LOC: 401 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Wall-clock reads in fencing/lease checks come from an injectable clock, consistent with the now-parameterized write paths.
- Every exported symbol's export is justified by an external consumer.
- The coupling between `decisions.ValidationError.Reason` strings and the `RejectionReason` registry is pinned by a test, so a new validator reason cannot break decision persistence at runtime.

## Findings
- **LOW F1. `time.Now().UTC()` called directly inside fencing validation** — `internal/episodes/lifecycle.go:224, 329`. `ValidateWorkerIdentity` and `validateTerminalAttemptIdentity` assert the runtime epoch lease against wall clock while every write path (`startAttempt`, `TransitionAttempt`, `RecordRejection`) takes `now` as a parameter. The lease assertion cannot be controlled or made deterministic in tests, and the two read paths behave differently under a virtual clock. Fix: thread `now` through both functions from the caller's clock.
- **LOW F2. Entire exported surface has zero external consumers** — `internal/episodes/lifecycle.go:15-132, 134-196, 198-247, 266-311, 345-384`. Repo-wide grep finds no use of `LifecycleStatus`, `AttemptStatus`, `RejectionReason`, `Identity`, `IdentityError`, `IsIdentityReason`, `CanTransitionAttempt`, `IsTerminalAttempt`, `StartAttempt`, `StartAttemptOwned`, `ValidateWorkerIdentity`, `TransitionAttempt`, or `RecordRejection` outside `internal/episodes` (only in-package callers and tests). For an `internal/` package this is over-exported API surface that reads as a public contract but is not one. Fix: unexport what has no external consumer, or document the package-internal status.
- **LOW F3. Validator-reason → registry coupling is enforced only at runtime** — `internal/episodes/lifecycle.go:346-349` with `internal/episodes/executor.go:462`. `RecordRejection` rejects any reason outside `validRejectionReason`, and the reasons arrive as raw strings from `decisions` (`internal/decisions/validator.go` rejects with 12 literal strings). Today the 12 are all covered and mirror the DB CHECK (`migrations/023_rejection_reasons.sql:12-32`), but no test pins the subset; a new validator reason would make `RecordRejection` fail, rolling back the entire decision-persist transaction. Fix: a unit test asserting every reason literal in `internal/decisions/validator.go` is accepted by `validRejectionReason`, or share the constants.

## Checked, not an issue
- P1: all SQL errors wrapped with `%w`; `sql.ErrNoRows` mapped to typed identity errors; transition guard checks `RowsAffected == 1`.
- P2: fencing is sound — monotonic per-episode fence, terminal-attempt checks, superseded-episode exception limited to cancellation/abandonment and re-validated by `validateTerminalAttemptIdentity` (267-277); decisions can never ride the exception.
- P7: `RecordRejection` is idempotent via content-hash `rejection_id` with `ON CONFLICT DO NOTHING`; unknown-episode rejections still durably recorded with NULL FK.
- P5: exported symbols documented; `RejectionReason` enum mirrors the `episode_rejections` CHECK constraint 1:1 (registry, not speculative constants — `RejectForgedReference`/`RejectEvidenceNotVisible`/`RejectOversized` are DB-schema values).
- P6: lifecycle tests cover fencing, supersession exception, rejection idempotency, and restart recovery.
