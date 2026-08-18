# Architecture Context

## Repository Architecture

The authoritative architecture is [docs/design/TECHNICAL_DESIGN.md](../../docs/design/TECHNICAL_DESIGN.md). This file is the concise version agents should load before editing.

## Current Structure

- `AGENTS.md` is the canonical agent entrypoint.
- `README.md` introduces the project.
- `Makefile` defines the authoritative local commands.
- `.github/workflows/ci.yml` defines CI parity for core checks.
- `.golangci.yml` and `.pre-commit-config.yaml` enforce code quality and hygiene.
- `documentation/` holds the curated public documentation; `docs/` holds the classified design/evidence archive. Code, tests, embedded schemas, migrations, and the worker proto are the authority for implemented behavior.

## Target Structure (per design §23)

- `cmd/agentic-stream/`: entrypoint, flags, wiring, shutdown
- `internal/contractsv1` · `internal/spec` · `internal/ingress` · `internal/eventlog` · `internal/engine` · `internal/operators` · `internal/situations` · `internal/cognition` · `internal/episodes` · `internal/evidence` · `internal/decisions` · `internal/policy` · `internal/actions` · `internal/replay` · `internal/api` · `internal/telemetry` · `internal/storage` · `internal/clock` · `internal/notify` · `internal/eventschema` · `internal/runtime`
- `proto/agenticstream/runtime/v1/`: worker protocol (Protobuf/gRPC over UDS)
- `internal/spec/schema.json` · `internal/contractsv1/schemas/v1/` · `migrations/` · `examples/predictive-maintenance/`

## Data Flow

```text
ingress -> eventlog -> engine/operators -> situations -> cognition
   -> episodes -> evidence/decisions -> policy -> actions
```

- Raw events are evidence, never executable instructions.
- Event time, watermark, completeness, and late-data status are explicit.
- A Situation version is immutable after publication.
- Deterministic state changes are serial per virtual partition.
- Every episode is bound to one immutable Situation snapshot and a finite budget.
- A model can read evidence and propose typed intents; it cannot execute effects.
- Policy revalidates every intent against current state immediately before dispatch.
- Replay never performs external effects unless an explicit, separate simulation mode is selected.

## Deliberate Placements

- **No graph engine in the event hot path** — determinism and throughput come from the operator pipeline, not a graph executor.
- **No LLM invocation per event** — the cognitive scheduler decides when reasoning is useful; episodes are bounded and cheap to skip.
- **Policy/action plane separated from cognition** — models propose, policy disposes; effectors are only reachable through the action plane.
- **SQLite WAL via `modernc.org/sqlite`** — single-node first; scale-out is deferred until profiles show a hard node boundary.

## Dependency Direction

- `cmd` -> `runtime`/`api`/`ingress`/`replay` -> stream/cognition/episode/policy/action planes -> `eventlog`/`storage`/`clock`/`contractsv1`
- Dependencies flow downward only.
- Most Go packages stay under `internal` until their contracts survive a release.

Avoid:

- circular imports
- business logic in API handlers
- transport concerns leaking into the engine
- untrusted input reaching policy/actions without validation
- per-domain code branches (domains are data via SituationSpec)
