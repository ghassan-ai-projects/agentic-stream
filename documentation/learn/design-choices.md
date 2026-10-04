# Why these design choices

For readers who have followed the full flow: this page connects the design to
its tradeoffs. These choices describe the current system, not an unlimited
scalability or production-safety promise.

## Keep continuous state separate from finite reasoning

The stream engine needs predictable ordering, time rules, and repeatable state
changes. An agent needs a bounded question and may return different answers.
Keeping those jobs separate allows deterministic stream replay and lets the
scheduler control reasoning cost and freshness.

The cost is explicit domain modeling: developers must define inputs,
calculations, lifecycle, and triggers. A model cannot invent the state rules
as it goes. See [ADR-001](../../docs/design/DECISIONS.md#adr-001-situation-is-the-primary-semantic-unit)
and [ADR-002](../../docs/design/DECISIONS.md#adr-002-separate-continuous-and-episodic-runtimes).

## Begin with one process and one database

A **modular monolith** is one application with clear internal owners. SQLite
in **write-ahead log (WAL)** mode supplies the durable records and transactions.
The stream can commit state and queued work together, without coordinating
separate services for every reading.

That keeps ordering, recovery, and audit easier to reason about. It also bounds
write concurrency and capacity to one node. Virtual partitions divide state
ownership inside this design; they do not automatically distribute it across
machines. See [ADR-004](../../docs/design/DECISIONS.md#adr-004-single-node-modular-monolith-first)
and [ADR-005](../../docs/design/DECISIONS.md#adr-005-sqlite-wal-is-the-local-system-of-record).

## Author behavior as data

A SituationSpec declares the domain rules. The compiler checks the schema and
meaning of those rules, then produces a canonical digest, a content identity
for the normalized spec. Equivalent authoring syntax can share that identity.

This makes the deployed behavior inspectable and replayable. It also restricts
what a spec can express: arbitrary scripts are not part of the contract.
Some accepted policy fields have partial enforcement today; schema acceptance
alone is not a capability promise. See
[ADR-006](../../docs/design/DECISIONS.md#adr-006-compile-situationspec-to-deterministic-ir)
and [SituationSpec limitations](../overview/limitations.md).

## Keep model proposals away from effect credentials

Typed proposals, validation, current-state policy, and a durable outbox create
an auditable execution path. An **outbox** is a durable queue of governed
requests awaiting dispatch. A separate worker can reason through a narrow
Go protocol without taking ownership of stream state or effects.

These boundaries add validation and possible approval latency. They also make
refusal, cancellation, and uncertain outcomes visible. Correctness still
requires trustworthy effect adapters and deployment controls. See
[ADR-008](../../docs/design/DECISIONS.md#adr-008-agents-cannot-execute-effects),
[worker boundary](../architecture/worker-boundary.md), and
[module ownership](../architecture/modules.md).

## Use identities and reconciliation across boundaries

Delivery can repeat after a crash or lost reply. Stable event identities,
transactional records, Command idempotency keys, and reconciliation let the
runtime distinguish a repeated request from new work.

This is not a universal exactly-once guarantee. Every external integration
needs an honest statement of its deduplication and outcome-verification
behavior. See [ADR-007](../../docs/design/DECISIONS.md#adr-007-at-least-once-plus-idempotent-state-effects).

## Choose your next level of detail

| You want to understand… | Read next |
| --- | --- |
| The four broad stages | [Learning overview](README.md) |
| The main runtime owners | [Architecture overview](../architecture/overview.md) |
| Timing, admission, and execution mechanics | [Design reading order](../design/README.md) |
| Package imports and durable write ownership | [Business module map](../architecture/modules.md) |
| The full design record and decisions | [ADRs](../adr/README.md) |

## Next reads

- [Architecture overview](../architecture/overview.md)
- [Try the quickstart](../getting-started/quickstart.md)
- [Current implementation status](../overview/status.md)
