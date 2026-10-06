# Ubiquitous language — approval ledger

The words of the human approval lifecycle. Code, storage and audit use the same names. Every
operation runs on the caller's transaction.

| Term | Meaning | Code name | Storage / wire name |
| --- | --- | --- | --- |
| Approval | A digest-bound human decision request for one intent | — | `approvals` |
| Request | Recording an approval as pending with its sealed document and single-use nonce | `Request` | `status = 'pending'`, `approval_json`, `nonce` |
| Pending | The only state a transition may leave; later transitions on a decided approval are no-ops | `StatusPending` | `pending` |
| Resolve | A principal's decision (approved or denied) with approver and relay identities | `Resolve` | `approver_identity`, `relay_identity` |
| Assertion binding | Attaching the verified assertion digest without touching the nonce | `BindAssertion` | `assertion_sha256` |
| Expire | A pending approval that ran out of time, by approval id or because its intent expired | `Expire`, `ExpireIntent`, `StatusExpired` | `expired`, reason `approval_expired` |
| Withdraw | Denying a pending approval because a newer Situation version superseded its basis | `Withdraw`, `StatusDenied` | `denied`, `withdrawn_at`, reason `approval_withdrawn` |
| Withdrawal reason | Why a withdrawal happened; today only a version conflict | `WithdrawalConflict` | `withdrawal_reason = situation_version_conflict` |
| Withdrawal | A withdrawn approval with what its notification needs, including the trace context | `Withdrawal` | — |
| Withdrawal publisher | The caller's function that publishes one withdrawal's notification in the same transaction | `WithdrawalPublisher` | `approval.withdrawn` event |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `WithdrawSuperseded(…, tenantID, clk)` | `WithdrawSuperseded(…, publish)` | The ledger no longer builds notifications or reads a clock; the caller supplies the publisher |
| `supersededApproval` row type | `Withdrawal` | One named fact, collected before any write |
