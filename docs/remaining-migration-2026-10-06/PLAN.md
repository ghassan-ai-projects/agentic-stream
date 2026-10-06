# Plan

Order is by risk and by dependency: remove dead weight first so later rounds
move less code, then the small structural fixes, then the four layered
migrations, then the operator-lever wiring. Each round is one or more commits
with focused tests, `gofmt`, `golangci-lint run ./...`; `make ci-check` before
handoff (`proto-check` needs the pinned `protoc`). Never loosen a lint
threshold. Each layered module gets its own dated record folder and an
`UBIQUITOUS_LANGUAGE.md` in the package, as the playbook requires.

## Rounds

| Round | Scope | Proof |
| --- | --- | --- |
| 0 | This folder | Docs only |
| 1 | **Dead-code deletions** ([DEADCODE](DEADCODE.md)): engine per-partition path, native batch runner, authority wrappers, `episodes.CompileIntentCatalog`, `control.SetCostLimit` unexport | `deadcode ./...` drops 14 symbols; replaced tests still pass; engine golden replay unchanged |
| 2 | **Test support moves**: `worker/workertest`, `contractsv1/contractstest`, native `MemoryArtifactStore` | `deadcode` drops 43 symbols; architecture gate still forbids production imports of the support packages |
| 3 | **`spec` store**: extract `spec/internal/store` for deployments, absorb `eventschema.Register`, make `eventschema` pure; update `durableOwners` for `event_schemas` and `spec_deployments` | Deployment golden digests unchanged; SQL-in-store gate covers `spec`; `eventschema` has no `database/sql` import |
| 4 | **`runartifact` + `soak`**: merge `soak` in, then facade · app (export, verify) · domain (manifest, file digests, soak verdict) · store (read-only snapshot SQL) · transport (artifact directory) | Exported artifact byte-identical for a fixed database (golden), `Verify` rejects each tampered-file case as before, read-only store has no `Exec` |
| 5 | **`worker` + `executor/remote`**: `worker` keeps protocol constants, `ValidateBudget`, sockets; `executor/remote` becomes facade · app (stream session) · domain (request, budget, capability, event checks) · transport (gRPC stream) | `executor/conformance` passes against the streamed worker; error classification (`transport_error`) tests unchanged; worker-boundary gate still lists only executors as transport importers |
| 6 | **`executor/native`**: facade · app (model/tool loop) · domain (types, usage, retry classification, decision checks) · transport (OpenAI-compatible provider, stream parsing) | Conformance passes; usage and cost accounting identical; no network calls in unit tests (provider tests use `httptest`) |
| 7 | **Gates and docs**: extend architecture tests, prove each new gate by injecting a violation, update `AGENTS.md`, `.agents/context/architecture-bar.md`, `documentation/architecture/repository-map.md`, and the migration-status README | Each gate fails on an injected violation, then passes |
| 8 | **Operator levers** (after owner decisions): `replay --mode/--repeat`, `quarantine`, `interlock`, `RecordGap` resolution | Each CLI command has an end-to-end test; `deadcode` shows only the conformance harness |

Rounds 1 to 3 are independent of 4 to 6. Rounds 4, 5 and 6 are independent of
each other, except that round 5 follows round 2 (the reference worker moves
first). Round 8 does not block the others.

## Behavior that must not change

- Identity, digest and wire inputs: spec deployment digests, the exported run
  artifact bytes and `Verify` result, the worker protocol frames and the
  episode request documents.
- Error precedence and sentinels in the executors: retryable versus fatal
  classification, budget refusals, `UnknownOutcome` handling.
- Transactions: the deployment writer and event schema registration stay in one
  transaction; export stays one read-only snapshot.
- Fail-closed construction: a missing database, provider or store is a
  constructor error, never a skipped check.

## Deliberate behavior changes

| Change | Round | Why |
| --- | --- | --- |
| `soak.Compute`/`ComputeTx` removed; export uses the tenant variant only | 1, 4 | Never called |
| `control.SetCostLimit` unexported | 1 | Production writes ceilings through `ApplyCostCeilings` |
| `native.RunBatch*` removed | 1 | No caller; the benchmark harness belongs with its protocol |
| `eventschema.Register` becomes a spec-store method; `event_schemas` writer is `spec` | 3 | One writer, inside the deployment transaction |
| `worker.Server` moves to `worker/workertest` | 2 | Test-only; the public protocol is the proto, not this Go server |

## Owner decisions needed

1. **Operator levers** (DEADCODE, round 8): wire `replay` modes, quarantine
   release/redrive and the interlock trip, or delete them. Recommendation: wire
   all three; delete `RecordGap` only if ingress has no overflow path.
2. **Reference worker**: keep as `worker/workertest` (recommended: it is the
   conformance fixture for the separate worker process) or promote to a public
   SDK package. `internal/` prevents outside use today.
3. **Benchmark batch runner**: confirm the external harness no longer needs
   `native.RunBatch` (recommended: delete).

## Deferred (not part of this plan)

Owner-provided read ports (cross-module SQL reads, status folder DEFERRED #1),
typed enum values, transaction-scope decisions, and re-deriving the layer table
from the import graph. `runartifact` and `soak` read tables they do not own;
they join the read-port work rather than blocking it.

## Status

| Round | Status | Commit |
| --- | --- | --- |
| 0 Plan | Written, not committed | — |
| 1 Dead-code deletions | Not started | — |
| 2 Test support moves | Not started | — |
| 3 `spec` store | Not started | — |
| 4 `runartifact` + `soak` | Not started | — |
| 5 `worker` + `executor/remote` | Not started | — |
| 6 `executor/native` | Not started | — |
| 7 Gates and docs | Not started | — |
| 8 Operator levers | Blocked on owner decision | — |

Update the migration-status README and `deadcode-production-unreachable.txt`
(`deadcode ./... > ...`) as each round lands.
