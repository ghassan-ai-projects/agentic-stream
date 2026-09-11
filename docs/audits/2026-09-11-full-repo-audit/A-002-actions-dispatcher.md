# A-002 · `internal/actions/dispatcher.go`

LOC: 892 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- A reclaimed outbox row with an expired lease can be finalized: the expired-lease branch at `dispatcher.go:412-420` invokes `finalizeTx` with a fully populated `Command`, and the lifecycle notifications at `dispatcher.go:604-609` and `dispatcher.go:617-624` pass `notify.AppendLifecycleEventWithTrace`'s non-empty-tenant guard (`internal/notify/lifecycle.go:45`).
- Every field of `leasedCommand` is read somewhere in the repo (`dispatcher.go:154-163`).
- No `database/sql` result error is discarded (`dispatcher.go:463`).

## Findings
- **HIGH F1. Expired-lease reclaim finalizes with an empty Command and wedges the dispatcher** — `internal/actions/dispatcher.go:412-420` (effect at `:604-609`, `:617-624`). The lease query (`:383-402`) scans only `CommandID` into `leased.Command`; `TenantID`/`IntentID` are captured into locals (`storedTenant`, `storedIntentID`) and populated much later (`:441-447`). But when a crashed worker's row is re-selected (`o.status = 'leased' AND o.lease_until <= now`, `:394`), `finalizeTx` is called at `:419` before that population. `notify.AppendLifecycleEventWithTrace` rejects the empty tenant ("lifecycle event identity is incomplete", `internal/notify/lifecycle.go:45`), the whole finalize transaction rolls back, `lease()` returns an error, and because the row stays `leased` with an expired `lease_until` the same row is re-selected first on every subsequent `DispatchOnce` (`ORDER BY o.outbox_id LIMIT 1`, `:395-396`). Result: the action plane is permanently head-of-line blocked, the unknown outcome is never recorded, and the command can never reach `reconciling`. No test covers the reclaim path (`dispatcher_test.go` has none). Fix: set `leased.Command.TenantID = storedTenant` and `leased.Command.IntentID = storedIntentID` before the `finalizeTx` call at `:419`, and add a reclaim test.
- **MED F2. Dead `leasedCommand.Now` field** — `internal/actions/dispatcher.go:162,409`. Written once at `:409`, never read anywhere in the repo (every consumer recomputes `d.clk.Now()`, e.g. `:491`, `:677`). Delete the field and the assignment.
- **LOW F3. `RowsAffected` error swallowed** — `internal/actions/dispatcher.go:463`. `count, _ := result.RowsAffected()` discards the error and treats any outcome as a lost race. Return the wrapped error like the surrounding code does (`:460-462`, `:363-365`).

## Checked, not an issue
- P1: all errors wrapped `%w`; no other swallowed errors; contexts threaded through every query and the provider call (`:183-184`).
- P2: policy revalidation immediately before dispatch is thorough (`:257-367`): lease liveness, command/intent/decision/episode/situation identity + SHA-256 digest binding, R2 approval existence and expiry, intent expiry, policy-digest match, interlock assert, lease refresh; unknown outcomes are recorded `reconciliation=required` and never blindly retried (`:519-527`); transport receipts stay out of `succeeded` (`:528-536`).
- P3: no other dead code or speculative abstraction; `finalizeDispatch`/`revalidateAuthorizationTx` length is justified by the authorization gate.
- P4: action plane isolated; no transport concerns; no per-domain branches (routes are data).
- P5: log/slog not needed (notification + telemetry surfaces used); exported symbols documented; stdlib usage fine.
- P6: `dispatcher_test.go` covers success, unknown-outcome, verification, reconcile, interlock, and lease-bounded verification — but not the reclaim path (see F1).
- P7: serial per-virtual-partition ledger mutations, stable identities, clock injection (`:139-150`).
