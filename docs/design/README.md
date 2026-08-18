# Agentic Stream Design

Status: current v1 design record; implementation has advanced beyond some original milestone wording
Decision date: 2026-07-29  
Target: first production-capable, single-node release of a streaming-native agent runtime

## Outcome

Build a **Situation Runtime**, not a general-purpose agent framework.

The product continuously converts unbounded evidence into durable, versioned
Situations. It starts bounded agent episodes only when a deterministic cognitive
scheduler decides that reasoning is useful. Agents return typed Decisions and
Action Intents. A separate deterministic policy and action plane decides what
may execute.

The runtime is deliberately lighter than Hermes Agent and OpenClaw:

- no channel gateway, messaging platform, desktop application, or marketplace;
- no graph engine in the event hot path;
- no LLM invocation per event;
- no multi-agent mesh, autonomous self-modification, or general workflow UI;
- no distributed stream engine in version 1;
- no direct model access to effectors or production credentials.

## Technology decision

| Area | Choice |
|---|---|
| Core runtime and CLI | Go 1.26 |
| Deployment shape | Modular monolith; one Go binary plus optional Go worker processes |
| Local persistence | SQLite 3 in WAL mode through `modernc.org/sqlite` |
| Spec format | YAML authoring, JSON Schema validation, canonical JSON digest |
| Rule expressions | CEL through `cel-go`, restricted to deterministic functions |
| Public local API | Small JSON/HTTP health/control surface plus Server-Sent Events using Go `net/http` |
| Worker protocol | Protobuf and gRPC over Unix domain socket by default |
| Worker implementations | Go 1.26 only; native executor or current-v1 Go worker process |
| Telemetry | OpenTelemetry traces, metrics, and structured logs |
| Current ingress | Simulator and file replay |
| Deferred ingress | HTTP and MQTT adapters are design targets, not current runtime surfaces |
| Later durable brokers | Kafka via `franz-go`; NATS via `nats.go` |
| Core license recommendation | Apache-2.0 |

The core does not depend on LangChain, LangGraph, Hermes Agent, or OpenClaw.
Those systems can be supported later as `EpisodeExecutor` adapters outside the
stream core.

## Document map

1. [Technical design](TECHNICAL_DESIGN.md) — product boundary, architecture,
   invariants, data model, algorithms, persistence, APIs, failure semantics,
   security, observability, and deployment.
2. [Implementation plan](IMPLEMENTATION_PLAN.md) — milestones, epics, ordered
   work packages, acceptance gates, testing strategy, and release criteria.
3. [Architecture decisions](DECISIONS.md) — language/framework evaluation and
   accepted ADRs.
4. [Research evidence](EVIDENCE.md) — findings from the existing reports and
   the four reference source trees, including adopted and rejected patterns.
5. [Runtime worker protocol](contracts/runtime-v1.proto) — the language-neutral
   episode worker and evidence-tool boundary.
6. [SituationSpec schema](contracts/situation-spec-v1.schema.json) — the
   authoring contract for the first implementation.
7. [Storage schema](contracts/storage-schema-v1.sql) — the logical SQLite
   tables, keys, constraints, and indexes for the first implementation.
8. [Predictive-maintenance example](examples/predictive-maintenance.situation.yaml)
   — an end-to-end example used by the simulator and golden replay suite.

## Product invariants

These are release-blocking, not guidelines:

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. A Situation version is immutable after publication.
4. Deterministic state changes are serial per virtual partition.
5. Every episode is bound to one immutable Situation snapshot and finite budget.
6. A model can read evidence and propose typed intents; it cannot execute effects.
7. Policy revalidates every intent against current state immediately before dispatch.
8. Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.
9. Replay never performs external effects unless an explicit, separate simulation
   mode is selected.
10. Every admitted, deferred, coalesced, rejected, canceled, and expired cognitive
    opportunity is explainable from durable records.

## MVP proof

The predictive-maintenance example is the first product acceptance test. A
simulated motor emits temperature, vibration, current, RPM, heartbeat,
operating-mode, and maintenance events. The release must:

- produce identical Situation history for repeated deterministic replay;
- handle duplicates, out-of-order input, late correction, and missing heartbeat;
- suppress noisy repeated cognition through hysteresis, debounce, and cooldown;
- cancel a stale episode after material supersession;
- validate a structured Decision and create a governed maintenance-ticket Intent;
- prevent duplicate ticket effects across crash and replay;
- reproduce the accepted decision with recorded cognition;
- run a new model or prompt against the same trace in effect-disabled shadow mode;
- explain every Situation field, trigger decision, and action outcome.

## Implementation status

This record remains the detailed design authority, but its milestone language
describes sequencing rather than a promise that the repository is still at that
milestone. The current executable status, CLI/API reality, limitations, and
public reading path are maintained under
[`documentation/`](../../documentation/README.md). Historical v0/v0.1 records
are archived separately and are not current implementation authority.
