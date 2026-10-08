# DUP-026: Execution deadline and cancel are classified three ways across executors

- Status: fixed
- Commit: 60c7b98
- Severity: medium
- Verdict (finders): DIVERGED
- Themes: mechanisms
- Wave: 3
- Finder sources: M6 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Pick one: `timed_out` as an attempt status versus `failed` with a reason. Tests and recorded experiments pin current behaviour, so the outcome must say which behaviour wins and why.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M6: Execution deadline/cancel classified three ways; `timed_out` is a status in two executors and a failure reason in the third

- Verdict: DIVERGED
- Shared meaning: map "the episode execution hit its deadline / was cancelled" to a durable attempt status.
- Sites:
  - internal/episodes/internal/domain/failure.go:44-53 `ExecutionFailureStatus`: `context.Canceled` -> `AttemptCancelled`, `DeadlineExceeded` -> `AttemptTimedOut`, else `AttemptFailed`; :23-41 reason codes (`worker_deadline_exceeded`, `worker_cancelled`).
  - internal/executor/remote/internal/domain/stream.go:192-201 `bindTerminalOutcome`: wire TIMED_OUT -> `AttemptTimedOut`, CANCELLED -> `AttemptCancelled`, FAILED and BUDGET_EXHAUSTED -> `AttemptFailed`; remote/internal/transport/context_error.go:12-22 maps gRPC codes back to `context.Canceled/DeadlineExceeded` so the runner reaches the episodes rule.
  - internal/executor/native/internal/domain/budget.go:56-61 `TerminalForContext`: `DeadlineExceeded` -> `Failed(req, "timed_out")`, i.e. status `AttemptFailed` with reason string "timed_out"; everything else -> `AttemptCancelled` with reason "canceled". Used at native app/loop.go:59,90,103,126 and loop_tools.go:24; loop.go:89 repeats the `Canceled || DeadlineExceeded` test.
  - Related: episodes runner_conclude.go:29 records `AttemptTimedOut` for "decision_after_deadline".
- How they differ: the same deadline is recorded as `timed_out` when the remote worker or the runner sees it and as `failed` when the native executor sees it; an Outcome's Status is recorded verbatim (episodes/internal/domain/lifecycle.go:36-45 `TerminalAttemptStatus`, used at runner_conclude.go:96), so the native deadline lands in the ledger as `failed` and the remote one as `timed_out`; the runner's own error path (`recordExecution`, runner_conclude.go:20) uses `timed_out` too. Check order also differs (episodes tests Canceled first; native tests Deadline first, so an error wrapping both resolves differently).
- Risk if left: replay of the same run through native vs remote executors yields different attempt statuses, retry budgets and explanations; a new executor picks a fourth rule.
- Proposed canonical owner: `internal/episodes` (owns `Outcome`, `Executor` port and `ExecutionFailureStatus`; native and remote executors already import episodes and episodeledger).
- Proposed fix: export one `episodes.OutcomeForContextError(req, err, usage)` (or reuse `ExecutionFailureStatus`) and make `TerminalForContext` build its outcome from that status; keep reason strings per executor if consumers read them. Decide explicitly whether native deadline means `timed_out` (recommended) and update the native tests.
- Behaviour to preserve: reason codes in existing goldens (`timed_out`, `canceled`, `worker_deadline_exceeded`); cost microunits carried on the outcome; attempt-transition rules (episodeledger/internal/domain/status.go:48-61 allow both).
- Verification: native loop tests, remote stream tests (`bindTerminalOutcome`), episodes runner failure tests. New test: native executor deadline yields the same attempt status as a remote TIMED_OUT terminal.
- Smaller sibling in the same package pair: "canonicalise and digest a Decision into a `produced` Outcome" is written in executor/native/internal/app/loop.go:154-163 (`produced`) and executor/fixture/executor.go:108-122 (`producedOutcome`): same `canonicaljson.Marshal` + `Digest(DomainDecision)` + `AttemptProduced` struct. A single `episodes.ProducedOutcome(req, decision)` would remove it (REAL, 2 sites; remote takes worker-supplied digest, stream.go:207-213, so it stays).

## Outcome

### Decision

A deadline is attempt status `timed_out`, never `failed` with a reason. The lifecycle owner already says so: `episodeledger` defines `AttemptTimedOut = "timed_out"` as a durable status (CHECK constraint in migrations 001 and 003, the storage schema contract), `CanTransitionAttempt` allows it from every unfinished state, `CountsAsFailure` counts it against the retry budget with `failed` and `cancelled`, the wire protocol has `TERMINAL_STATUS_TIMED_OUT`, and the runner and the remote executor already recorded it. The design docs (TECHNICAL_DESIGN, DECISIONS) name no competing form; DECISIONS.md (2026-10-03 executor boundary) says transport deadline status becomes a context error so the runner classifies it. Native was the lone outlier. Cancellation is `cancelled`, already the same everywhere.

### Verified

