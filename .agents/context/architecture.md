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
- `internal/contractsv1` · `internal/spec` · `internal/ingress` · `internal/eventlog` · `internal/engine` · `internal/operators` · `internal/situations` · `internal/cognition` · `internal/episodes` · `internal/evidence` · `internal/decisions` · `internal/policy` · `internal/actions` · `internal/replay` · `internal/api` · `internal/telemetry` · `internal/storage` (and `internal/storage/storagetest`, migrated temporary databases for tests only) · `internal/kernel` (pure shared vocabulary) · `internal/notify` · `internal/eventschema` · `internal/runtime`
- `proto/agenticstream/runtime/v1/`: worker protocol (Protobuf/gRPC over UDS)
- `internal/spec/internal/domain/schema.json` · `internal/contractsv1/internal/domain/schemas/v1/` · `migrations/` · `examples/predictive-maintenance/`

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

## Business ownership refinement (ADR-017)

Additional current modules: `actionport`, `device`, `episodeledger`
(scheduler queue and episode lifecycle), `approvalledger`, `control`, `authority`,
and `executor/remote` (the streamed worker adapter behind the episode
`Executor` port; `episodes` never imports worker transport, rule A8).
Their contracts are in [architecture-bar.md](architecture-bar.md); the full
maintainer map is [business modules](../../documentation/architecture/modules.md).
`storage` is SQLite infrastructure. `internal/kernel` is the shared pure
vocabulary (durable time text, digest text): standard library only, importable
by any production package without a declared edge, kept pure by
`TestKernelStaysPure`; clocks and ids stay in `sources`, SQL in `storage`.
`internal/storage/storagetest` is test support that opens migrated temporary
databases; production code never imports it. `runtime` and `cmd` alone wire concrete
adapters; dispatch and device implementations meet through `actionport`.

Every production import is allowlisted and points to a strictly lower reviewed
level (`architecture_flow_test.go`). Logical feedback reenters through durable
records/evidence; it does not create reverse service dependencies. Control and
cognition call ledger APIs using the original transaction.

`architecture_ownership_test.go` pins all current production SQL mutation
owners. Shared handoffs are restricted by phase and update columns: episodes
produce intents, policy governs them and publishes commands/outbox, actions
consume the outbox, and cognition only marks the last reasoned Situation
version. Policy can discard only its exact pending prepared command before
outbox publication. No general foreign-table write permission exists.
