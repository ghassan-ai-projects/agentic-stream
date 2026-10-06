# Design

## Layers

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade `internal/control` | `RuntimeOwner`, `EpochControl`, `CostLedger` and their one-line delegations; `NewDispatchAuthorization`; sentinel errors; `store.Join` of the caller's `*sql.Tx` | Hold logic, SQL or transactions |
| `internal/app` | Use cases in order: validate → open unit of work → assert → write → audit; kill = record + supersede + release costs in one transaction | Import `database/sql` or `storage` |
| `internal/domain` | Epoch state refusals, lease defaults and validity, cost reservation/settlement/ceiling decisions, state vocabulary | I/O or clock reads |
| `internal/store` | Every statement on the four tables, an opaque `Tx` that is the caller's transaction or one it opens; the `episodeledger` supersede call as plumbing | Decide anything |

## Public API

| Operation | Purpose |
| --- | --- |
| `RuntimeOwner.{Claim, ClaimAndRecover, Renew, Release, Assert}` | Singleton lease |
| `EpochControl.{Drain, Kill, State, AssertDecision, AssertDecisionTx, AssertOrdinaryTx, AssertAdmission}` | Epoch drain/kill and gates |
| `CostLedger.{Reserve, Settle}`, `SetCostLimit`, `ApplyCostCeilings` | Cost control on the caller's transaction |
| `NewDispatchAuthorization` | Final readiness gate |

`RuntimeOwner` and `EpochControl` keep exported configuration fields (`DB`, `Lease`, `Now`,
`InstanceID`) because they are built at about 70 sites; an unconfigured value still fails closed
at the first call. Constructors returning errors are a deferred follow-up.

## Rules every operation follows

- A state change and its audit/side effects commit together (kill: record, supersede, release costs).
- Result sets are read and closed before any write.
- Clock reads are confined to the facade's default; domain and store take times as values.
- Error precedence, sentinels and the SQL predicates (lease validity, ceiling arithmetic) are unchanged.

## Schema or wire changes

None.
