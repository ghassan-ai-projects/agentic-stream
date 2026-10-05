# Cognition module

Cognition evaluates bounded reasoning opportunities deterministically. It
records why a trigger was ignored, deferred or admitted, manages queue
replacement through the owning ledgers, and admits reconsideration when a
correction invalidates a successful action. It never invokes models or effects.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `New(Config)`, `Process`, and the cost-refusal handoff |
| App | Ordered evaluation, admission, supersession and correction use cases |
| Domain | CEL gates, deltas, identities, timing, capacity and correction evidence |
| Store | Opaque caller transaction, SQL, record codecs and ledger/notification plumbing |

`New` requires a compiled spec and tenant/deployment identities. Clock and ID
source retain their physical/random defaults; deterministic composition supplies
both explicitly. `Process` joins its caller's transaction. History reads,
evaluations, coalescing, episode cancellation, approval withdrawal, notifications
and the last-reasoned marker commit or roll back together. There is no internal
commit and no exposed SQL handle.

Trigger order and clock-read points remain unchanged. Condition precedes score,
material delta and threshold. A material-delta error preserves its score without
recording a verdict. Capacity allows replacement of an existing same-trigger
pending item. Corrections verify the persisted snapshot digest and deduplicate
by Situation, superseded version and command identity.

The root exposes no `Engine`, `Scheduler` or mutable evaluation state. Tests
exercise the same durable path as production. [Migration findings and audit](../../docs/cognition-reference-module-2026-10-05/README.md)
record changes and remaining owner-read-port work. Architecture gates enforce
pure domain, opaque transactions, exact write ownership and facade delegation.
