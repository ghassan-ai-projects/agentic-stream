# Approval ledger module

The approval ledger owns the durable human approval lifecycle: request, expiry, resolution, assertion
binding and withdrawal. It stays separate from `policy` (cognition withdraws approvals on supersession and
reasoning layers may not reach policy) and from `episodeledger` (unrelated lifecycle).

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | Operations that join the caller's `*sql.Tx`; `Withdrawal` and `WithdrawalPublisher` |
| App | Lifecycle writes with their stable reasons; `WithdrawSuperseded`: list, then withdraw and publish each approval in order |
| Domain | States, stable reasons and the `Withdrawal` fact |
| Store | The only SQL for `approvals`; reads the intents and decisions that bind an approval to a Situation version |

Every transition starts from `pending`; a repeat on a decided approval changes nothing. A withdrawal
without a publisher is refused (`ErrPublisherRequired`): no approval is withdrawn silently. The ledger
does not import `notify`: the cognition store supplies the publisher, which reads the clock after each
withdrawal and appends the `approval.withdrawn` event in the same transaction, so a failed publication
rolls the withdrawal back. This keeps the ledger at the bottom of the layer table.

Migration record.
