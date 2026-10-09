# architecture (repository root gates)

Status: done
Round: 1

The 29 test files in the repository root (package `agenticstream`) moved into
`internal/architecture`. The root now holds no `_test.go` file (T1) and no Go
package: `doc.go` is deleted. `tools.go` (build tag `tools`) stays.

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `.` (root) → `internal/architecture` | no statements | no statements | 2.8 s (3.9 s in BASELINE) | 1.6 s | 74 top-level, 99 passing (sub)tests | 39 top-level, 104 passing (sub)tests |

Without `-race`: 0.50 s → 0.16 s. Every top-level test and subtest is parallel
and the repository is parsed once for the whole package.

Files: 29 root test files (~2 990 lines) → 16 test files, `doc.go`, `README.md`
and `UBIQUITOUS_LANGUAGE.md` in `internal/architecture` (largest test file:
`sql_test.go`, 286 lines; the two rule-table files are data only).

## Layout

| File | Content |
| --- | --- |
| `repository_test.go` | shared repository snapshot (parsed once), helpers, `TestNoTestsAtRepositoryRoot` |
| `import_rules_test.go` | `foundationPackages`, `forbiddenImports`, `allowedImports` |
| `package_layers_test.go` | `packageLayers` |
| `imports_test.go` | layering, stale edges, layers, reachability, contract packages, final authorization, A8 |
| `layers_test.go` | domain purity, application vs infrastructure, SQL in store, composition roots, deterministic layers |
| `facades_test.go` | `facadeSpecs` table and `TestFacadesOnlyDelegate` |
| `module_shape_test.go` | domain-layer exceptions, `TestStoresKeepTransactionsOpaque`, `TestApplicationsUseOpaquePorts` |
| `module_specific_test.go` | the four checks that belong to one module |
| `ownership_test.go`, `sql_test.go` | `durableOwners`, mutation ownership, SQL lexer/classifier and self-tests |
| `kernel_test.go`, `timetext_test.go`, `language_test.go`, `documentation_test.go`, `quality_test.go`, `module_path_test.go` | as named |

## Findings and changes

### Removed
- Nothing was dropped. 37 top-level tests became subtests of three table-driven
  tests (below); `architecture_sql_conflict_test.go` merged into `sql_test.go`.

### Renamed or moved

Every old top-level test and where it lives now. "same" means the same name in
the new package.

