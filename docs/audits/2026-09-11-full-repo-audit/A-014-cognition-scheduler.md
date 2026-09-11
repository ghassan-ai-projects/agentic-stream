# A-014 · `internal/cognition/scheduler.go`

LOC: 479 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The trigger-evaluation event stream never contains an outcome event that contradicts the durable evaluation row.
- Every scheduler function parameter is used; no dead API surface.
- Approval state transitions are owned by one place; the identical withdrawal logic is not re-implemented in a second package.
- Timestamp ordering in SQL matches chronological ordering.

## Findings
- **[MED] F1. Deferred admission still emits a false `admitted` trigger-evaluation event** — `internal/cognition/scheduler.go:76,101-108,155-170`. `Admit` saves the evaluation with `Outcome: "admitted"` (appending a `situation.trigger.evaluated` notification whose ID embeds outcome `admitted`), then the capacity check mutates `eval.Outcome = "deferred"` and re-saves. The durable row ends as `deferred`, but the event stream permanently carries an `admitted` event for an item that was never queued — a false lifecycle signal for any consumer and an explainability defect. Fix: decide the capacity outcome before the first `saveEvaluation`, or emit the notification only after the final outcome is known.
- **[MED] F2. Unused parameter `v situations.Version` in `Admit`** — `internal/cognition/scheduler.go:75`. The parameter is never referenced in the body (verified by read and by grep of all call sites: `internal/cognition/engine.go:124`). Dead API surface every caller must satisfy. Remove it.
- **[MED] F3. Approval withdrawal re-implemented outside the policy plane** — `internal/cognition/scheduler.go:376-412` duplicates `internal/policy/policy_approval.go:81-94` (`withdrawStaleApproval`): same SQL shape, same reasons `situation_version_conflict`/`approval_withdrawn`, same `TypeApprovalWithdrawn` notification — but the policy version also appends `approval.resolved` and resets intent state context; the cognition version does not. Two owners for one policy-plane state transition that have already diverged. Fix: extract a single shared withdrawal routine (policy-owned) or route the cognition withdrawal through it.
- **[LOW] F4. `ORDER BY evaluated_at` on RFC3339Nano text is not chronological** — `internal/cognition/scheduler.go:238-241` (same pattern in `internal/policy/policy_command.go:263-269`). `Format(time.RFC3339Nano)` trims trailing zeros, so `…T00:00:00Z` sorts lexicographically after `…T00:00:00.1Z`; `latestAdmittedTime` can pick the wrong "latest" admitted evaluation within a second and compute a slightly wrong cooldown `not_before`. Fix: store a fixed-width fraction format, or select the row and compare parsed times in Go.

## Checked, not an issue
- P1: all errors wrapped with `%w`; rows closed on all paths; context honored via `ExecContext`/`QueryRowContext`.
- P2: cognition never creates Commands or dispatches effects; capacity/supersession are fail-closed; untrusted event content never becomes executable parameters.
- P4: dependencies point downward (spec, situations, notify, storage-adjacent); no per-domain branches — triggers are spec data.
- P5: `log/slog` not needed here (no logging); exported `Item`/`Admit`/`NewScheduler` documented.
- P6: `scheduler_test.go` covers admission, supersession, capacity deferral; `go test ./internal/cognition/` passes.
- P7: scheduler item identity from `ids.Generator` (deterministic under replay), supersession and dedupe keys deterministic; serial per tx.
