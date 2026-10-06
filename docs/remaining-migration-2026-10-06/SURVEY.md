# Survey of packages not yet on the reference-module shape

Method: layer folders per package (`internal/app|domain|store`), production
line counts, SQL and network use, production and test importers, owner tables in
`architecture_ownership_test.go`, and the layer table in
`architecture_flow_test.go`. Shapes follow the playbook table: tables → facade ·
app · domain · store; external systems without tables → facade · app · domain ·
adapter; pure rules → single package. The merge rule from the
[package merge review](../package-merge-review-2026-10-06.md) applies: a layer
or package that would be tiny is merged instead.

## Decisions

| Package | Prod lines | Prod importers | What it is | Decision |
| --- | --- | --- | --- | --- |
| `executor/native` | 1387 | 2 | Model loop, tools, OpenAI-compatible provider over HTTP, deterministic provider, artifact store, benchmark batch runner | **Migrate** (adapter shape). Delete `batch.go` (no caller); move `MemoryArtifactStore` to test support |
| `executor/remote` | 1143 | 1 | Streamed `EpisodeWorker` executor over gRPC: request building, budgets, capability checks, stream state | **Migrate** (adapter shape) |
| `runartifact` + `soak` | 1042 + 220 | 1 + 1 | Read-only run export, manifest, verification; soak verdict computed only for the export | **Merged** (`soak` is now unexported files in `runartifact`, commit `478882b`); **then migrate** (read-only module: facade · app · domain · store · transport) |
| `worker` | 685 | 4 | Protocol constants, UDS socket helpers (prod); reference `Server`, handshake and stream validation (only tests register it) | **Split**: reference server to `worker/workertest`; the remaining ~250 lines stay one package (below the size where layers pay off) |
| `spec` | 1014 | 34 | Compiler, schema, references (pure) plus `deployments.go` (131 lines) with SQL on `spec_deployments` | **Decided: no.** A store layer for one 131-line writer is the tiny layer the merge rule rejects; `deployments.go` stays beside the compiler it persists |
| `eventschema` | 183 | 3 | Embedded registry (pure) plus `storage.go`, the only writer of `event_schemas` | **Decided: no merge.** `Register` has one production caller (`spec`) but four test seeders in `eventlog` and `ingress`, `eventlog` reads the table, and `spec` (layer 2) may not import `eventlog` (layer 3). It stays the layer-1 owner of the table |
| `contractsv1` | 580 | 69 | Envelopes and schemas; `conformance.go` holds six test-only fixture functions | **Move `conformance.go`** to a `contractsv1/contractstest` support package; rest stays |
| `executor/conformance` | 81 | 0 | Shared executor contract harness, imported by 7 tests | **Keep** as test support; documented as such |
| `executor/fixture` | 124 | 1 | Deterministic fixture executor used by composition and 17 tests | **Keep** |
| `interlock` | 61 | 11 | Layer-0 reader of `runtime_interlock` (and an unused writer) | **Keep** (merge into `control` still rejected: policy, actions and watch would inherit control's imports). Decide the trip path, see DEADCODE |
| `situations`, `operators` | 719, 1011 | 16, 9 | Pure rule sets | **Leave**: no I/O, already under lint limits; merging them couples two unrelated rule sets |
| `duration` | 43 | 6 | Duration parser | **Leave**: foundation leaf. Merging into `spec` would add a spec import to `episodes/internal/domain/budget.go` |
| `api` | 705 | 3 | HTTP/SSE handlers | **Leave** (decided earlier: thin adapter, no tables) |
| `actionport`, `clock`, `ids`, `storage`, `telemetry` | — | 11–48 | Contracts and infrastructure | **Leave** |

No package is deleted outright: every one has a production importer except
`executor/conformance`, which is test support by design.

## Evidence

- `executor/native`: no SQL. Production code outside the package uses `New`,
  `Config`, the provider/tool types and `DeterministicProvider`. `RunBatch` and
  `RunBatchJSON` serve an external benchmark harness (`go_native_executor`
  baseline) and have no caller in this repository or the CLI.
- `executor/remote`: imports `worker` only for constants (`ProtocolVersion`,
  `EvidenceToolsFeature`, `DefaultMax*`), `ValidateBudget` and
  `ValidateEvidenceSocketPath`. It does not use `worker.Server`.
- `worker`: production uses are `ListenEvidenceSocket` and
  `DialEpisodeWorkerSocketTLS` (runtime transport), plus the constants above.
  `Server`, `Execute`, `Handshake` and `validation.go` (34 unreachable
  symbols) are registered only in `executor/remote` and conformance tests. They act as the protocol reference for the separate worker
  process (see [worker boundary](../../documentation/architecture/worker-boundary.md)),
  so they are kept, as test support.
- `runartifact`/`soak`: `soak` has exactly one production importer
  (`runartifact/export_snapshot.go`). `soak.Compute` and `soak.ComputeTx` are
  never called; export uses `ComputeTenantTx`. Both read tables they do not own
  (`soak` reads device authority through `authority.ReadSafetyRecord`).
- `spec` and `eventschema`: `eventschema.Register` has one production caller,
  `spec.registerInputSchemas`, but also four test callers in
  `eventlog` and `ingress` that seed schemas. Folding it into a `spec` store would
  make those tests import `spec` internals or build whole deployments. Not worth it.

## Gates that change

For each migrated package, in the same commit as the code: `packageLayers` and
`allowedImports` (`architecture_test.go`), the layer table
(`architecture_flow_test.go`, executors are layer 10), `durableOwners` where a
table writer moves (`event_schemas`, `spec_deployments`), the
`UBIQUITOUS_LANGUAGE.md` gate, and
`documentation/architecture/repository-map.md`.
