# Core concepts

The runtime is easier to reason about when its records and boundaries stay
distinct. This page is the shared vocabulary for the rest of the documentation.

## Evidence and events

An **event** is an observed fact with an identity, event time, ingestion time,
tenant, source, partition key, entity, classification, schema version, and
payload. The event log treats it as evidence. It is never an instruction to
execute a command.

Event time and watermarks are explicit. A late event can be dropped with an
audit, retained only in history, or correct an earlier result depending on the
specification.

## Situation and Situation version

A **Situation** is the durable, domain-shaped state derived from a partition's
evidence. A **Situation version** is an immutable publication of that state,
including completeness, provenance, material deltas, and a canonical digest.
Later evidence creates a new version; it mutates no published version.

The stream plane owns this state. An agent never edits it directly.

## Cognitive opportunity and episode

A **cognitive opportunity** is a durable scheduler item that says a Situation
may deserve reasoning. The scheduler applies deterministic trigger conditions,
scores, thresholds, debounce, cooldown, coalescing, capacity, freshness, and
supersession rules.

An **episode** is the bounded execution attempt attached to one immutable
Situation snapshot. Its budget covers dimensions such as wall time, model calls,
tokens, tool calls, tool-result bytes, retries, and cost. A stale or superseded
episode can be canceled; a late worker result is fenced out.

## Decision, Intent, policy, Command, outcome

The roles are different:

| Record | Meaning | Authority |
| --- | --- | --- |
| Decision | Structured agent proposal tied to an episode and snapshot | Model proposes; runtime validates |
| Intent | One typed, risk-classified proposed effect | Runtime validates and policy evaluates |
| Policy result | Durable allow, deny, defer, approval, or idempotent reuse | Deterministic policy plane |
| Command | Governed action request created from an accepted Intent | Action plane dispatches |
| Outcome | Effect result, including unknown/reconciliation state | Effector and dispatcher |

An agent can propose a Decision and Intents. It cannot create a Command or call
an effector.

## Replay modes

The default deterministic replay reconstructs the stream plane and has no
external effects. Recorded replay uses a durable worker ledger; shadow replay
compares a new executor without entering governance; counterfactual replay is
simulator-only. These are explicit capabilities, not flags that silently turn
production effects on.

## Virtual partition

State changes are serialized per virtual partition, normally derived from the
event's partition key. Different partitions can progress independently, but
one partition has one deterministic order at a time.

## Evidence boundary

Episode tools are read-only and scoped. A worker receives an immutable request,
snapshot, schemas, catalogs, budgets, and a short-lived capability. The runtime
hosts EvidenceTools when configured and validates the worker's output after it
returns. Credentials and effectors stay outside the model/tool boundary.

## Next reads

- [Architecture overview](../architecture/overview.md)
- [Stream processing design](../design/stream-processing.md)
- [Cognition design](../design/cognition.md)
- [Decisions and actions](../design/decisions-and-actions.md)
