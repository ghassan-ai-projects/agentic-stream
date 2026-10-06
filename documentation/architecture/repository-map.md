# Repository map

This is the current implementation map. It intentionally differs from the
historical design map where the code has chosen a more specific package name.

| Path | Responsibility |
| --- | --- |
| `cmd/agentic-stream/` | CLI entrypoint and runtime wiring |
| `internal/spec/` | SituationSpec schema, compiler, CEL, deployment persistence (facade) |
| `internal/spec/internal/domain/` | pure spec parsing, validation, normalization, reference resolution, CEL, durations and the event schema registry |
| `internal/spec/internal/app/` | compile a spec from a file |
| `internal/spec/internal/store/` | spec deployment and event schema SQL |
| `internal/ingress/` | ingress configuration and replay/serve delegation facade |
| `internal/ingress/internal/app/` | JSONL and simulator replay loops, admission, quarantine and live line handling |
| `internal/ingress/internal/store/` | connector checkpoint SQL |
| `internal/ingress/internal/domain/` | pure envelope admission, simulator trace grammar and conversion, identities, checkpoint codec |
| `internal/ingress/internal/transport/` | trace file and Unix-socket framing, listener safety and client plumbing |
| `internal/eventlog/` | append-only events, validation, dedup, quarantine, gaps |
| `internal/eventlog/internal/domain/` | pure evidence admission, quarantine identity and decode rules (reference module layer) |
| `internal/eventlog/internal/app/` | append, quarantine, release/redrive and read use cases |
| `internal/eventlog/internal/store/` | event-log SQL and units of work (reference module layer) |
| `internal/engine/` | engine configuration and global-run delegation facade |
| `internal/engine/internal/app/` | serialised runs, per-record transactions, timers and rollback restore |
| `internal/engine/internal/store/` | opaque transactions, engine SQL and owner fence |
| `internal/engine/internal/domain/` | pure watermark, Situation state, lineage, timer and heartbeat rules |
| `internal/operators/` | aggregates, slopes, missing-heartbeat and related features (facade) |
| `internal/operators/internal/domain/` | pure operator runtime: windows, aggregates, slopes, heartbeats and boot admission |
| `internal/situations/` | Situation state and immutable versions (facade) |
| `internal/situations/internal/domain/` | pure Situation engine: reducers, transition evaluation, materialization and versioning |
| `internal/cognition/` | configured deterministic scheduler facade |
| `internal/cognition/internal/app/` | ordered trigger, queue and correction use cases |
| `internal/cognition/internal/domain/` | pure evaluation, timing, capacity and correction rules |
| `internal/cognition/internal/store/` | opaque caller transaction, cognition SQL and owning-ledger handoffs |
| `internal/episodes/` | public facade over episode assembly and bounded execution |
| `internal/episodes/internal/app/` | transaction-scoped episode use cases: assembly, claim, execution, decisions |
| `internal/episodes/internal/domain/` | pure episode contracts and rules: request assembly, budgets, decisions, failures, shadow scoring |
| `internal/episodes/internal/store/` | opaque caller transaction, episode SQL and atomic ledger/cost/epoch handoffs |
| `internal/evidence/` | configured capability, bounded-query and recovery facade |
| `internal/evidence/internal/domain/` | pure evidence scope, lifetime and authorization rules |
| `internal/evidence/internal/wire/` | exact v1 token, argument and result codecs |
| `internal/evidence/internal/app/` | capability issuance/verification, call admission/query and durable lifecycle use cases |
| `internal/evidence/internal/store/` | opaque original transactions, evidence-call ledger SQL and ownership assertions |
| `internal/evidence/internal/transport/` | gRPC adaptation and the eventlog owner-provided evidence source |
| `internal/decisions/` | thin facade over pure Decision and Intent validation |
| `internal/decisions/internal/domain/` | pure binding, catalog authority, risk, freshness and evidence rules |
| `internal/policy/` | policy configuration and delegation facade |
| `internal/policy/internal/app/` | ordered intent evaluation and human approval use cases |
| `internal/policy/internal/store/` | caller-owned transactions, policy SQL and ledger/notification plumbing |
| `internal/policy/internal/domain/` | pure policy records, canonical definitions and governance checks |
| `internal/actions/` | governed dispatch configuration and delegation facade |
| `internal/actions/internal/app/` | ordered lease, authorization, dispatch, verification and reconciliation use cases |
| `internal/actions/internal/store/` | opaque transactions, command/outbox/outcome/verification SQL and owner, interlock, authority and notification plumbing |
| `internal/actions/internal/domain/` | pure dispatch, authorization and reconciliation rules with command, intent, decision and outcome document checks |
| `internal/watch/` | watch configuration and effect-port delegation facade |
| `internal/watch/internal/app/` | install, fire and expire use cases with busy retry |
| `internal/watch/internal/store/` | opaque transactions, watch SQL and owner/interlock plumbing |
| `internal/watch/internal/domain/` | pure payload rules, watch identity, CEL validation and evaluation |
| `internal/runartifact/internal/app/` | export and verify use cases |
| `internal/runartifact/internal/store/` | read-only snapshot SQL and the device authority safety read |
| `internal/runartifact/internal/domain/` | pure manifest, ledger encoding, checksum, binding, soak verdict and verification rules |
| `internal/runartifact/internal/transport/` | artifact directory: reserve, atomic publish, read |
| `internal/executor/remote/internal/app/` | worker episode execution: budget bound, handshake, capability, stream consumption |
| `internal/executor/remote/internal/domain/` | pure wire-request mapping, handshake and stream validation, budget accounting |
| `internal/executor/remote/internal/transport/` | EpisodeWorker gRPC calls and cancellation mapping |
| `internal/executor/native/internal/app/` | native episode loop: budgets, tool runs, repair, artifacts |
| `internal/executor/native/internal/domain/` | pure provider and tool contracts, budget accounting, request decoding, Decision and evidence-scope rules |
| `internal/executor/native/internal/transport/` | OpenAI-compatible HTTP provider and response decoding |
| `internal/executor/native/internal/store/` | scope-bound event-log evidence tool |
| `internal/actionport/` | approved command/effect contracts |
| `internal/device/` | device effect boundary facade: effect profiles, catalog loading, gateway dial and effector constructors |
| `internal/device/internal/app/` | device session use cases and the gateway, simulated and fail-closed effectors |
| `internal/device/internal/wire/` | device record codec: schema-validated canonical NDJSON, typed records and original evidence documents |
| `internal/device/internal/transport/` | Unix-socket gateway link: framing, deadlines and the may-have-sent signal |
| `internal/device/internal/domain/` | device-boundary rules: capability catalog, materialization, effect profiles, record matching, output verification |
| `internal/episodeledger/` | scheduler queue, episode and attempt lifecycle facade: fencing, rejection audit, supersession and recovery |
| `internal/episodeledger/internal/app/` | queue, admission, attempt, identity, rejection and recovery use cases |
| `internal/episodeledger/internal/domain/` | pure statuses, identity and fence checks, transition table, rejection and recovery rules |
| `internal/episodeledger/internal/store/` | the only SQL for `scheduler_items`, `episodes`, `episode_attempts` and `episode_rejections` |
| `internal/approvalledger/` | human approval lifecycle facade: request, expiry, resolution, assertion binding, withdrawal |
| `internal/approvalledger/internal/app/` | lifecycle writes and the superseded-approval withdrawal with caller-published notifications |
| `internal/approvalledger/internal/domain/` | approval states, stable reasons and the withdrawal fact |
| `internal/approvalledger/internal/store/` | the only SQL for `approvals` |
| `internal/control/` | runtime control plane facade: owner lease, epoch drain/kill, cost ledger and ceilings, readiness capability |
| `internal/control/internal/app/` | owner, epoch, cost and dispatch-readiness use cases |
| `internal/control/internal/domain/` | pure lease, epoch-refusal and cost rules |
| `internal/control/internal/store/` | the only SQL for `runtime_owner`, `epoch_control`, `cost_limits` and `cost_reservations` |
| `internal/authority/` | device target claims, bindings, reconciliation and safety evidence: public API only (configuration and delegation) |
| `internal/authority/internal/app/` | device-authority use cases: validation, admission, unit of work, audit (reference module layer) |
| `internal/authority/internal/domain/` | device-authority vocabulary and pure rules (reference module layer) |
| `internal/authority/internal/store/` | device-authority persistence: transactions and the only SQL for its tables (reference module layer) |
| `internal/interlock/` | durable readiness state and read-only assertions |
| `internal/canonicaljson/` | RFC 8785 canonical JSON and domain-separated digests: thin facade (public API only) |
| `internal/canonicaljson/internal/domain/` | pure canonical encoder, number and string rules, strict validation, digest and stored-document rules (reference module layer) |
| `internal/sources/` | deterministic digest, time, duration and identity primitives |
| `internal/executor/fixture/` | deterministic episode executor for explicit demo and replay fixtures |
| `internal/executor/native/`, `internal/executor/remote/`, `internal/worker/` | in-process and out-of-process executors and the worker protocol (facade) |
| `internal/worker/internal/domain/` | pure protocol constants, limits, handshake, request and stream validation, budget rule |
| `internal/worker/internal/transport/` | reference worker gRPC server and the private Unix sockets |
| `internal/testsupport/executorconformance/` | test support: the semantic contract every episode executor must meet |
| `internal/runartifact/` | bounded operational evidence (soak verdict) and immutable artifact verification; facade, `internal/app`, pure `internal/domain`, read-only `internal/store`, artifact directory in `internal/transport` |
| `internal/replay/` | effect-safe replay modes |
| `internal/replay/internal/app/` | replay sessions: epoch derivation, ingestion, engine runs and capability phases |
| `internal/replay/internal/domain/` | pure replay verification rules and vocabulary (reference module layer) |
| `internal/replay/internal/store/` | replay SQL and transactions against the isolated database (reference module layer) |
| `internal/replay/internal/transport/` | trace files, isolated databases and trace ingestion (reference module layer) |
| `internal/runtime/` | thin live-pipeline, readiness and worker facades; [module guide](../../internal/runtime/README.md) |
| `internal/storage/` | SQLite infrastructure, migrations and transactions |
| `internal/contractsv1/` | versioned envelope/schema contracts (facade) |
| `internal/contractsv1/internal/domain/` | pure envelope, CloudEvent, trace context, schema validation and digest rules |
| `internal/telemetry/` | OpenTelemetry and runtime metrics (facade) |
| `internal/telemetry/internal/domain/` | pure runtime counters, latency histogram and percentiles |
| `internal/telemetry/internal/transport/` | Prometheus metrics handler and OpenTelemetry tracer provider, spans and links |
| `internal/api/` | HTTP health, controls and SSE delivery (facade) |
| `internal/api/internal/domain/` | pure problem document, approval decoding, bearer matching, SSE frames, cursors and failure mapping |
| `internal/api/internal/transport/` | net/http handlers: health, controls, approvals and the SSE stream |
| `internal/notify/` | durable notification outbox facade: transactional append, paged reads, retention |
| `internal/notify/internal/app/` | append, lifecycle append, page read with resume, lag and poison handling, prune |
| `internal/notify/internal/domain/` | lifecycle contract (schema and binding), sealing, dedupe, resume, poison and retention rules |
| `internal/notify/internal/store/` | the only SQL for notifications, cursors, tombstones, poison attempts and audits |
| `proto/agenticstream/runtime/v1/` | generated current-v1 Go protocol |
| `migrations/` | ordered SQLite schema changes |
| `examples/` | trace fixtures and predictive-maintenance evidence |
| `docs/design/` | current full design record, contracts, examples, plans |
| `docs/design-v0/`, `docs/design-v0.1/` | archived design iterations |
| `docs/research/` | research and generated working artifacts |
| `documentation/` | curated public documentation |

## Data ownership

Event schemas, simulator channel mappings, and the aquaculture intent catalog
are JSON data, not Go literals. Their tests pin the expected digest or parity
where the data is a cross-repo contract.

## Why `internal/` is intentional

The runtime packages are not a stable public Go SDK. Keep implementation
contracts under `internal` until their compatibility and ownership survive a
release. Public integration surfaces are the CLI, documented HTTP/SSE behavior,
versioned JSON contracts, and current-v1 worker protocol.

## Next reads

- [Architecture overview](overview.md)
- [Business modules and ownership](modules.md)
- [Contract index](../contracts/README.md)
- [Contributing](../../CONTRIBUTING.md)

Runtime reference layers:

| Package | Responsibility |
| --- | --- |
| `internal/runtime/internal/app/` | Ordered live-runtime use cases and process lifetimes |
| `internal/runtime/internal/composition/` | Concrete plane and adapter wiring |
| `internal/runtime/internal/domain/` | Pure runtime values and configuration rules |
| `internal/runtime/internal/store/` | Transaction plumbing through lifecycle owners |
| `internal/runtime/internal/transport/` | Source and worker connection adapters |
