# Findings

## episodeledger (about 900 lines, three tables)

- **Mixed responsibilities.** Every file mixes SQL with decisions: `attempt_identity.go` reads the fence
  and decides stale/wrong/terminal in the same function; `attempt_lifecycle.go` builds SQL text from the
  target status; `recovery.go` mixes the abandon/requeue decision with the statements; `rejection.go`
  mixes reason validation and id derivation with the insert.
- **Clock read in persistence.** `ValidateWorkerIdentity` calls `time.Now()` to assert the owner epoch.
- **Foreign table reads.** `assertRuntimeEpoch` reads `runtime_owner` (control's table). The read cannot
  become a call into control: control imports this ledger. `SupersedeCoalesced` reads `scheduler_items`.
- **No-op error wraps.** Six lifecycle writers return `fmt.Errorf("%w", err)`.
- **Two time encodings** (`formatTime` RFC 3339 and a fixed-width `accepted_at`) with no stated rule.
- **Vocabulary leaks upward.** `episodes/internal/domain`, the executors and `episodes` store import the
  ledger for statuses, `Identity`, `Admission` and rejection reasons. After layering these come from
  the facade as aliases of domain values.
- **Dead and test-only code.** `RecoverUnfinishedAttempts` (the non-cost wrapper) and
  `StartAttempt` (unowned) are used by tests only; production uses the `...WithCost` and `...Owned` forms.
- **Optional safety dependency.** A nil `CostSettler` silently skips cost release during recovery.

## scheduleledger (about 220 lines, one table)

- Five functions over `scheduler_items`; `Coalesce` reads `trigger_evaluations` (cognition's table).
- Used in the same transactions as the episode ledger (`MarkAdmitted` with `Admit`; `Coalesce` with
  `SupersedeCoalesced`) by the same callers (`admission`, `cognition` store, `episodes` store).
- `cognition`'s pure layers import it only for the `Item` struct.
- Two near-identical coalescers (`CoalesceCostRejected`, `CoalesceSkipped`) differ in error text only.
- Too small to carry four layers of its own.

## approvalledger (about 150 lines, one table)

- Six lifecycle writers (`Request`, `Expire`, `ExpireIntent`, `Resolve`, `BindAssertion`, `Withdraw`) and
  `WithdrawSuperseded`, which also publishes an `approval.withdrawn` notification per row.
- Imports `notify`, which sits at layer 4: layering the ledger on top of it would push `policy`'s store
  and `cognition`'s store up and gives a lifecycle writer a transport-level dependency.
- Reads `intents` (policy) in `ExpireIntent` and joins `intents` in the superseded-approval query.
- No-op error wraps, as above.

## Common

- All three keep the `*sql.Tx` in their public signatures; callers are stores of other modules.
- None has a `UBIQUITOUS_LANGUAGE.md` beside the code.
