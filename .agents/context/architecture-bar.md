# Architecture bar: business ownership and one-way flow

Accepted design: [ADR-017](../../docs/design/DECISIONS.md#adr-017-business-ownership-and-directed-module-boundaries).
This extends Q5; Q1–Q7 remain mandatory. Package count is not a quality target.

| ID | Acceptance criterion | Evidence |
| --- | --- | --- |
| A1 | Every production package has a named business or infrastructure responsibility and explicit approved dependencies. Every import points to a lower layer; no unclassified package or same-layer edge. | `TestPackageLayering`, `TestImportsOnlyPointToLowerArchitectureLayers`; module map below. |
| A2 | SQLite infrastructure contains no authority, episode, policy, calibration, or shadow business rules. Contract-only packages have no database, network, or runtime implementation imports. | Foundation/import checks, `TestContractPackagesExcludePersistenceAndTransport`, durable ownership checks. |
| A3 | Governed dispatch depends only on effect ports, never concrete device adapters. Device adapters cannot reach dispatcher, policy, cognition, or episode execution. Composition owns concrete wiring. | Import tests plus authorization/routing regressions. |
| A4 | Durable mutations have declared module ownership. Episode and scheduler lifecycle writes use their owning ledger APIs; control/cognition do not write another module's lifecycle tables. Authority and qualification tables have one owning module. | `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` over production Go SQL literals; handoff column/operation checks; rollback/cancellation regressions. |
| A5 | Forward evidence/decision/command flow and control cancellation are explicit. Results and feedback are returned values or durable records, never adapter callbacks into upstream services. Replay cannot reach live effect adapters, directly or transitively. | `TestReasoningAndReplayCannotReachEffectImplementations`, final-authorization construction checks; documented flow. |
| A6 | Module extraction preserves identities, digest inputs, error precedence, clocks, locks, cancellation, atomic transactions, write fencing, unknown outcomes, and fail-closed behavior. No schema/protocol/dependency changes. | Existing replay, worker, policy, action, recovery and device tests, unchanged golden fixtures; new boundary regressions. |
| A7 | Every implementation round is reviewed, focused-tested, and committed. Final full CI and uncached race suite pass, including coverage of new packages. | Round log and final validation record. |
| A8 | The episode lifecycle depends only on the `Executor` port. Concrete executors (in-process native, out-of-process worker protocol) live under `internal/executor/`; `episodes` imports no worker protocol, gRPC or protobuf package, and classifies failures from transport-neutral errors. | `forbiddenImports`, `TestEpisodeLifecycleImportsNoExecutorTransport` across all episode layers; worker conformance and failure-classification regressions. |
| A9 | Every production package states its business or infrastructure responsibility in its package comment, and the public module map lists every package. | `TestEveryPackageDocumentsItsResponsibility`, `TestModuleMapListsEveryPackage`. |
| A10 | Composition roots (`runtime`, `cmd`) wire modules and drive loops only. They contain no SQL and no business decision; episode admission, intent selection, cost-ceiling configuration and evidence reads are calls into the owning module. | `TestCompositionRootsContainNoSQL`; admission, cost and evidence regressions in the owning modules. |
| A11 | The governed dispatcher (`actions`) contains dispatch only. Internal effect adapters (watches, the simulator) live in their own modules behind `actionport`, so the event pipeline never calls into the action plane. `watch` owns `watch_conditions` and `watch_fires`. | `forbiddenImports`, `durableOwners`, watch and routing regressions. |
| A12 | A module that owns tables and real rules adopts the reference module pattern of `authority` when it is built or refactored: a thin facade (one `Service` built by `New(Config)` with required safety dependencies, delegating only); `internal/<module>/internal/app` holds the use cases and never touches the database; `internal/<module>/internal/domain` holds pure rules (no I/O, no clock reads); `internal/<module>/internal/store` holds transactions and every SQL statement and is the declared owner of the module's tables. Pure-rule modules use only facade and domain when there is no I/O or configuration use case. The checks apply to every module that has these layers. | `TestDomainPackagesArePure`, `TestApplicationLayersDoNotTouchInfrastructure`, `TestModuleSQLStaysInStore`, `TestDecisionFacadeOnlyDelegates`, `TestDecisionCatalogKeepsAuthorityPrivate`, `TestEpisodeFacadeOnlyDelegates`, `TestEpisodeStoreKeepsTransactionsOpaque`, `TestEpisodeApplicationUsesTransactionalPorts`, `TestEvidenceFacadeOnlyDelegates`, `TestEvidenceStoreKeepsInfrastructurePrivate`, `TestEvidenceRulesExcludeProtocolAndCodecs`, `TestEvidenceApplicationUsesOpaquePorts`, `durableOwners`; [module pattern](../../docs/authority-reference-module-2026-10-05/MODULE_PATTERN.md). |

## Module map and flow

`runtime` and `cmd` are composition roots. The deterministic evidence pipeline
remains ingress → event log → stream/situations → cognition → bounded episodes
→ typed decisions → policy → approved-command outbox → dispatcher → effect port.
Import direction differs from event direction: callers depend on lower-level
contracts and services, never on upstream composition.

Additional modules have current concrete responsibilities:

- `runtime`: thin pipeline/readiness/worker facades; private composition assembles
  existing planes; app orders runtime use cases and process lifetimes; domain owns
  pure options/routing/report rules; store joins original transactions through
  table-owning modules; transport owns sources and worker/evidence resources.
  Runtime owns no foreign lifecycle tables. See [runtime guide](../../internal/runtime/README.md).
- `actionport`: typed approved commands, outcomes, final authorization, and effect interfaces.
- `device`: thin effect-boundary facade; app session use cases; pure domain rules
  and typed records; wire schema validation/parsing; gateway transport. Authority
  owns durable admission and device safety facts.
- `episodes`: configured Service facade; app assembles, claims, executes and concludes;
  domain owns pure request/decision rules; store joins the original transaction
  behind an opaque Tx and owns episode SQL. The facade delegates only; app
  cannot call SQL or ledger transactions directly. The deterministic fixture
  executor lives beside native/remote adapters. See [episode guide](../../internal/episodes/README.md).
- `evidence`: configured Service facade; app orders capability issuance/verification,
  bounded query and durable call lifecycle/recovery; domain owns pure authorization,
  deadline, result and live-attempt rules; store owns ledger SQL and opaque original
  transactions; wire owns exact codecs; transport adapts gRPC and eventlog-owned
  evidence reads. Ownership, keys and durable call ports are required configured
  dependencies. See [evidence guide](../../internal/evidence/README.md).
- `decisions`: pure validator facade; private domain binds attempt/snapshot identity,
  catalog authority, risk, parameters, evidence and freshness. Compiled catalog
  entries are private and isolated from source mutation; time is caller-supplied.
  See [decisions guide](../../internal/decisions/README.md).
- `episodeledger`: episode/attempt identities, durable lifecycle transitions and recovery mutations.
- `scheduleledger`: durable queue lifecycle transitions shared by admission and episode assembly.
- `approvalledger`: pending approval, signed assertion and supersession/expiry lifecycle.
- `policy`: thin configured facade; app evaluation and human approval use cases;
  pure domain rules and typed documents; store joins the caller transaction and
  owns policy SQL. Owner, epoch and readiness checks are required constructor
  inputs. Read-only handoff projections retain the original transaction. See
  [policy pattern](../../internal/policy/README.md).
- `control`: singleton runtime ownership and epoch drain/kill; cancellation calls the episode ledger in the same transaction.
- `authority`: target claims, command bindings, device reconciliation and safety evidence.
- `qualification`: calibration activation and shadow decision/comparison evidence.
- `executor/remote`: the out-of-process EpisodeWorker protocol adapter: request mapping, streamed budget accounting and terminal outcome assembly.
- `admission`: turns pending scheduler items into admitted, epoch-stamped episodes, or records why an item can never be admitted.
- `watch`: bounded, expiring derived triggers installed by approved commands and fired by matching evidence.

These are internal Go packages in the same modular monolith. They add no broker,
service, asynchronous queue, storage engine, or model authority. Concrete SQL
repositories receive the existing transaction, so an ownership boundary never
splits an atomic state change. Shared outbox and decision handoffs are declared
producer/consumer contracts, not permission to issue arbitrary SQL across modules.

Control flows downward: composition invokes control; control invokes lifecycle
operations; execution observes durable fencing/cancellation state. The final authorization capability binds a read-only lower-level readiness gate;
effect adapters cannot call back into the dispatcher. Feedback
enters through evidence or durable outcome records and starts another forward
pass. Logical feedback is permitted; reverse service dependencies are not.

## Review questions

- Does the package name explain the business capability it owns?
- Can its consumer use the contract without importing an adapter or composition root?
- Is each durable mutation performed by its owner, with all participants using the original transaction?
- Can replay or model-facing code reach a physical effect through a transitive dependency?
- Did extraction introduce wrappers, duplicate stores, callback cycles, or a second writer?
- Do changed entry points still meet Q7, with mechanisms below domain steps?

## Shared handoff authority

The complete current mutation inventory is pinned by `durableOwners`. Commands,
outbox, intents and Situations have explicitly constrained producer/consumer
phases, described in the public module map. Policy's prepared-command cleanup
binds command + intent + pending status before outbox publication. All other
current mutation tables have one owning package. Adding a new table or mutation
requires an owner and a reviewed phase contract, not another allowlist exception.
