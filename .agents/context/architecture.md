# Architecture Context

What the code cannot tell you on its own. The maintained package list is
[repository-map.md](../../documentation/architecture/repository-map.md) (kept
complete by `TestModuleMapListsEveryPackage`); business ownership is
[modules.md](../../documentation/architecture/modules.md); the enforced criteria
are [architecture-bar.md](architecture-bar.md).

## Where Authority Lives

- `Makefile` defines the local commands; `.github/workflows/ci.yml` and
  `.github/workflows/proto.yml` run them in CI.
- `.golangci.yml` and `.pre-commit-config.yaml` enforce code quality and hygiene.
- Runtime contracts are `proto/agenticstream/runtime/v1/runtime-v1.proto`,
  `internal/contractsv1/`, `internal/spec/internal/domain/schema.json` and `migrations/`.
- `documentation/governance/release-status.json` records the release posture.
- `examples/` holds fixtures shared by several modules; data private to one module
  lives in its `testdata/`.

## Data Flow

```text
ingress -> eventlog -> engine/operators -> situations -> cognition
   -> episodes -> evidence/decisions -> policy -> actions
```

## Deliberate Placements

- **No graph engine in the event hot path** — determinism and throughput come from the operator pipeline, not a graph executor.
- **No LLM invocation per event** — the cognitive scheduler decides when reasoning is useful; episodes are bounded and cheap to skip.
- **Policy/action plane separated from cognition** — models propose, policy disposes; effectors are only reachable through the action plane.
- **SQLite WAL via `modernc.org/sqlite`** — single-node first; scale-out is deferred until profiles show a hard node boundary.
- **Most Go packages stay under `internal`** until their contracts survive a release; public SDK packages hold client and authoring types only.

## Dependency Direction

- `cmd` -> `runtime`/`api`/`ingress`/`replay` -> stream/cognition/episode/policy/action planes -> `eventlog`/`storage`/`sources`/`contractsv1`
- Dependencies flow downward only.

Avoid:

- circular imports
- business logic in API handlers
- transport concerns leaking into the engine
- untrusted input reaching policy/actions without validation
- per-domain code branches (domains are data via SituationSpec)

## Shared Packages With Special Rules

- `internal/kernel` is the shared pure vocabulary: durable time text and digest
  text. Standard library only: no database, file, network, random source or clock
  read. Any production package may import it without an `allowedImports` edge
  (`TestKernelStaysPure`). A symbol is admitted only when two or more modules need
  it and it is a stable representation rule, not business behaviour. Clocks and
  identities stay in `internal/sources`, SQL in `internal/storage`.
- `internal/storage` is SQLite infrastructure with no business rules.
  `internal/storage/storagetest` is test support; production code never imports it.
- `internal/architecture` holds the repository-wide gates: tests only, no production
  code. It is the only place for tests that span modules; the repository root holds none.
- `runtime` and `cmd` alone wire concrete adapters; dispatch and device
  implementations meet through `actionport`. `episodes` never imports worker
  transport (rule A8).

## Adding a Module or Table

The gates fail on each step you miss; doing them up front is cheaper.

- A table: a migration in `migrations/` (edit the one that introduced the table when it already exists), its writer in `durableOwners` (`internal/architecture/ownership_test.go`), its SQL only in the owning `internal/store`.
- A module: the shape in [reference-module-refactor.md](../prompts/reference-module-refactor.md), its edges in `allowedImports` (`internal/architecture/import_rules_test.go`) and its layers in `facadeSpecs`, `opaqueStoreModules` and `applicationLayerSpecs`, a package comment in `doc.go`, a row in the repository map, and its `UBIQUITOUS_LANGUAGE.md`.

## Business Ownership (ADR-017)

Every production import is allowlisted and points to a strictly lower reviewed
level (`internal/architecture/imports_test.go`, tables in
`internal/architecture/package_layers_test.go`). Logical feedback reenters through
durable records/evidence; it does not create reverse service dependencies. Control
and cognition call ledger APIs using the original transaction.

`internal/architecture/ownership_test.go` pins all current production SQL mutation
owners. Shared handoffs are restricted by phase and update columns: episodes
produce intents, policy governs them and publishes commands/outbox, actions
consume the outbox, and cognition only marks the last reasoned Situation
version. Policy can discard only its exact pending prepared command before
outbox publication. No general foreign-table write permission exists.
