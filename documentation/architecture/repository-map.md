# Repository map

This is the current implementation map. It intentionally differs from the
historical design map where the code has chosen a more specific package name.

| Path | Responsibility |
| --- | --- |
| `cmd/agentic-stream/` | CLI entrypoint and runtime wiring |
| `internal/spec/` | SituationSpec schema, compiler, CEL, deployment persistence |
| `internal/ingress/` | normalized and simulator JSONL adapters |
| `internal/eventlog/` | append-only events, validation, dedup, quarantine, gaps |
| `internal/engine/` | deterministic partitioned stream processing |
| `internal/operators/` | aggregates, slopes, missing-heartbeat and related features |
| `internal/situations/` | Situation state and immutable versions |
| `internal/cognition/` | scheduler, trigger evaluation, reconsideration |
| `internal/episodes/` | requests, lifecycle, workers, budgets, fencing, recovery |
| `internal/evidence/` | capability tokens, evidence server, call ledger |
| `internal/decisions/` | Decision and Intent validation |
| `internal/policy/` | deterministic governance and approvals |
| `internal/actions/` | outbox dispatcher and effectors |
| `internal/replay/` | effect-safe replay modes |
| `internal/runtime/` | service, pipeline, ownership, recovery, worker composition |
| `internal/storage/` | SQLite wrapper and runtime state |
| `internal/contractsv1/` | versioned envelope/schema contracts |
| `internal/telemetry/` | OpenTelemetry and runtime metrics |
| `internal/api/` and `internal/notify/` | HTTP, health, SSE, notifications, controls |
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
- [Contract index](../contracts/README.md)
- [Contributing](../../CONTRIBUTING.md)
