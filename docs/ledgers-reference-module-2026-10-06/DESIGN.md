# Design

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade | Aliases of domain vocabulary; one-line delegations that join the caller's `*sql.Tx`; the clock default for owner-epoch assertion | Hold logic, SQL or transactions |
| App | Use cases in order: read state → decide (domain) → write; recovery loop; coalescing | Import `database/sql` or `storage` |
| Domain | Statuses, `Identity`, fence and attempt checks over read values, transition table, rejection reasons and ids, recovery terminal document, admission defaults, queue item | I/O, clock reads |
| Store | Every statement; opaque `Tx` (the caller's transaction); unique-violation classification | Decide anything |

## episodeledger operations (unchanged names)

`Admit`, `StartAttempt(Owned)`, `TransitionAttempt`, `ValidateWorkerIdentity`, `RecordRejection`,
`Rebind`, `BindRequest`, `AbandonRebind`, `Abandon`, `Conclude`, `RetainForRetry`, `SupersedeEpoch`,
`SupersedeCoalesced`, `RecoverUnfinishedAttemptsWithCost`; plus the merged queue operations
`UpsertSchedulerItem`, `MarkSchedulerItemAdmitted`, `CoalesceSchedulerItems`, `CoalesceCostRejectedItem`,
`CoalesceSkippedItem`, `NextPendingSchedulerItem`.

## Rules

- Every operation runs on the caller's transaction; the ledger never begins or commits.
- Fencing and identity errors keep their `IdentityError` reasons and precedence.
- The foreign read of `runtime_owner` stays in the store, named `OwnerHoldsLease`, until control can be
  asked without a cycle (follow-up).
- Time is a parameter in domain and app; only the facade reads the clock.
