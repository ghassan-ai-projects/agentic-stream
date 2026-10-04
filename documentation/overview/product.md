# Product overview

Agentic Stream follows conditions that develop over time. It processes
events continuously, stores the evolving condition as a Situation, and asks
an agent to reason when declared rules say that would be useful. The current
runtime runs on one node.

## What it is

The runtime separates continuous processing from bounded reasoning and action:

- A deterministic stream plane ingests events, applies event-time rules and
  operators, publishes immutable Situation versions, and records why a change
  mattered.
- A bounded cognition and action path reads one immutable Situation snapshot,
  proposes typed Decisions and Intents, and sends only policy-approved Commands
  to an effector.

This approach is useful when an event stream is noisy, late, duplicated, or
incomplete, and when an AI proposal must not become an external effect by
itself. Predictive maintenance is the first example. A `SituationSpec` declares the
inputs, state rules, reasoning triggers, and permitted proposals for a domain.

## What it is not

Agentic Stream is not:

- a general workflow or graph engine;
- a message gateway, chat application, desktop agent, or marketplace;
- an LLM call on every event;
- a multi-agent mesh or self-modifying agent;
- a distributed broker-backed stream engine in version 1;
- a direct model-to-effector bridge;
- a web UI or a replacement for a domain system of record.

The runtime starts as one application with separate internal modules. It uses
SQLite in write-ahead log (WAL) mode, reads JSON Lines (JSONL) from files or a
live Unix socket, and can delegate reasoning to a Go worker. Kafka, NATS, MQTT,
and distributed deployments are deferred until the single-node behavior is
proven.

## Who it is for

- Engineers building event-driven systems that need a stored history of
  conditions, beyond individual alerts.
- Teams evaluating limited AI proposals with explicit policy and operational
  controls.
- Researchers and maintainers who need replayable evidence, explainable
  admission decisions, and effect-disabled shadow evaluation.

It is not yet a production service ready to deploy as supplied. The [current status](status.md)
and [limitations](limitations.md) pages describe the evidence boundary.

## The first proof

The predictive-maintenance example models a motor with temperature, vibration,
current, and heartbeat events. Acceptance requires deterministic replay, duplicate and out-of-order handling,
hysteresis/debounce/cooldown, stale-episode cancellation, typed and governed
intents, idempotent effects, shadow mode, and explainability.

## Guarantees worth understanding

The runtime is designed around ten release-blocking invariants. The most
important product boundary is simple: raw evidence may influence a model, but
it cannot directly instruct the action plane. Read the [invariant contract]
before evaluating an integration.

[invariant contract]: ../architecture/invariants.md

## Next reads

- [Why the project exists](../learn/why.md)

- [Core concepts](concepts.md)
- [Current status](status.md)
- [Limitations](limitations.md)
- [Architecture overview](../architecture/overview.md)
