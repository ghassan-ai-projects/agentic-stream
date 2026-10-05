# Actions module

Actions delivers policy-approved commands to effectors and records what it
knows about each attempt. It leases one command, re-proves its authority,
calls the effector outside any transaction, verifies device state when the
effector can, and records outcome, command, outbox, verification and lifecycle
notification atomically. It never opens an effect route itself: effects cross
only `internal/actionport`.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `New(Config)`, `DispatchOnce`, and the transaction-scoped `CountUnresolvedOutcomes` callback |
| App | Ordered lease, authorization, dispatch, device-verification, finalization and reconciliation use cases |
| Domain | Lease and admission decisions, authorization rules, dispatch classification, reconciliation evidence rules, and command/intent/decision/outcome document checks |
| Store | Opaque transactions, ledger SQL, read-only authority projections, and same-transaction owner, interlock, device-authority and notification plumbing |

Document codecs live in domain, not in a separate wire layer: they are a few
schema-and-digest checks over `canonicaljson` and `contractsv1`, they decide
nothing about I/O, and a package of that size would only add an import hop.

`New` requires the database, effector, runtime ownership check and interlock;
a missing one is a constructor error, never a silently skipped check. Clock,
identity source, lease owner name and lease duration keep their defaults, none
of which weaken authorization. Composition without a runtime owner supplies an
explicit always-pass ownership check, as it does for policy.

Ordering is fixed. Dispatch asserts ownership, finds the oldest eligible
outbox row, and decides its admission: an expired in-flight lease becomes an
unknown outcome without calling the effector, an altered command document fails
the command, a terminal command only closes its outbox row, and anything else
is leased and marked dispatching. Authorization is then re-proven in one
transaction (live lease, command/intent/decision digests and bindings, approval,
current Situation and episode, interlock, policy digest) and the lease refreshed
last. The effector call and device-state query run outside any transaction; a
deadline or uncertain result stays unknown and is never retried blindly.

An effector that cannot enforce the final authorization check
(`actionport.AuthorizedEffector`) fails the command closed before any call.
Reconciliation is a private use case reached only after independent device
verification settles an unknown outcome; its evidence rules apply in order:
final status, evidence presence, source, type, device-feedback fields, first
present digest.

[Migration findings and audit](../../docs/actions-reference-module-2026-10-05/README.md)
record changes and remaining owner-read-port work. Architecture gates enforce
pure domain, opaque transactions, exact write ownership of `commands`,
`outbox`, `outcomes` and `verifications`, and facade delegation.
