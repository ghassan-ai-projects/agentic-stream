# Product overview

Agentic Stream is a single-node, streaming-native runtime for situations that
develop over time. It continuously processes evidence, maintains durable
Situation state, and invokes an agent only when a deterministic scheduler has
enough reason to do so.

## What it is

The runtime has two deliberately different halves:

- A deterministic stream plane ingests events, applies event-time rules and
  operators, publishes immutable Situation versions, and records why a change
  mattered.
- A bounded cognition and action path reads one immutable Situation snapshot,
  proposes typed Decisions and Intents, and sends only policy-approved Commands
  to an effector.

This shape is useful when an event stream is noisy, late, duplicated, or
incomplete, and when an AI proposal must not become an external effect by
itself. Predictive maintenance is the first proof domain, but domain behavior
is authored as data in a `SituationSpec`.

## What it is not

Agentic Stream is not:

- a general workflow or graph engine;
- a message gateway, chat application, desktop agent, or marketplace;
- an LLM call on every event;
- a multi-agent mesh or self-modifying agent;
- a distributed broker-backed stream engine in version 1;
- a direct model-to-effector bridge;
- a web UI or a replacement for a domain system of record.

The runtime intentionally starts with a modular monolith, SQLite WAL, JSONL
ingress, and Go worker boundaries. Kafka, NATS, MQTT, and broader deployment
topologies are deferred until the single-node semantics are proven.

## Who it is for

- Engineers building event-driven systems that need durable intermediate
  situations rather than stateless alerts.
- Safety- and operations-minded teams evaluating bounded AI proposals beside a
  deterministic policy plane.
- Researchers and maintainers who need replayable evidence, explainable
  admission decisions, and effect-disabled shadow evaluation.

It is not yet positioned as a turnkey production service. The [current status]
and [limitations](limitations.md) pages describe the evidence boundary.

## The first proof

The predictive-maintenance example models a motor with temperature, vibration,
current, RPM, heartbeat, operating mode, and maintenance events. The acceptance
path requires deterministic replay, duplicate and out-of-order handling,
hysteresis/debounce/cooldown, stale-episode cancellation, typed and governed
intents, idempotent effects, shadow mode, and explainability.

## Guarantees worth understanding

The runtime is designed around ten release-blocking invariants. The most
important product boundary is simple: raw evidence may influence a model, but
it cannot directly instruct the action plane. Read the [invariant contract]
before evaluating an integration.

[invariant contract]: ../architecture/invariants.md

## Next reads

- [Core concepts](concepts.md)
- [Current status](status.md)
- [Limitations](limitations.md)
- [Architecture overview](../architecture/overview.md)
