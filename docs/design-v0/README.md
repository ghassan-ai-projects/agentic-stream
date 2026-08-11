# Agentic Stream V0

Status: ready to implement

Decision date: 2026-07-29

Time box: five weeks for one senior engineer

## Executive decision

V0 is a **single-purpose streaming diagnosis runtime**, not an agent platform.

It accepts events, evaluates an injected and versioned Situation Model,
maintains one durable Situation per entity, and asks one model for a structured
diagnosis only when the model moves that Situation into `warning`.

```text
JSONL or HTTP events
        |
        v
validate + deduplicate + persist
        |
        v
injected Situation Model
        |
        v
normal / watch / warning Situation
        |
        v
one bounded model call on warning transition
        |
        v
stored diagnosis + recommendation
```

The runtime never executes the recommendation. A human reads it through the API
or CLI.

## Injection invariant

The binary contains no domain behavior.

The motor example is an external Situation Model loaded through the same public
model-installation path as any other domain. Event names, units, windows, fact
definitions, state names, thresholds, hysteresis, cognition transitions,
episode instructions, and Decision schemas all come from that injected model.

The compiled runtime contains only bounded execution semantics such as “compute
an average,” “evaluate a typed comparison,” and “select a prioritized state.”
Those are platform mechanics, not product rules.

## Product hypothesis

The product is worth continuing only if this loop is useful:

> Continuous evidence can be reduced into a stable Situation so that a model is
> called rarely, with better context, and produces a more useful diagnosis than
> a threshold alert alone.

V0 tests that claim directly. It does not build the generalized platform needed
for hypothetical future use cases.

## Technology

| Area | V0 choice |
|---|---|
| Language | Go 1.26 |
| Process | One binary, one process |
| Persistence | SQLite WAL via `database/sql` and `modernc.org/sqlite` |
| HTTP | Go `net/http` |
| CLI | Go standard `flag` package |
| Logging | Go `log/slog` |
| Reasoning | One configured LLM JSON/HTTP provider adapter |
| Test stack | Go `testing`, `httptest`, deterministic fake model |
| Domain model | Injected strict JSON, validated and compiled to typed internal form |
| Input | JSONL replay and loopback HTTP |

There is no Python runtime, agent framework, broker, RPC layer, ORM, arbitrary
expression language, plugin system, or web application.

## Documents

- [Technical design](TECHNICAL_DESIGN.md) defines the exact runtime behavior,
  contracts, state machine, storage model, APIs, and failure semantics.
- [Situation Model design](SITUATION_MODEL_DESIGN.md) defines the injected
  domain model, deterministic runtime, versioning, explanations, and boundaries.
- [Implementation plan](IMPLEMENTATION_PLAN.md) defines the five-week build
  order, tests, acceptance gates, and stop/go decision.
- [Situation Model schema](contracts/situation-model-v0.schema.json) defines the accepted
  machine-readable configuration.
- [Motor example](examples/motor-warning.situation-model.json) expresses the complete
  demo behavior without Go code.
- [SQLite schema](contracts/storage-schema-v0.sql) is the six-table persistence
  contract.

The broader [version 1 design](../design/README.md) remains research and future
architecture input. It is not an implementation checklist for V0.

Terminology:

- **Situation Model** means the injected deterministic domain configuration.
- **Reasoning model** means the optional LLM used after a cognition trigger.

## V0 boundaries

### Included

- one configured domain: motor predictive maintenance;
- configurable input mappings for vibration, temperature, operating mode, and
  heartbeat;
- configurable fixed windows and lateness allowances;
- duplicate suppression by event ID;
- one active, immutable Situation Model version per entity type;
- configurable derived facts, state transitions, hysteresis, and cognition
  triggers using a constrained operator set;
- deterministic replay from JSONL;
- one bounded, structured model call on entry to `warning`;
- stored diagnoses and recommendations;
- loopback HTTP inspection and a CLI demo;
- restart recovery for pending diagnoses.

### Explicitly excluded

- external actions, tools, approvals, and effectors;
- arbitrary user-defined operators or executable scripts;
- LangChain, LangGraph, Hermes, or OpenClaw integration;
- chat, channels, memory, skills, multi-agent delegation, and tool loops;
- MQTT, Kafka, NATS, gRPC, Python workers, and distributed processing;
- multiple model providers or automatic model routing;
- multitenancy and hostile tenant isolation;
- counterfactual replay, prompt experiments, and a web UI;
- OpenTelemetry, Kubernetes, and production cluster deployment;
- a second production domain.

An excluded item requires a new product decision. It must not enter V0 as
“small infrastructure.”

## Definition of success

V0 succeeds when the included demo proves all of the following:

1. The same trace produces the same Situation versions and trigger decisions.
2. Duplicate and bounded out-of-order events do not corrupt the result.
3. Noise around a threshold does not repeatedly call the model.
4. One warning produces one useful, evidence-linked diagnosis.
5. A restart cannot lose or duplicate an accepted diagnosis.
6. An operator can understand the result from the CLI without reading a model
   transcript.
7. A 24-hour local soak stays within the stated resource bounds.

The product decision after V0 is not “did the software run?” It is whether model
diagnosis materially improves the operational decision over the deterministic
Situation alone.