| Old test (root file) | New test |
| --- | --- |
| `TestActionsFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/actions` |
| `TestApprovalLedgerFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/approvalledger` |
| `TestCanonicalJSONFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/canonicaljson` |
| `TestCognitionFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/cognition` |
| `TestControlFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/control` |
| `TestDecisionFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/decisions` |
| `TestEngineFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/engine` |
| `TestEpisodeLedgerFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/episodeledger` |
| `TestEpisodeFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/episodes` |
| `TestEvidenceFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/evidence` |
| `TestIngressFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/ingress` |
| `TestNativeExecutorFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/executor_native` |
| `TestNotifyFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/notify` |
| `TestRemoteExecutorFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/executor_remote` |
| `TestRunArtifactFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/runartifact` |
| `TestWatchFacadeOnlyDelegates` | `TestFacadesOnlyDelegate/watch` |
| `TestActionsStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/actions` |
| `TestApprovalLedgerStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/approvalledger` |
| `TestCognitionStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/cognition` |
| `TestControlStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/control` |
| `TestEngineStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/engine` |
| `TestEpisodeLedgerStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/episodeledger` |
| `TestEpisodeStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/episodes` |
| `TestEvidenceStoreKeepsInfrastructurePrivate` | `TestStoresKeepTransactionsOpaque/evidence` |
| `TestIngressStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/ingress` |
| `TestNotifyStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/notify` |
| `TestRunArtifactStoreKeepsSnapshotOpaque` | `TestStoresKeepTransactionsOpaque/runartifact` |
| `TestWatchStoreKeepsTransactionsOpaque` | `TestStoresKeepTransactionsOpaque/watch` |
| `TestActionsApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/actions` |
| `TestApprovalLedgerApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/approvalledger` |
| `TestCognitionApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/cognition` |
| `TestControlApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/control` |
| `TestEngineApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/engine` |
| `TestEpisodeLedgerApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/episodeledger` |
| `TestEpisodeApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/episodes` (raw calls) and `TestEpisodeApplicationCallsLedgerOnlyThroughPureHelpers` (ledger calls) |
| `TestEvidenceApplicationUsesOpaquePorts` | `TestApplicationsUseOpaquePorts/evidence` (no `Query`: its service defines a `Query` method, as before) |
| `TestIngressApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/ingress` |
| `TestNotifyApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/notify` |
| `TestRunArtifactApplicationUsesOpaquePorts` | `TestApplicationsUseOpaquePorts/runartifact` |
| `TestWatchApplicationUsesTransactionalPorts` | `TestApplicationsUseOpaquePorts/watch` |
| `TestApprovalLedgerDoesNotImportTransport` | same (`module_specific_test.go`) |
| `TestDecisionCatalogKeepsAuthorityPrivate` | same (`module_specific_test.go`) |
| `TestEvidenceRulesExcludeProtocolAndCodecs` | same (`module_specific_test.go`) |
| `TestDigestTextHasOneOwner` | same (`kernel_test.go`) |
| `TestKernelStaysPure`, `TestKernelPurityCheckCatchesEveryBypass`, `TestKernelHasNoSubpackages` | same (`kernel_test.go`) |
| `TestPackageLayering`, `TestAllowedImportsHaveNoStaleEdges` | same (`imports_test.go`) |
| `TestImportsOnlyPointToLowerArchitectureLayers`, `TestReasoningAndReplayCannotReachEffectImplementations`, `TestDependencyReachabilityIncludesIndirectAndCyclicPaths`, `TestContractPackagesExcludePersistenceAndTransport`, `TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability` | same (`imports_test.go`) |
| `TestEpisodeLifecycleImportsNoExecutorTransport` | same (`imports_test.go`) |
| `TestDomainPackagesArePure`, `TestApplicationLayersDoNotTouchInfrastructure`, `TestModuleSQLStaysInStore`, `TestDeterministicLayersDoNotUseTimeOrRandomSources`, `TestCompositionRootsContainNoSQL` | same (`layers_test.go`) |
| `TestEveryModuleHasItsUbiquitousLanguage` | same (`language_test.go`) |
| `TestEveryModuleHasAFacadeAndADomain` | same (`module_shape_test.go`) |
| `TestEveryPackageDocumentsItsResponsibility`, `TestModuleMapListsEveryPackage` | same (`documentation_test.go`) |
| `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase`, `TestHandoffOwnershipRejectsAuthorityAndPayloadBypasses` | same (`ownership_test.go`) |
| `TestSQLMutationClassifierRecognizesOwnershipBoundaries`, `TestSQLGateDistinguishesInsertStatementsFromErrorMessages`, `TestInsertConflictClassifierDistinguishesIgnoreFromRewrite` | same (`sql_test.go`) |
| `TestProductionFileSize`, `TestProductionFunctionLengthBar`, `TestGeneratedProtocolImportsStayGeneratorOwned` | same (`quality_test.go`) |
| `TestTimestampTextHasOneOwner` | same (`timetext_test.go`; `timestampGateFile` now `internal/architecture/timetext_test.go`) |
| `TestModulePathSingleSourceOfTruth` | same (`module_path_test.go`) |

74 old tests, all mapped.

### Improved
- T1: `TestNoTestsAtRepositoryRoot` (new) fails when a `_test.go` sits in the root.
- T6: every test and subtest is parallel; `TestSQLGateDistinguishesInsertStatementsFromErrorMessages` lacked `t.Parallel()`, the classifier subtests lacked it too.
- T7: the repository is parsed once (`sync.OnceValues`, files parsed concurrently, comments included) and shared; the tests that each walked and re-parsed the tree now read one snapshot. Test files are parsed only for the timestamp gate, on first use.
- T4: failure messages state the offending file, package or call; the `dependencyPath` self-test prints got and want.
- Per-module facade checks are one table (`facadeSpecs`) with named operations and a check per operation (`to("app", ...)`, `onMethods`, `onFunctions`, `singleStatement`, `reachesApp`); the same strictness per module as before, including the receiver rule of `episodes` (methods `Assemble`, `Persist`, `RunOnce`; functions `Decisions`, `CompileIntentCatalog`), the one-statement rule of `internal/executor/native`, the app-selector rule of `internal/executor/remote`, the `*Event` domain rule of `notify`, the exempt names of `control`, and the per-name target layer of `evidence` and `episodeledger`.
- Vacuous-gate guard: a facade, store, application or contract gate now fails when the package it names has no production files (before, a renamed package silently passed). This is stricter, not looser.
- Shared helpers: the copies of "walk, parse, find files in directory" are one `repository`; `repoRoot` (a `runtime.Caller` path) is replaced by the directory of the working directory's `go.mod`, so the gates also work when the package moves.

