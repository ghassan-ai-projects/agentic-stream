# Design

Episodes takes a three-layer shape dictated by its transaction-threaded
public API: the facade package keeps the tx-scoped use cases (assembler and
runner — exactly like episodeledger's transaction-scoped operations), pure
rules move to domain, and every SQL statement moves to a store layer that
receives the caller's transaction.

## Layers

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade `internal/episodes` | Public API unchanged (`Request`, `Outcome`, `Executor`, `Assembler`, `Runner`, `FakeExecutor`, catalog compile, budget errors); the transaction-scoped use cases that sequence claim → re-bind/freshness → fence → execute → validate → persist → conclude | Assert on raw maps for values it built itself; contain pure rules or SQL that have moved down |
| Domain `internal/episodes/internal/domain` | Pure rules: wall-time budget parsing and validation, snapshot evidence validation and binding, request identity/evidence document building, decision digest and validation-failure rules, failure classification (reason and attempt status), quarantine reason mapping, intent catalog entry construction over projected intents, reconsideration document assembly, rebind document rules | Any I/O, clock reads, `database/sql`, `os`, `net`, storage, spec, decisions (time and spec/decision values arrive as parameters or projections) |
| Store `internal/episodes/internal/store` | Every SQL statement with the caller's transaction: scheduler item, evaluation and snapshot loads, dispatchable episode query and hydration scan, live situation recheck, decision and validated-intent inserts, failed-attempt counts, shadow decision insert; owner-API plumbing (episodeledger, scheduleledger, qualification) | Decide freshness, classify failures, validate snapshots or build documents |

Dependency levels: domain 2 (imports `canonicaljson`, `contractsv1`), store 3
(imports storage, the three ledgers, qualification, domain), facade 5
(unchanged level; imports domain and store plus its current module set). No
edge moves; `allowedImports`, `forbiddenImports` and `durableOwners` gain
entries only.

Spec- and decisions-dependent work stays above domain: `CompileIntentCatalog`
and the assembler's executor document stay in the facade (they consume
`spec.CompiledSpec`); the runner's `decisions.Validate` call stays in the
facade (validation input assembly may use domain helpers). Intent catalog
entry construction receives domain-projected intents, mirroring replay's
baseline projection.

## Public API (unchanged)

Every symbol and signature is preserved, including all `*sql.Tx` parameters,
the `With*` option chain on `Runner` and `Assembler`, and the two budget
error types.

## Rules every operation follows

1. Admission assembles inside the caller's transaction: load scheduler item,
   evaluation and validated snapshot; build the canonical request document;
   validate the budget before persisting; admit through episodeledger and
   mark admitted through scheduleledger, with cost reservation when wired.
2. Dispatch claims the oldest dispatchable episode in one transaction,
   re-checks live freshness (re-bind bounded at 3, else quarantine), refuses
   killed policy epochs, fences a new attempt and binds worker identity.
3. Execution is traced and watched for supersession; failures keep budget
   types; the terminal state and proposed decision persist through a
   detached-context transaction with a five-second persist budget.
4. Decisions validate against the bound request, catalog and epoch; rejected
   decisions persist their validation failure; validated intents insert only
   in shadow mode; scores never touch intents or commands.
5. A stale or refused episode is quarantined inside the claim transaction so
   the queue never blocks; abandonment releases reservations and records
   terminal JSON.

## Enforcement

| Guarantee | Gate |
| --- | --- |
| Downward-only imports | `packageLayers`, `allowedImports`, stale-edge test |
| Domain purity | `TestDomainPackagesArePure` |
| SQL only in store | `TestModuleSQLStaysInStore` |
| Durable ownership unchanged | `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` |
| Effect isolation | existing `forbiddenImports` plus entries for the new layers |
| Behavior preserved | the package's ten test suites, unchanged |

## Schema / wire changes

None.