- Native `TerminalForContext` returned status `failed` with reason `timed_out` for a deadline, and status `cancelled` with reason `canceled` otherwise (confirmed, budget.go). It tested Deadline first and treated any non-deadline error as a cancel (confirmed; the episodes rule tests Canceled first).
- Runner `ExecutionFailureStatus` / `ExecutionFailureReason`: Canceled -> `cancelled`/`worker_cancelled`, Deadline -> `timed_out`/`worker_deadline_exceeded` (confirmed). `decision_after_deadline` is `timed_out` (confirmed, same status).
- Remote: wire TIMED_OUT -> `timed_out`, CANCELLED -> `cancelled`, FAILED and BUDGET_EXHAUSTED -> `failed` (confirmed). Its context deadline is returned as an error through `AsContextError`, so the runner records `timed_out` (confirmed by a new test). The wire-enum map is a mapping from a different vocabulary, not a second classification, so it stays.
- Partly right: the finder says replay through native vs remote differs in "retry budgets". Only the status differed. The runner treats an executor-returned Outcome (any terminal status, including `failed`) as a conclusion of the episode, and an executor-returned error as a failed attempt that retries within the budget. That Outcome-versus-error difference is by design of the runner (an Outcome is the executor's verdict) and was not touched.
- The fixture executor ignores its context, so it has no deadline or cancel behaviour to align. Left as is.
- The smaller sibling (produce a sealed Decision into a `produced` Outcome in native `produced` and fixture `producedOutcome`) is confirmed, 2 sites; the remote executor takes a worker-supplied digest and stays.

### Changed

- `internal/episodes/internal/domain/failure.go`: new `(*Request).ContextEndingOutcome(err, cost)`, the single rule that builds an Outcome from a context error with the runner's status and reason (`ExecutionFailureStatus`, `ExecutionFailureReason`); nil when the error is not a context error. Cancel is checked first, as in the runner.
- `internal/episodes/internal/domain/execution.go`: new `(*Request).ProducedOutcome(decision, cost)` (Seal + digest + produced Outcome).
- Both are methods on `Request`, which executors already receive through the `episodes.Request` alias, so the episodes facade (and its `TestEpisodeFacadeOnlyDelegates` gate) gains no function.
- `internal/executor/native/internal/app/loop.go`, `loop_tools.go`: all five deadline/cancel returns use `ContextEndingOutcome`; the repeated `Canceled || DeadlineExceeded` test in `providerFailure` is the nil check of the same call; `produced` uses `ProducedOutcome`.
- `internal/executor/native/internal/domain/budget.go`: `TerminalForContext` deleted.
- `internal/executor/fixture/executor.go`: `producedOutcome` replaced by a four-line `fakeOutcome` over `ProducedOutcome` that only adds the fake reason.
- `internal/testsupport/executorconformance/conformance.go`: new `CheckContextEnding(req, outcome, ending)`, which requires an executor-reported Outcome to equal what the runner records for the same context error.
- `internal/episodes/README.md`: short section recording the rule.
- `architecture_test.go`: removed four now-stale `allowedImports` edges (`executor/fixture` and `executor/native/internal/app` to `canonicaljson` and `episodeledger`). No edge added; `packageLayers` untouched.

### Behaviour changes on purpose

- A native executor deadline is now status `timed_out` (was `failed`), reason `worker_deadline_exceeded` (was `timed_out`). Native cancellation reason is now `worker_cancelled` (was `canceled`); status unchanged. No golden, fixture or stored row pinned the old reason strings (searched `*.json`, `*.golden`, docs); only the two native tests below asserted them.
- A wrapped error holding both Canceled and DeadlineExceeded resolves to cancelled in native (runner order).
- Fixture's seal error text is now `produce fake outcome: seal decision: ...`.

### Tests

- Changed: `TestNativeExecutorEnforcesWallTimeAfterLateProviderResponse` (now asserts the runner-parity outcome instead of `failed`/`timed_out`); `TestTerminalOutcomesClassifyContextErrors` removed with the deleted function, replaced by `TestFailedOutcomeBindsAttemptIdentityAndCost`; `TestStreamBindsTerminalStatusToOutcome` now also covers CANCELLED and derives the TIMED_OUT/CANCELLED expectations from `ContextEndingOutcome`.
- Pin the rule: `TestContextEndingOutcomeIsWhatTheRunnerRecordsForTheSameError`, `TestContextEndingOutcomeExistsExactlyForContextErrors`, `TestProducedOutcomeSealsTheDecisionForTheAttempt` (episodes domain); `TestNativeExecutorEnforcesWallTimeAfterLateProviderResponse` (deadline) and `TestNativeExecutorRecordsCancellationAsTheRunnerDoes` (cancel) via `CheckContextEnding` (native); `TestRemoteExecutorReturnsContextEndingsTheRunnerClassifies` (real gRPC stream, deadline and cancel, flake-checked with -race -count=60) and `TestStreamBindsTerminalStatusToOutcome` (wire terminals) (remote); `TestRunnerRecordsAContextEndingTheSameWhetherReturnedAsOutcomeOrError` (runner: the attempt row is `timed_out` / `cancelled` whether the executor returns the Outcome or the error). Existing `TestExecutionFailureClassificationSurvivesWrapping`, `TestDispatchDecisionAfterDeadlineIsRefused` stay green.

### Deferred

- An executor-returned Outcome concludes the episode even for a retryable `timed_out`/`failed`, while an executor error retries within the budget (see Partly right above). That is a runner policy question, not a classification one; not changed here.
- The fixture executor still ignores its context.
- `go test -count=1 .` fails only on other fixers' in-flight work (storage edges in authority, episodeledger, policy, contractsv1) at the time of writing; no failure names the executor or episodes trees touched here.