### Added
- `TestNoTestsAtRepositoryRoot`: T1.
- `TestEpisodeApplicationCallsLedgerOnlyThroughPureHelpers`: the ledger half of the old `TestEpisodeApplicationUsesTransactionalPorts`, separate because it is episodes-specific.
- `internal/architecture/README.md` (gate index by file with rule IDs), `UBIQUITOUS_LANGUAGE.md` (required by `TestEveryModuleHasItsUbiquitousLanguage`), `doc.go`.

### Speed
- Before: most of the 74 tests walked and parsed the repository again (`productionGoFiles` + `parser.ParseFile` per test); 2.8 s with `-short -race`.
- After: one parse; 1.6 s with `-short -race`, 0.16 s without. Wall time is now dominated by the single parse under the race detector.

## Registration of the new package
- `moduleShapeExceptions` (reason: tests only), `allowedImports` (`{}`), `packageLayers` (0), `documentation/architecture/repository-map.md`, `UBIQUITOUS_LANGUAGE.md`, package comment in `doc.go`.
- Root `doc.go` deleted: nothing needed a root package (Makefile `PKGS` and `MAIN_PKGS` use `./...`, deadcode and govulncheck use `./...`, CI and pre-commit use `./...`; the gates skipped package `.`). `tools.go` stays. The stale Makefile comment that said `doc.go` anchors a root package is rewritten.

## Live references updated
`AGENTS.md` (architecture overview entry, module-path gate path, `allowedImports` location, gates location, "make test" line), `.agents/context/architecture.md`, `.agents/context/architecture-bar.md` (A12 evidence names), `.agents/context/go-style.md`, `.agents/context/quality-bar.md` (Q3, Q5 and the allowlist rule), `.agents/context/testing.md` (new "Repository-Wide Gates" section), `.agents/prompts/reference-module-refactor.md`, `documentation/architecture/modules.md` (three links), `documentation/architecture/repository-map.md`, `.pre-commit-config.yaml` (comment), `Makefile` (comment). `.claude/skills/**`, `scripts/`, `.github/workflows/` and `.golangci.yml` named none of the old files. `docs/` (dated archive) is untouched.

## Mutation check

Each violation was added, the gate run, and the change removed; `git status --short` was identical before and after.

| Violation | Gate that failed |
| --- | --- |
| `internal/eventlog/internal/domain` imports `internal/actions` | `TestPackageLayering`, `TestImportsOnlyPointToLowerArchitectureLayers`, `TestReasoningAndReplayCannotReachEffectImplementations` |
| `internal/watch` gains an unreviewed function that does more than delegate | `TestFacadesOnlyDelegate/watch` |
| `internal/engine` `RunGlobal` gets a second statement | `TestFacadesOnlyDelegate/engine` |
| `internal/ingress` gains an unreviewed delegating method | `TestFacadesOnlyDelegate/ingress` |
| 306-line production file in `internal/kernel` | `TestProductionFileSize` |
| `zz_mutation_test.go` in the repository root | `TestNoTestsAtRepositoryRoot` |
| `internal/cognition/internal/app` calls `Exec` | `TestApplicationsUseOpaquePorts/cognition` |
| `internal/evidence/internal/store` declares `type Tx = struct{}` | `TestStoresKeepTransactionsOpaque/evidence` |
| `internal/kernel` calls `time.Now()` | `TestKernelStaysPure` |
| `internal/actions` facade holds `UPDATE episodes SET ...` | `TestModuleSQLStaysInStore`, `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` |
| new package `internal/zzmut` without comment, allowlist or map entry | `TestEveryPackageDocumentsItsResponsibility`, `TestPackageLayering`, `TestModuleMapListsEveryPackage` |
| `time.RFC3339Nano` in a test file under `internal/storage` | `TestTimestampTextHasOneOwner` |

## Production code touched
- Root `doc.go`: deleted (anchored the root package only while scaffolding).
- New `internal/architecture/doc.go`: package comment only, no code.

## Invariants proven here
- 6 (a model cannot execute effects): `TestReasoningAndReplayCannotReachEffectImplementations`, `TestDecisionCatalogKeepsAuthorityPrivate`, `TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability`.
- 9 (replay performs no external effects): `TestReasoningAndReplayCannotReachEffectImplementations`.
- 8 (stable identities, one writer per table): `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase`.
- The invariant-to-test map in `documentation/architecture/invariants.md` is the final round's job (T12).

## Open items
- The 37 per-module checks are now subtests, so `go test -run TestActionsFacadeOnlyDelegates` no longer exists; use `-run 'TestFacadesOnlyDelegate/actions'`.
- `docs/` still names the old root files in dated plans and audits, by design.
