# Validation and review

Coverage (short): facade 100%, app 79.4%, domain 95.7%, store 79.2%. The
dispatch behavior suite moved unchanged in assertions into the app layer and
passes against the layered implementation: success, unknown, pending
verification, device verification, verification failure and deadline, interlock
trip and acceptance race, expired lease reclamation, and notification payloads.
New tests cover constructor refusal for each safety dependency, dispatch refusal
without runtime ownership, an effector that cannot enforce authorization, lease
expiry observation, pure admission and authorization refusals, the evidence rule
order, lease compare-and-swap, ledger closure atomicity and rollback.

`make ci-check` (tidy, build, vet, lint, coverage floor, deadcode, vulncheck, docs) passes. Non-short race
tests pass for actions, runtime, cmd, device, soak and the architecture gates.

Fault injection: a storage failure at every write boundary of dispatch and of reconciliation rolls the whole outcome back.

Gates proven by injection: facade logic (`TestActionsFacadeOnlyDelegates`),
`database/sql` in app and a raw `Exec` call (`TestApplicationLayersDoNotTouchInfrastructure`,
`TestActionsApplicationUsesTransactionalPorts`), an exported field on `store.Tx`
(`TestActionsStoreKeepsTransactionsOpaque`), SQL outside the store
(`TestModuleSQLStaysInStore`) and an outbox write from app
(`TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase`).

| Dimension | Rating / 10 | Remaining limitation |
| --- | --- | --- |
| Layering | 9 | Authority-row reads still join foreign tables in the store |
| Domain rules | 9 | Documents stay `map[string]any` behind `Document` to keep digest bytes |
| Fail-closed safety | 9 | None known |
| Ubiquitous language | 9 | Statuses are constants, not a typed state machine |
| Tests | 8 | Store fault injection per write boundary is thin |
| Encapsulation | 9 | `store.Tx` is opaque; owner and interlock are injected ports |
| Type safety | 8 | Effect provider and observed results stay maps |
| Simplicity | 9 | Three internal layers; wire merged into domain |

Future work, in order: owner-provided transactional read ports for the
authorization projection; typed provider and observed-effect results.
