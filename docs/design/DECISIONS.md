# Architecture Decisions

## Decision summary

| ID | Decision | Status |
|---|---|---|
| ADR-001 | Situation is the primary semantic unit | Accepted |
| ADR-002 | Separate continuous evidence processing from episodic cognition | Accepted |
| ADR-003 | Use Go 1.26 for the core; isolate optional Python workers | Accepted |
| ADR-004 | Start as a single-node modular monolith | Accepted |
| ADR-005 | Use SQLite WAL as the local system of record | Accepted |
| ADR-006 | Author SituationSpec in YAML and compile it to deterministic IR | Accepted |
| ADR-007 | Use at-least-once delivery plus idempotent state effects | Accepted |
| ADR-008 | Agents produce Decisions and Intents, never direct effects | Accepted |
| ADR-009 | Make replay a runtime mode with four explicit semantics | Accepted |
| ADR-010 | Keep agent frameworks outside the stream core | Accepted |
| ADR-011 | Use a typed, process-isolated worker protocol | Accepted |
| ADR-012 | Defer distributed processing and multi-agent orchestration | Accepted |

## Language and framework decision

### Workload characteristics

The core workload is not primarily prompt construction. It is a long-running
stateful service that must:

- consume concurrent streams without unbounded queues;
- preserve per-key ordering and event-time semantics;
- manage windows, durable timers, checkpoints, and replay;
- run predictably on a laptop, server, or edge gateway;
- isolate model SDKs and ML dependencies from the hot path;
- ship with low operational overhead;
- expose simple connector, worker, and effector ports.

### Weighted evaluation

Scores are 1–5. Weights reflect the first product, not general language merit.

| Criterion | Weight | Go | Python | TypeScript | Rust | Java/Kotlin |
|---|---:|---:|---:|---:|---:|---:|
| Concurrent streaming runtime | 25 | 5 | 3 | 4 | 5 | 5 |
| Single-binary/edge operations | 20 | 5 | 2 | 3 | 5 | 2 |
| Delivery speed and maintainability | 20 | 5 | 5 | 4 | 3 | 3 |
| Broker/network ecosystem | 10 | 5 | 4 | 4 | 4 | 5 |
| Agent/model ecosystem | 10 | 3 | 5 | 5 | 2 | 3 |
| Contributor accessibility | 10 | 4 | 5 | 5 | 3 | 3 |
| Isolation from dependency churn | 5 | 5 | 2 | 3 | 5 | 4 |
| Weighted result (/5) | 100 | **4.70** | 3.55 | 4.00 | 4.05 | 3.65 |

### Decision

Use **Go 1.26** for the runtime, CLI, API, persistence, operator engine,
Situation engine, scheduler, policy engine, replay engine, and the native
episode executor.

Use **Python 3.12** only for optional process-isolated workers that need the
Python ML or agent ecosystem. Python is not required to run the simulator,
deterministic stream plane, scheduler, policy plane, replay, or a direct
OpenAI-compatible model executor.

### Why not TypeScript

OpenClaw and OpenCode prove that TypeScript can host excellent agent loops and
event APIs. Their strongest reusable mechanisms are typed event vocabularies,
tool-policy seams, cancellation, and session serialization. The new product's
differentiator is instead durable event-time state and edge-friendly operation.
Go gives simpler deployment, lower baseline overhead, strong concurrency, and
mature broker clients without adopting Bun/Node or Effect-TS as a runtime
dependency.

### Why not Python

Hermes, LangChain, and LangGraph prove Python's strength for agent authoring,
model integration, checkpointed workflows, and experimentation. Python is the
right adapter language, but a weaker default for an embedded long-running
stream engine that must be distributed as one binary and keep model
dependencies out of deterministic recovery.

### Why not Rust

Rust offers the strongest memory safety and performance profile. It adds
implementation and contributor cost before the product has demonstrated a
performance ceiling. The protocol boundaries allow a later Rust edge connector
or operator without replacing the core contracts.

