# Agentic Stream

> **Codename:** Situation Runtime · **Status:** implementation-ready design baseline · **Decision date:** 2026-07-29

A streaming-native agent runtime. It continuously converts unbounded evidence into durable, versioned **Situations**, and starts bounded agent **episodes** only when a deterministic cognitive scheduler decides that reasoning is useful. Agents return typed Decisions and Action Intents; a separate deterministic policy and action plane decides what may execute.

Deliberately lighter than Hermes Agent and OpenClaw:

- no channel gateway, messaging platform, desktop application, or marketplace;
- no graph engine in the event hot path;
- no LLM invocation per event;
- no multi-agent mesh, autonomous self-modification, or general workflow UI;
- no distributed stream engine in version 1;
- no direct model access to effectors or production credentials.

## Product invariants

Release-blocking, not guidelines:

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. A Situation version is immutable after publication.
4. Deterministic state changes are serial per virtual partition.
5. Every episode is bound to one immutable Situation snapshot and finite budget.
6. A model can read evidence and propose typed intents; it cannot execute effects.
7. Policy revalidates every intent against current state immediately before dispatch.
8. Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.
9. Replay never performs external effects unless an explicit, separate simulation mode is selected.
10. Every admitted, deferred, coalesced, rejected, canceled, and expired cognitive opportunity is explainable from durable records.

## Technology

| Area | Choice |
|---|---|
| Core runtime and CLI | Go 1.26 |
| Deployment shape | Modular monolith; one Go binary plus optional Go worker processes |
| Local persistence | SQLite 3 in WAL mode through `modernc.org/sqlite` |
| Spec format | YAML authoring, JSON Schema validation, canonical JSON digest |
| Rule expressions | CEL through `cel-go`, restricted to deterministic functions |
| Public local API | JSON/HTTP plus Server-Sent Events using Go `net/http` |
| Worker protocol | Protobuf and gRPC over Unix domain socket by default |
| Worker implementations | Go 1.26 only; native executor or current-v1 Go worker process |
| Telemetry | OpenTelemetry traces, metrics, and structured logs |
| First ingress | Simulator, file replay, HTTP, then MQTT |
| Later durable brokers | Kafka via `franz-go`; NATS via `nats.go` |
| Core license | Apache-2.0 |

The core does not depend on LangChain, LangGraph, Hermes Agent, or OpenClaw. Those can be supported later as `EpisodeExecutor` adapters outside the stream core.

## Data flow

```text
ingress -> eventlog -> engine/operators -> situations -> cognition
   -> episodes -> evidence/decisions -> policy -> actions
```

## MVP proof

The predictive-maintenance example is the first product acceptance test. A simulated motor emits temperature, vibration, current, RPM, heartbeat, operating-mode, and maintenance events. The release must produce identical Situation history for repeated deterministic replay; handle duplicates, out-of-order input, late correction, and missing heartbeat; suppress noisy repeated cognition through hysteresis, debounce, and cooldown; cancel a stale episode after material supersession; validate a structured Decision and create a governed maintenance-ticket Intent; prevent duplicate ticket effects across crash and replay; reproduce the accepted decision with recorded cognition; run a new model or prompt against the same trace in effect-disabled shadow mode; and explain every Situation field, trigger decision, and action outcome.

## Documents

The full specification lives in [docs/](docs/design/README.md):

| Document | Contents |
|---|---|
| [docs/design/TECHNICAL_DESIGN.md](docs/design/TECHNICAL_DESIGN.md) | Product boundary, architecture, invariants, data model, algorithms, persistence, APIs, failure semantics, security, observability, deployment |
| [docs/design/IMPLEMENTATION_PLAN.md](docs/design/IMPLEMENTATION_PLAN.md) | Milestones, epics, ordered work packages, acceptance gates, testing strategy, release criteria |
| [docs/design/DECISIONS.md](docs/design/DECISIONS.md) | Language/framework evaluation and accepted ADRs |
| [docs/design/EVIDENCE.md](docs/design/EVIDENCE.md) | Research findings from the reports and the four reference source trees |
| [docs/contracts/](docs/contracts/) | `runtime-v1.proto` worker protocol, `situation-spec-v1` schema, `storage-schema-v1.sql` |
| [docs/examples/predictive-maintenance.situation.yaml](docs/examples/) | End-to-end example used by the simulator and golden replay suite |
| [docs/design-v0/](docs/design-v0/) · [docs/design-v0.1/](docs/design-v0.1/) | Archived design iterations (incl. critique of v0, evaluation design, review) |
| [docs/research/](docs/research/) | Study reports: OpenClaw architecture & agent patterns; streaming-native agent runtime architecture |

**Start here:** [docs/design/README.md](docs/design/README.md), then the technical design, then the implementation plan. Begin with Milestone 0 — not MQTT, LangGraph integration, a web UI, or a real model provider. The first vertical slice uses a file trace, virtual clock, deterministic operators, SQLite, a fake episode executor, and a simulated effector.

## Development

```bash
make ci-check        # tidy + build + vet + lint + test-short + deadcode + vulncheck
make test            # race + shuffle + coverage
make test-coverage   # coverage HTML report
make lint            # golangci-lint
make cross-compile   # linux/amd64 binary
```

For coding agents: read [AGENTS.md](AGENTS.md) before editing.

## License

Apache-2.0 — see [LICENSE](LICENSE).
