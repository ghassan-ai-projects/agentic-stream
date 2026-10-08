# DUP-025: approval.withdrawn is built in two places with different payloads; notification ids and sources drift

- Status: fixed
- Severity: medium
- Verdict (finders): DIVERGED
- Themes: mechanisms
- Wave: 1
- Finder sources: M4 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `notify` constructors. Confirm whether both withdrawal paths can fire for one approval; if so, fix the payload mismatch that would abort the transaction. The `//agentic-stream/tenants/` vs `tenant/` source drift is known follow-up #8: fix only if the unfinished-work review says it is in scope.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M4: Notification identity built in several places; `approval.withdrawn` built twice with different payloads; source string drifted

- Verdict: DIVERGED
- Shared meaning: a lifecycle notification's event ID (`<type>:<key>`), subject (`<kind>/<id>`), partition key, source and trace are derived from the domain record; the same record must always yield the same event.
- Sites:
  - internal/policy/internal/store/notifications.go:21-33 `approvalWithdrawnEvent`: ID `"approval.withdrawn:"+id`, reason from `event.Reason`, At = `event.Now`, trace from the intent row.
  - internal/cognition/internal/store/tx.go:44-58 `supersededWithdrawalEvent`: ID `"approval.withdrawn:"+w.ApprovalID` (same ID), reason fixed `"situation_version_conflict"`, At = `clk.Now()`, trace from the withdrawal.
  - Other literal-prefix constructors: policy notifications.go:46,74 (`approval.resolved:`, `approval.requested:`, `"approval/"`); actions/internal/store/notifications.go:17-19,34-36,60-62 (`command.dispatched:`, `outcome.recorded:`, `outcome.reconciled:`, `command/`, `outcome/`); cognition store/supersession.go:113-118 (`situation.superseded:`...), reconsideration.go:48-55 (`reconsideration.admitted:`).
  - Source drift: notify/internal/domain/lifecycle_contract.go:27,40 `//agentic-stream/tenant/`+tenant vs cognition/internal/store/evaluations.go:93 `"//agentic-stream/tenants/"+tenantID` (plural), built outside `notify.NewLifecycleEvent` via raw `notify.Append`. (known: #8)
- How they differ: if both withdrawal paths ever fire for one approval, the second `notify.Append` meets the same (tenant, event_id) with a different SHA and `CheckSamePayload` (notify/internal/domain/seal.go:40) turns it into an error that aborts the surrounding transaction. The two paths currently look mutually exclusive by approval status, but nothing pins that. Trigger-evaluated events carry a different source for the same tenant, and the source is part of the stored digest.
- Risk if left: a change to the withdrawn payload (new field, new reason) must be made twice; new modules invent ID/subject prefixes; subscribers keyed on source see two sources per tenant.
- Proposed canonical owner: `internal/notify` (owns the lifecycle contract and already exports `LifecycleEvent`, payload types and `SourceForTenant`). Policy, cognition and actions already import notify.
- Proposed fix: add constructors in notify that fix ID/subject/partition for each payload, e.g. `notify.ApprovalWithdrawnEvent(w Withdrawal, reason string, at time.Time, trace TraceContext) LifecycleEvent`, one per payload type; callers pass domain facts only. Both withdrawal sites call the one constructor. For trigger-evaluated: register it as a contract type (FOLLOW_UPS #8) or at minimum build `Source` from `notify.SourceForTenant` and re-golden.
- Behaviour to preserve: existing event IDs, subjects, partition keys and payload JSON byte-for-byte (notification-goldens-v1.json), `CheckSamePayload` idempotency on replay, trigger-evaluated digests (change deliberately with a golden).
- Verification: docs/contracts/notification-goldens-v1.json tests in notify; policy and cognition notification tests. New test: withdrawal through the policy path then the cognition path for one approval must yield one notification (or an explicit documented conflict).

## Outcome

Commit: pending (reviewer commits)

Verified:
- Two `approval.withdrawn` builders (policy `approvalWithdrawnEvent`, cognition `supersededWithdrawalEvent`) with the same id, subject, partition and payload shape: confirmed. Reason was an argument in policy and the literal `situation_version_conflict` in cognition (the policy caller also passes that literal). `At` differed (event time vs clock read), which is why a second append would conflict.
- Both paths can fire for one approval: wrong. Both only act on `approvals.status = 'pending'` and each moves it to `withdrawn` (`approvalledger.Withdraw`) in the same transaction; policy's `ApprovalDisposition` returns `resolved` for any non-pending approval, so a withdrawn approval is never withdrawn again. No latent abort; the mutual exclusion is now pinned by a test.
- Other literal id/subject/partition spellings (policy requested/resolved, actions dispatched/recorded/reconciled, cognition superseded/reconsideration): confirmed, all drift-prone copies of the same convention.
- Source drift `tenants/` vs `tenant/` for `situation.trigger.evaluated` (known follow-up #8): confirmed (`internal/cognition/internal/store/evaluations.go`). Not in the unfinished-work review scope, and the source is part of stored digests, so it is deferred.

Changed:
- `internal/notify/internal/domain/lifecycle_identity.go` (new): one constructor per payload, `ApprovalRequestedEvent`, `ApprovalWithdrawnEvent`, `ApprovalResolvedEvent`, `CommandDispatchedEvent`, `OutcomeRecordedEvent`, `OutcomeReconciledEvent`, `SituationSupersededEvent` (takes the superseded work item id, which is not in the payload), `ReconsiderationAdmittedEvent`. They derive id, subject and partition key from the payload; callers pass tenant, payload, time and trace.
- `internal/notify/notify.go`: one delegating facade function per constructor.
- `architecture_notify_test.go`: the facade gate now allows `*Event` constructors delegating to domain.
- Callers rewritten to use the constructors, with the same ids, subjects, partitions and payloads: `internal/policy/internal/store/notifications.go`, `internal/cognition/internal/store/tx.go`, `supersession.go`, `reconsideration.go`, `internal/actions/internal/store/notifications.go`.
- `internal/notify/README.md`, `UBIQUITOUS_LANGUAGE.md`: name the constructors.

Decisions: for approval.requested the id and partition now come from the sealed payload (`ApprovalID`, `SituationID`) instead of `request.ID` and the intent row; they are equal in production (`approvalNotificationData` is built from `request.ID` and the same intent row). No allowedImports edge was needed.

Deferred: trigger-evaluated source (follow-up #8) and its contract registration; changing it requires a deliberate digest/golden change.

Pinning tests: `TestLifecycleConstructorsFixEveryEventIdentity`, `TestEveryLifecycleTypeHasAConstructor` (notify domain); `TestApprovalWithdrawnEventUsesTheSharedNotifyIdentity` (policy store); `TestSupersededWithdrawalEventUsesTheSharedNotifyIdentity` (cognition store); `TestWithdrawnApprovalIsNeverWithdrawnAgainByThePolicyPath` (policy domain); `TestNotifyFacadeOnlyDelegates` (root).
