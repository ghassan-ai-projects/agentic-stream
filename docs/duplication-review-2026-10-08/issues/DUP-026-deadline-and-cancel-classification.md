# DUP-026: Execution deadline and cancel are classified three ways across executors

- Status: open
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

Not started.
