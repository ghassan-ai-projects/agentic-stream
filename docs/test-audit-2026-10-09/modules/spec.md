# spec

Status: done
Round: 4

## Metrics

Measured with `go test -short -race -count=1 -cover -json`, other workers running. Package time is dominated by building and linking the test binary under `-race`, so before/after wall times are within noise; the per-compile cost is the real speed result (below).

| Package | Coverage before | after | Time before | after | Top-level tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/spec` | 81.2% | 93.8% | 1.82 s | 1.95 s | 6 | 7 |
| `internal/spec/internal/app` | 100% | 100% | 1.35 s | 1.56 s | 2 | 3 |
| `internal/spec/internal/domain` | 88.8% | 95.9% | 2.34 s | 1.91 s | 31 | 23 |
| `internal/spec/internal/store` | 68.9% | 86.5% | 1.83 s | 2.20 s | 6 | 12 |
| `internal/spec/spectest` | 71.4% | 85.7% | 1.48 s | 1.80 s | 1 | 3 |

Passing tests and subtests: 46 top-level + 111 subtests → 48 top-level + 140 subtests. Test files: 13 → 15. Hygiene lint (`paralleltest`, `tparallel`, `usetesting`, `thelper`): 53 findings across spec and ingress before the round (51 `paralleltest`, 2 `thelper`; the split by module was not recorded) → 0 in both. No `time.Sleep`. The remaining uncovered statements are unreachable error branches (`json.Marshal` of fixed shapes, the embedded-data decode panic, a YAML mapping with a non-scalar key's odd-length content).

## Findings and changes

### Removed
- `TestRotatingMachinerySchemasDescribeAdapterPayloads`, `TestPondDissolvedOxygenSchemaDescribesAdapterPayload`, `TestBaySchemasDescribeAdapterPayloads`, `TestZoneHumiditySchemaDescribesAdapterPayload` (`event_schema_test.go`): T10. They re-asserted values (event type, path, unit, optional flag) that `TestAllBuiltinsLoadFromData` already pins through the golden digest of the whole registry. The one thing they proved beyond the digest, the `required` array and untyped-number rendering of `EventSchemaJSON`, moved into `TestEventSchemaJSONEmitsEnumsAndRequiredFields`. `contains` helper deleted with them.
- Second half of `TestP8GraphVersionChangeIsAFreshNamespace` (store): it inserted a row into `situations` and `lineage_sets` (tables other modules own) and read it back, proving a SQL foreign key, not a spec behavior. What it claimed about spec (two versions are two deployments) stays, now with statuses asserted.
- `TestCELBoolAcceptsOnlyBooleans` from the facade test: T2, `CELBool` is a domain function; its test moved to `cel_test.go` and covers more cases.
- Comment blocks inside tests (T11, AGENTS.md no-comments rule), including the block in `experiment_contract_test.go`; its guidance (change a pin only with the named consumer, the EXPERIMENT_COMPATIBILITY path) now lives in the failure messages.

### Renamed or moved
- `internal/spec/internal/domain/parsing_test.go` → `validation_order_test.go` (T3): it tests the precedence of every compile stage, not `parsing.go`.
- `internal/spec/internal/store/p8_graph_version_test.go` deleted (T3, phase number in file name): its tests merged into `deployments_test.go` as `TestSameDigestRedeployKeepsTheDeploymentActive` and `TestNewSpecVersionRetiresThePreviousDeploymentOfTheSameTenantAndName`.
- `TestParse` → `TestParseDurationAcceptsGoUnitsAndWholeDays`; `TestCompilerReusePreservesJSONAndYAMLDigestParity` → `TestCompileGivesJSONAndYAMLSourcesTheSameDigest` (a `Compiler` no longer carries state); `TestJSONEmitsStringEnum` → `TestEventSchemaJSONEmitsEnumsAndRequiredFields`; `TestFacadeDurationsAndEventSchemas` → `TestFacadeExposesTheSpecVocabulary`; `TestFacadeCompilesAndDeploysASpec` + `TestLoadDeploymentReturnsTheDeployedSpec` → `TestFacadeCompilesDeploysAndLoadsASpec`.
- Merged: `TestCompilePredictiveMaintenance`, `TestCompileRotatingMachinery`, `TestCompileZoneThermal` → `TestCompileSealsTheExampleSpecs` (table); `TestCompileRejectsUndeclaredPayloadField`, `...PayloadUnitMismatch`, `...UnsupportedRuntimeSurface`, `...UnenforcedTopLevelControls`, `...UnknownOperatorOutput`, `...DuplicateYAMLKey`, `...DuplicateWindowName` → `TestCompileRejectsASpecThatBreaksARule` (table, one row per rule, now asserting the compile-error path and message rather than `err != nil`); `TestCompileRejectsInvalidCEL` → `TestCompileReportsTheCELExpressionThatDoesNotParse`.
- Shared fixtures (`minimalSpecYAML`, `editedSpec`, `compileSource`, `compileFile`) moved to `fixtures_test.go` (T9).

### Improved
- T6: every top-level test and subtest in the module is parallel.
- T4: `TestParse…` asserts the message of each error (`empty duration`, `parse days`, `must be positive`, `overflows time.Duration`); the policy-plane test asserts `schema validation`; facade and store error tests assert the sentinel (`fs.ErrNotExist`, `sql.ErrNoRows`) and the wrapping text.
- `TestExperimentSpecsCompileToTheirPinnedDigests` is one parallel subtest per pinned spec. Pins unchanged.
- `TestAllBuiltinsLoadFromData`: only its explanatory comment was removed; the 52-ref count and golden digest are untouched.
- `TestCompileBoundsTheWatchConfidenceFloorToZeroAndOne` adds the inclusive boundaries 0 and 1.

### Added
- `TestCompileRejectsASpecThatBreaksARule` rows for reference rules that had no test: duplicate input, operator name, operator output and phase names; unregistered event schema; schema bound to another event type; unknown operator input; unknown window; unknown reducer input; unknown initial phase; unknown transition source and target; malformed YAML.
- `TestCompileReportsTheCELExpressionThatDoesNotParse`: the compile error names the failing path for open, close, transition, trigger when/score/materialDelta and operator `where` expressions.
- `TestConcurrentCompilesAgreeOnEverySource`: concurrent valid and invalid compiles agree (proves the shared schema, below).
- `TestCELBoolAcceptsOnlyBooleans` (domain), `TestCELEnvDeclaresOnlyTheSpecVariables`.
- `TestResolveReferencesRejectsALaneOutsideTheVocabulary`: lane check is defense in depth behind the JSON Schema enum, unreachable through `CompileBytes`.
- Store: `TestSaveDeploymentRefusesASpecItCannotIdentify` (nil, empty and unprefixed digest, no rows left), `TestSaveDeploymentWithoutCanonicalJSONStoresTheCompiledSpecAsSource`, `TestNewSpecVersionRetiresThePreviousDeploymentOfTheSameTenantAndName` (retirement is scoped to tenant and name), `TestSaveDeploymentRegistersTheEventSchemasOfItsInputs` (built-ins registered, unknown refs skipped), `TestLoadDeploymentReturnsTheSavedSpec`, `...ReportsAnUnknownDeployment`, `...ReportsAStoredSpecItCannotDecode`, `TestSchemaRegistrationReportsAFailedStorageRead`; validation table rows for each missing identity field.
- Facade: `TestFacadeNamesTheFileAndWrapsTheCauseWhenCompilationFails`, `TestFacadeRefusesToDeployWhatItCannotIdentify`, `TestFacadeReportsAnUndeployedSpec`; `EffectiveDispatchPolicy` wiring.
- App: `TestCompileFileReturnsTheCompilerDiagnosticUnchanged` (the `*CompileError` path survives).
- Spectest: `TestEventSchemaJSONRendersTheStructuralSchema`, `TestRegisterEventSchemaRefusesDifferentBytesForARegisteredVersion`; the idempotency test now counts rows.

### Speed
- Measured, not assumed. `Compiler.CompileBytes` on the minimal spec: 4.1 ms/op (26.9 ms/op under `-race`); of that, preparing the embedded 16 KB JSON Schema is 3.4 ms (20.0 ms under `-race`), because `NewCompiler()` is built per call (`spec/internal/app`) and its schema cache lived on the instance. The CEL environment costs 4 µs (27 µs under `-race`): not worth caching.
- After caching the compiled schema once per process: 0.7 ms/op (6.9 ms/op under `-race`), 5.8× faster per compile under race. Every replay, serve, CLI and operator test that compiles a spec pays this once instead of per compile.

## Production code touched
- `internal/spec/internal/domain/compiler_schema.go`, `compiler.go`: the embedded situation-spec JSON Schema is compiled once through `sync.OnceValues` (the same pattern as `notify`'s contract schema) instead of per `Compiler`. `Compiler` becomes an empty struct; `prepareSchema`/`validateSchema` methods become `embeddedSchema()`/`validateSchema(schema, raw)`. Behavior is preserved: the input is an embedded constant, a compile error is deterministic and would now be cached instead of repeated, the schema is read-only after compile and `jsonschema.Schema.Validate` keeps its state per call, the schema error still precedes parse errors, and `TestConcurrentCompilesAgreeOnEverySource` runs 8 goroutines under `-race`. The CEL environment is not cached (4 µs; no gain).

## Invariants proven here
- 1 (raw events are evidence, never instructions) and 6 (a model cannot execute effects), at the spec boundary: `TestCompileRefusesIntentPoliciesThePolicyPlaneDoesNotEnforce` (an intent that would dispatch automatically is refused at compile time), `TestCompileRejectsASpecThatBreaksARule`.
- Deterministic replay input identity: `TestCompileGivesEquivalentYAMLTheSameDigest`, `TestCompileGivesJSONAndYAMLSourcesTheSameDigest`, `TestExperimentSpecsCompileToTheirPinnedDigests`, `TestAllBuiltinsLoadFromData`.
- The real-world-sensor experiment contract: `TestExperimentSpecsCompileToTheirPinnedDigests`, `TestExperimentZoneEventSchemasAreUnchanged` (pins unchanged).

## Open items
- None for this module. The remaining uncovered statements are unreachable branches; not tested on purpose.
