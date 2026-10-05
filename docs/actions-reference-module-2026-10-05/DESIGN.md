# Actions module design

## Layers

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade (`internal/actions`) | `New(Config)`, public domain aliases, and one-line `Service` operations; public unresolved-outcome read delegate needed by authority | Dispatch logic, SQL, transaction management, codecs, mutable after-construction safety setters |
| App (`internal/app`) | Ordered dispatch/reconciliation use cases, clocks, deadlines, effector calls, device verification, and transaction sequencing through ports | SQL, database handles, raw transactions, JSON field access, or wire encoding |
| Domain (`internal/domain`) | Lease liveness, candidate admission, authorization identity/currentness, approval expiry, effect-result classification, reconciliation admission/status decisions, and command/intent/decision/outcome schema and digest checks | I/O, clock reads, database/protocol types, persistence, or external effects |
| Store (`internal/store`) | Opaque SQL transactions, actions-owned reads/writes, read projections, and same-transaction owner/interlock/authority/notification plumbing | Authorization, lifecycle, lease, reconciliation, or status decisions |

```text
runtime composition -> actions facade -> app -> domain / wire / store -> storage and lower owner capabilities
                                      app ---------------------------> actionport.Effector
```

The internal layer levels in the repository graph are domain 2, store 5, app 6,
facade 7. The planned `wire` layer was merged into domain during implementation:
the document codecs are a handful of schema-and-digest checks over
`canonicaljson` and `contractsv1`, perform no I/O, and a separate package of that
size would only add an import hop (domain of `policy` already holds its
documents the same way). Store sits at layer 5 because it joins
authority's layer-4 evidence verification while owning one original transaction.
No concrete `device` or `watch` package is imported by actions.

## Public interface

| API | Purpose |
| --- | --- |
| `New(Config) (*Service, error)` | Validate the database, effector, runtime ownership check and interlock, then compose the private use cases and store. The epoch is passed to the ownership check, which owns its meaning; composition without a runtime owner supplies an explicit always-pass check, as for policy. |
| `(*Service).DispatchOnce(ctx) (bool, error)` | Lease, revalidate, dispatch, independently verify when supported, and finalize at most one command. |
| `CountUnresolvedOutcomes(ctx, tx, commandIDs)` | Count unresolved commands within a caller's existing transaction for the device-authority port. This is a one-line delegate to the store. |

Clock, ID generator, telemetry, and lease duration remain configurable for
deterministic tests and runtime metrics. No public `Dispatcher` or mutation
setters remain. The exported manual reconciliation operation becomes a private
app use case because it is currently called only by the action flow; expose a
separate operator API only when a concrete caller is added.

## Operation ordering and preserved behavior

`DispatchOnce` first asserts runtime ownership and finds the oldest eligible
command. For a reclaimed expired lease it records an unknown result without
calling the effector. It verifies the command document, closes already terminal
outbox rows, or atomically claims the row and changes command state to
`dispatching`. It then revalidates ownership, live lease, command/intent/decision
digests and bindings, approval, current Situation/episode state, policy digest,
and interlock, in the existing order, refreshing the lease last. The effector
call and any independent device-state query occur outside a database
transaction. A deadline or uncertain result remains unknown. Finalization
rechecks the lease under the owner fence, records outcome and verification,
updates command/outbox state, and appends lifecycle events in one transaction.

Evidence rejection order remains final status, evidence presence, source,
evidence type, required device-feedback fields, and first-present digest. A
reconciliation must verify device binding and append the outcome, verification,
and event atomically. `manual_review` remains accepted with current behavior;
transition semantics are a future-work item, not a behavior change in this
migration.

No database schema, JSON schema, wire format, digest input, idempotency key,
notification contract, or dependency change is planned. Keep document bytes and
map field spellings stable where their canonical digest is computed.

## Enforcement

| Concern | Gate or proof |
| --- | --- |
| Thin facade | `TestActionsFacadeOnlyDelegates` |
| Opaque store transaction and private DB/Tx handles | `TestActionsStoreKeepsTransactionsOpaque` |
| App has no SQL or infrastructure imports | Existing generic domain/app/SQL gates plus `TestActionsApplicationUsesTransactionalPorts` |
| Pure domain | Existing `TestDomainPackagesArePure` plus an actions-specific forbidden wire/store import gate if shared imports do not cover `encoding/json` |
| Single writer and command/outbox handoff | `durableOwners`, `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` |
| Forbidden effect-adapter dependencies | `forbiddenImports` and import graph |
| Constructor fails closed | Facade construction tests for each absent or mismatched safety dependency |
| Preserved result behavior | Action lifecycle, interlock race, deadline, expiry, unknown-outcome, evidence-order, and same-transaction rollback tests |
| Boundary enforcement | Inject representative facade logic, app SQL, store alias, and unconfigured safety dependency violations and show each named gate fails |

The repository owner will register package imports/levels and point all four
durable owners at `internal/actions/internal/store` in the matching architecture
round.
