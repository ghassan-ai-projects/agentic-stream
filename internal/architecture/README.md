# Architecture gates

`internal/architecture` holds every repository-wide gate: a test that reads the
Go sources, configuration or documentation of the whole repository and fails
when a rule breaks. It has no production code (`doc.go` only) and nothing
imports it. The repository root holds no tests (rule T1 of the
[test audit bar](../../docs/test-audit-2026-10-09/TEST_BAR.md)); a gate that
spans modules lives here, in the file named for its subject. Tests that prove
one module's behavior live in that module.

Rules are cited by ID: Q = [quality bar](../../.agents/context/quality-bar.md),
A = [architecture bar](../../.agents/context/architecture-bar.md), T = test bar,
AGENTS = [AGENTS.md](../../AGENTS.md).

```bash
go test ./internal/architecture/...
```

The package parses the repository once (`loadRepository` in
`repository_test.go`) and every test runs in parallel over that snapshot.
Paths resolve from the repository root (the directory with `go.mod`), never
from this directory. A gate that names a package fails when the package has no
production files, so renaming a package cannot silently disable its gate.

## Rule tables

A change that the tables do not allow is a change to the table, reviewed with
the code.

| File | Table | Read by |
| --- | --- | --- |
| `import_rules_test.go` | `foundationPackages`, `forbiddenImports`, `allowedImports` | `TestPackageLayering`, `TestAllowedImportsHaveNoStaleEdges` |
| `package_layers_test.go` | `packageLayers` | `TestImportsOnlyPointToLowerArchitectureLayers` |
| `ownership_test.go` | `durableOwners` | `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` |
| `facades_test.go` | `facadeSpecs` | `TestFacadesOnlyDelegate` |
| `module_shape_test.go` | `moduleShapeExceptions`, `opaqueStoreModules`, `applicationLayerSpecs` | module shape gates |

A new module adds itself to `allowedImports`, `packageLayers`, `durableOwners`
(for tables it owns), and, once it has the reference shape, to `facadeSpecs`,
`opaqueStoreModules` and `applicationLayerSpecs`.

## Gates

### `repository_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestNoTestsAtRepositoryRoot` | T1 | No `_test.go` sits in the repository root. |

### `imports_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestPackageLayering` | Q5, A1 | Every production import is in `allowedImports`; foundation packages import no domain package; forbidden edges stay forbidden; nothing imports `cmd/`; no stale package entry. |
| `TestAllowedImportsHaveNoStaleEdges` | Q5, A1 | An approved edge that no file uses is removed, so the allowlist documents real dependencies. |
| `TestImportsOnlyPointToLowerArchitectureLayers` | A1 | Imports point to a strictly lower reviewed layer; no unclassified or stale layer. |
| `TestReasoningAndReplayCannotReachEffectImplementations` | A3, A5 | Reasoning, executors and replay reach no effect implementation, policy or composition root, directly or transitively; device adapters reach no upstream service. |
| `TestDependencyReachabilityIncludesIndirectAndCyclicPaths` | A5 | Self-test of the reachability search over indirect and cyclic graphs. |
| `TestContractPackagesExcludePersistenceAndTransport` | A2 | `actionport` and `contractsv1` import no database, network or storage. |
| `TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability` | A5 | Only `internal/control` builds the dispatch authorization callback. |
| `TestEpisodeLifecycleImportsNoExecutorTransport` | A8 | The episode lifecycle imports no worker protocol, gRPC or protobuf package. |

### `layers_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestDomainPackagesArePure` | A12 | Domain layers import no I/O, storage or store and read no clock. |
| `TestApplicationLayersDoNotTouchInfrastructure` | A12 | Application layers reach the database and network only through store or adapter layers. |
| `TestModuleSQLStaysInStore` | A12 | A module with a store layer keeps every SQL statement there. |
| `TestCompositionRootsContainNoSQL` | A10 | `runtime` and `cmd` contain no SQL. |
| `TestDeterministicLayersDoNotUseTimeOrRandomSources` | A12, AGENTS | Domain layers and the replay store take time and randomness as parameters. |

### `module_shape_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestEveryModuleHasAFacadeAndADomain` | A12 | Each module has an `internal/domain` layer unless listed with a reason. |
| `TestStoresKeepTransactionsOpaque` | A12 | Per module (subtest): `Store` and `Tx` are structs with private fields, never aliases of a database handle. |
| `TestApplicationsUseOpaquePorts` | A12 | Per module (subtest): application layers call no raw `Exec`, `Query`, `Begin`, `Commit` or `Rollback`. |

### `facades_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestFacadesOnlyDelegate` | A12, AGENTS | Per module (subtest): every function of a facade package is a reviewed operation that only delegates to the private application or domain layer. |

