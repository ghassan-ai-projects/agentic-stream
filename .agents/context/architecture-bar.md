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

## Module map and flow

`runtime` and `cmd` are composition roots. The deterministic evidence pipeline
remains ingress → event log → stream/situations → cognition → bounded episodes
→ typed decisions → policy → approved-command outbox → dispatcher → effect port.
Import direction differs from event direction: callers depend on lower-level
contracts and services, never on upstream composition.

Additional modules have current concrete responsibilities:

- `actionport`: typed approved commands, outcomes, final authorization, and effect interfaces.
- `device`: capability materialization, device sessions, gateway transport and concrete device effectors.
- `episodeledger`: episode/attempt identities, durable lifecycle transitions and recovery mutations.
- `scheduleledger`: durable queue lifecycle transitions shared by admission and episode assembly.
- `approvalledger`: pending approval, signed assertion and supersession/expiry lifecycle.
- `control`: singleton runtime ownership and epoch drain/kill; cancellation calls the episode ledger in the same transaction.
- `authority`: target claims, command bindings, device reconciliation and safety evidence.
- `qualification`: calibration activation and shadow decision/comparison evidence.

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