### Why not Java/Kotlin

Java/Kotlin is the natural choice for a Flink- or Kafka-Streams-native product.
Version 1 explicitly avoids building or embedding a distributed stream engine.
Java adapters remain appropriate when SituationSpec is mapped to those systems.

## Framework choices

The default is the standard library. Dependencies are admitted only at a real
boundary.

| Need | Choice | Reason |
|---|---|---|
| HTTP and SSE | Go `net/http` | Adequate routing and streaming; no web framework required |
| CLI | Cobra | Nested operational commands, generated help, stable ecosystem |
| Local SQL | `database/sql` + `modernc.org/sqlite` | Pure-Go builds, WAL, transactions, indexes, FTS option |
| Schema validation | `santhosh-tekuri/jsonschema/v6` | Draft-compatible compiled validation |
| Rule expressions | `google/cel-go` | Typed, sandboxed expressions without arbitrary code |
| Worker RPC | Protobuf + `grpc-go` | Typed streaming, cancellation, Go/Python support |
| Model transport | Provider-specific adapters; official provider SDK where available | Keep SDK types and quirks outside the episode loop |
| MQTT | Eclipse Paho Go | Standard MQTT 5 adapter |
| Kafka, later | `twmb/franz-go` | Native Go, good control over offsets and transactions |
| NATS, later | `nats-io/nats.go` | JetStream support and mature Go client |
| Telemetry | OpenTelemetry Go | Vendor-neutral trace, metric, and log correlation |
| Tests | Go `testing`, fuzzing, `google/go-cmp` | Small test surface and reproducible comparisons |
| Broker tests | `testcontainers-go`, phase 4 onward | Real compatibility tests without mocking protocols |
| Releases | GoReleaser | Reproducible multi-platform binaries and checksums |

Avoid in version 1:

- an ORM;
- a general dependency-injection framework;
- a dynamic Go plugin system;
- OPA/Rego in the request path;
- a vector database;
- an embedded workflow/graph framework;
- Kubernetes operators;
- a JavaScript runtime;
- a mandatory Python environment.

## ADR-001: Situation is the primary semantic unit

**Context.** Messages and graph nodes describe execution. They do not represent
an evolving domain condition supported by many events and time windows.

**Decision.** A durable, versioned Situation keyed by tenant, type, and entity
is the central aggregate. Episodes consume immutable Situation snapshots.

**Consequences.** Developers must define reducers, lifecycle, evidence, and
triggers. A chat transcript cannot become the canonical state store.

## ADR-002: Separate continuous and episodic runtimes

**Context.** Calling an agent for every event creates cost amplification,
backlog instability, contradictory decisions, and weak temporal correctness.

**Decision.** Deterministic stream processing is continuous. Cognition is
finite, scheduled, budgeted, cancelable, and version-bound.

**Consequences.** The scheduler is a first-class product component. Agent
streaming events remain useful for observers but are not the data-stream model.

## ADR-003: Go core with optional Python workers

**Context.** The hot path needs concurrency, predictable resource use, and
simple distribution. Some time-series and agent integrations need Python.

**Decision.** Go owns state and orchestration. Python workers run out of process
behind a versioned protocol and capability-scoped tools.

**Consequences.** The system can run without Python. Worker failure cannot
corrupt stream state. Cross-language contracts must remain compatible.

## ADR-004: Single-node modular monolith first

**Context.** Distribution magnifies errors in ordering, watermark, timer,
checkpoint, and replay semantics.

**Decision.** One process owns the local log, virtual partitions, state,
scheduler, action policy, API, and telemetry. Modules have explicit internal
boundaries but are not independent services.

**Consequences.** Scale is bounded by one node. Virtual partitioning and ports
preserve a later broker-backed path without paying distributed-systems cost now.

## ADR-005: SQLite WAL is the local system of record

