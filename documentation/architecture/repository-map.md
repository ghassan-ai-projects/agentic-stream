# Repository map

This is the current implementation map. It intentionally differs from the
historical design map where the code has chosen a more specific package name.

| Path | Responsibility |
| --- | --- |
| `cmd/agentic-stream/` | CLI entrypoint and runtime wiring |
| `internal/spec/` | SituationSpec schema, compiler, CEL, deployment persistence |
| `internal/ingress/` | normalized/simulator JSONL files and live normalized JSONL Unix socket |
| `internal/eventlog/` | append-only events, validation, dedup, quarantine, gaps |
| `internal/eventlog/internal/domain/` | pure evidence admission, quarantine identity and decode rules (reference module layer) |
| `internal/eventlog/internal/app/` | append, quarantine, release/redrive and read use cases |
| `internal/eventlog/internal/store/` | event-log SQL and units of work (reference module layer) |
| `internal/engine/` | deterministic partitioned stream processing |
| `internal/operators/` | aggregates, slopes, missing-heartbeat and related features |
| `internal/situations/` | Situation state and immutable versions |
| `internal/cognition/` | scheduler, trigger evaluation, reconsideration |
| `internal/admission/` | episode admission: due scheduler items to epoch-stamped episodes, or recorded skips |
| `internal/episodes/` | request assembly, bounded execution lifecycle, budgets and the `Executor` port |
| `internal/episodes/internal/domain/` | pure episode rules: budgets, snapshot evidence, failure classification, decision digests |
| `internal/evidence/` | capability tokens, evidence server, call ledger |
| `internal/decisions/` | Decision and Intent validation |
| `internal/policy/` | policy configuration and delegation facade |
| `internal/policy/internal/app/` | ordered intent evaluation and human approval use cases |
| `internal/policy/internal/store/` | caller-owned transactions, policy SQL and ledger/notification plumbing |
| `internal/policy/internal/domain/` | pure policy records, canonical definitions and governance checks |
| `internal/actions/` | governed outbox dispatch, verification and reconciliation |
| `internal/watch/` | derived-trigger watches: install as an effect, fire on matching evidence, expire |
| `internal/actionport/` | approved command/effect contracts |
| `internal/device/` | device effect boundary facade: effect profiles, catalog loading, gateway dial and effector constructors |
| `internal/device/internal/app/` | device session use cases and the gateway, simulated and fail-closed effectors |
| `internal/device/internal/wire/` | device record codec: schema-validated canonical NDJSON, typed records and original evidence documents |
| `internal/device/internal/transport/` | Unix-socket gateway link: framing, deadlines and the may-have-sent signal |
| `internal/device/internal/domain/` | device-boundary rules: capability catalog, materialization, effect profiles, record matching, output verification |
| `internal/episodeledger/` | durable episode/attempt lifecycle, fencing and recovery |
| `internal/scheduleledger/` | durable queue identity, admission and coalescing |
| `internal/approvalledger/` | human approval lifecycle and supersession notification |
| `internal/control/` | runtime ownership, epoch control and readiness capability |
| `internal/authority/` | device target claims, bindings, reconciliation and safety evidence: public API only (configuration and delegation) |
| `internal/authority/internal/app/` | device-authority use cases: validation, admission, unit of work, audit (reference module layer) |
| `internal/authority/internal/domain/` | device-authority vocabulary and pure rules (reference module layer) |
| `internal/authority/internal/store/` | device-authority persistence: transactions and the only SQL for its tables (reference module layer) |
| `internal/qualification/` | calibration and shadow evidence |
| `internal/costcontrol/` | reservation, bounded usage and settlement |
| `internal/interlock/` | durable readiness state and read-only assertions |
| `internal/notifycontract/` | versioned notification names and metadata |
| `internal/canonicaljson/`, `internal/clock/`, `internal/duration/`, `internal/ids/` | deterministic digest, time, duration and identity primitives |
| `internal/executor/native/`, `internal/executor/remote/`, `internal/executor/conformance/`, `internal/worker/` | in-process and out-of-process executors, executor qualification and worker protocol transport |
| `internal/soak/`, `internal/runartifact/` | bounded operational evidence and immutable artifact verification |
| `internal/replay/` | effect-safe replay modes |
| `internal/replay/internal/app/` | replay sessions: epoch derivation, ingestion, engine runs and capability phases |
| `internal/replay/internal/domain/` | pure replay verification rules and vocabulary (reference module layer) |
| `internal/replay/internal/store/` | replay SQL and transactions against the isolated database (reference module layer) |
| `internal/replay/internal/transport/` | trace files, isolated databases and trace ingestion (reference module layer) |
| `internal/runtime/` | thin live-pipeline, readiness and worker facades; [module guide](../../internal/runtime/README.md) |
| `internal/storage/` | SQLite infrastructure, migrations and transactions |
| `internal/contractsv1/` | versioned envelope/schema contracts |
| `internal/telemetry/` | OpenTelemetry and runtime metrics |
| `internal/api/` and `internal/notify/` | HTTP health, controls and SSE delivery; the durable notification outbox |
| `internal/eventschema/` | data-driven event schema registry |
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