### `module_specific_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestApprovalLedgerDoesNotImportTransport` | A4 | The approval ledger stays below the notification outbox; the caller supplies the publisher. |
| `TestDecisionCatalogKeepsAuthorityPrivate` | invariant 1 | The compiled intent catalog is an opaque struct with private fields. |
| `TestEvidenceRulesExcludeProtocolAndCodecs` | A12 | Evidence rules, use cases and store import no protocol or codec package. |
| `TestEpisodeApplicationCallsLedgerOnlyThroughPureHelpers` | A4 | The episode lifecycle calls only the pure helpers of the episode ledger. |

### `ownership_test.go`, `sql_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` | A4 | Every SQL mutation in production code is made by the table's owner, or by a reviewed handoff phase with the columns it may write. |
| `TestHandoffOwnershipRejectsAuthorityAndPayloadBypasses` | A4 | Self-test: foreign lifecycle writes and payload or status bypasses are refused. |
| `TestSQLMutationClassifierRecognizesOwnershipBoundaries` | A4 | Self-test: quoted identifiers, comments, CTEs, upserts and update columns classify correctly. |
| `TestInsertConflictClassifierDistinguishesIgnoreFromRewrite` | A4 | Self-test: ignoring a conflict differs from rewriting an existing row. |
| `TestSQLGateDistinguishesInsertStatementsFromErrorMessages` | A4, A10 | Self-test: error text such as `insert item: %w` is not SQL. |

### `kernel_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestKernelStaysPure` | AGENTS (kernel) | `internal/kernel` imports only the standard library, no database, file, network or random source, and never reads the clock or host time zone. |
| `TestKernelPurityCheckCatchesEveryBypass` | AGENTS (kernel) | Self-test: aliased, dot and blank imports and every clock selector are caught. |
| `TestKernelHasNoSubpackages` | AGENTS (kernel) | The kernel stays one package. |
| `TestDigestTextHasOneOwner` | AGENTS (kernel) | No production package spells the `sha256:` prefix; digests go through `kernel.EncodeDigest` and `DecodeDigest`. |

### `test_hygiene_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestTestsNeverSleep` | T5 | No `_test.go` file calls `time.Sleep` (under any import alias, or as a function value) unless `sleepAllowlist` names the file with a reason. The list is empty. |
| `TestSleepAllowlistHasNoStaleEntries` | T5 | An allowlist entry whose file no longer sleeps is removed. |
| `TestSleepDetectorCatchesEveryWayToCallTimeSleep` | T5 | Self-test: direct, aliased and function-value uses are caught; timers and other packages' `Sleep` are not. |

The other mechanical test rules (T6 parallel and isolated, T9 helpers) are
enforced by `paralleltest`, `tparallel`, `usetesting` and `thelper` in
`.golangci.yml` (`make lint`).

### `invariants_test.go`

| Test | Rule | Protects |
| --- | --- | --- |
| `TestEveryInvariantNamesTestsThatExist` | T12 | Each of the ten product invariants lists proving tests in `documentation/architecture/invariants.md`, and every listed test is declared in the file its link names. |
| `TestProvingTestsAreReadPerInvariantSection` | T12 | Self-test: links are assigned to the invariant section they sit in. |

### Other files

| File | Test | Rule | Protects |
| --- | --- | --- | --- |
| `timetext_test.go` | `TestTimestampTextHasOneOwner` | AGENTS (kernel) | No file, tests included, formats or parses an instant with `time.RFC3339Nano` or spells the stored layout. |
| `language_test.go` | `TestEveryModuleHasItsUbiquitousLanguage` | A1 | Each module carries a `UBIQUITOUS_LANGUAGE.md`. |
| `documentation_test.go` | `TestEveryPackageDocumentsItsResponsibility` | A9 | Each production package has a package comment. |
| `documentation_test.go` | `TestModuleMapListsEveryPackage` | A9 | `documentation/architecture/repository-map.md` lists every production package. |
| `quality_test.go` | `TestProductionFileSize` | Q3 | Production files stay under 300 lines; generated files are exempt. |
| `quality_test.go` | `TestProductionFunctionLengthBar` | Q2 | `.golangci.yml` keeps `funlen` at 15 lines and 15 statements. |
| `quality_test.go` | `TestGeneratedProtocolImportsStayGeneratorOwned` | Q6 | The `goimports` hook leaves generated protocol files to the generator. |
| `module_path_test.go` | `TestModulePathSingleSourceOfTruth` | AGENTS (module path) | The few literal copies of the module path match `go.mod`, and the Makefile derives it. |