**Context.** The product needs an append log, keyed state, immutable Situation
history, timers, ledgers, inbox/outbox records, and atomic transitions.

**Decision.** Use one SQLite database in WAL mode for version 1. Large raw
payloads may spill to content-addressed files with references in SQLite.

**Consequences.** Transactions can atomically connect state and outbox records.
Write concurrency is deliberately bounded. Broker-backed deployments retain the
same logical schema but do not assume a distributed transaction with SQLite.

## ADR-006: Compile SituationSpec to deterministic IR

**Context.** A developer needs one portable artifact for inputs, time policy,
operators, Situation lifecycle, triggers, cognition, and actions.

**Decision.** Accept YAML or JSON, validate with JSON Schema, perform semantic
validation, normalize defaults, compile CEL expressions, and hash canonical JSON.
Deployments reference the immutable digest.

**Consequences.** Syntax alone is insufficient; compiler errors must explain
invalid topology, units, references, time policy, and non-deterministic rules.
Arbitrary scripts are not allowed in specifications.

## ADR-007: At-least-once plus idempotent state effects

**Context.** Exactly-once stream state does not guarantee exactly-once model
calls, tickets, notifications, or physical actions.

**Decision.** Cross-boundary delivery is at least once. Stable event IDs,
inboxes, unique constraints, transactional outboxes, command idempotency keys,
and reconciliation prevent duplicate accepted effects.

**Consequences.** Each connector and effector publishes its real guarantee.
Unknown external outcomes become `reconcile_required`, not blind retries.

## ADR-008: Agents cannot execute effects

**Context.** Model output is non-deterministic and untrusted. Direct access to
effectors makes replay unsafe and policy bypass possible.

**Decision.** Episodes may call capability-scoped read tools and return typed
Decisions plus Action Intents. Policy creates Commands only after deterministic
validation against current state.

**Consequences.** The native loop is smaller than a general assistant loop.
Action latency includes validation or approval. Safety and audit improve.

## ADR-009: Four replay modes

**Decision.**

1. `deterministic`: rerun ingress, operators, reducers, and triggers with no cognition or effects.
2. `recorded`: reuse recorded worker/tool events to reproduce accepted decisions.
3. `shadow`: run a new executor/model/prompt with all effects disabled.
4. `counterfactual`: evaluate proposed commands through a simulator or recorded outcome model.

Replay mode is explicit in every run and trace. No replay mode silently enables
real effectors.

## ADR-010: Agent frameworks are adapters

**Context.** LangGraph supplies durable graph execution; LangChain supplies
model/tool middleware; Hermes and OpenClaw supply mature personal-agent loops.
None owns event-time temporal truth.

**Decision.** The core defines a narrow `EpisodeExecutor` port. Framework
adapters run in optional worker packages.

**Consequences.** Framework upgrades do not migrate Situation state or control
the action plane. The native direct-model executor remains the compatibility
baseline.

## ADR-011: Typed process-isolated worker protocol

**Decision.** Use Protobuf/gRPC. The runtime sends an immutable request and
capability token. Workers stream lifecycle and model events. Evidence tools are
called through a runtime-owned gRPC service. Unix domain sockets are the local
default; remote workers require mTLS and explicit enablement.

**Consequences.** Cancellation maps to RPC context cancellation. Worker stdout
is diagnostic only. Protocol compatibility tests are mandatory.

## ADR-012: Defer distribution and multi-agent orchestration

**Decision.** Version 1 has one bounded episode per admitted trigger and at most
one active episode per Situation. It does not implement agent-to-agent
communication, debate, recursive delegation, or distributed state ownership.

**Revisit when.**

- single-node throughput or availability is a measured blocker;
- a use case demonstrates better outcomes from multi-agent execution after
  controlling for added cost and latency;
- the compatibility suite and replay semantics are stable enough to detect
  regressions across nodes.
