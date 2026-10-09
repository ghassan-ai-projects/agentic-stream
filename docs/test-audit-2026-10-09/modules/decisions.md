# decisions

Status: done
Round: 9

## Metrics

Measured with `go test -short -race -count=1` while other workers ran (compare back to back, not absolute).

| Package | Coverage before | after | Time before | after | Tests before (top-level / passing incl. subtests) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/decisions` (facade) | 100.0% | 100.0% | 1.24 s | 1.13 s | 3 / 3 | 3 / 3 |
| `internal/decisions/internal/domain` | 84.5% | 95.3% | 1.55 s | 1.18 s | 12 / 37 | 27 / 103 |

Test-hygiene findings: 19 → 0 (`paralleltest`, `tparallel`, `usetesting`, `thelper`). Repository lint: 0 issues; `dupl` at threshold 60: 0 issues. No `time.Sleep`. Slowest test 0.03 s.

`validator_test.go` (543 lines) is gone. The domain package now has 7 test files, the largest 209 lines.

## Findings and changes

### Removed
- `TestValidateCompensatingIntentTypes` (`validator_test.go`): loaded the predictive-maintenance spec through `spec.CompileFile` to take an allowlist the validator never consults for a compensating intent (`checkIntentPermission` skips `AllowedIntentTypes` when `compensates` is set). Its "allowed by spec" cases proved nothing about the spec; the behavior it meant to prove is now `TestValidateAllowsCompensationOnlyInReconsiderEpisodes`. The `spec` import and the repository-relative file read are gone from this package (T2, T10).
- `digest tamper` and `duplicate raw key` subtests of `TestValidateRejectsSecurityAndBindingFailures`: repeat `TestParseDecisionNamesTheStoredDocumentFailure` (`wrong digest`, `ambiguous bytes`) at the same layer.
- `TestValidateRejectsImplicitAbstention` and `TestValidateAllowsExplicitAbstention`: merged into `TestValidateAbstainsOnlyWhenTheDecisionAsksForMoreEvidence`.
- `TestIntentDigestValidationPrecedesIdentityBinding` (was at the bottom of `validator_test.go`) became `TestValidateChecksIntentDigestBeforeIdentityBinding` in `rejection_order_test.go`.
- Ticket and phase comments in the tests (`P4`, `A-013 F1`, `G5`): the no-comments rule holds in tests.

### Renamed or moved
- `validator_test.go` split by subject:
  - `fixtures_test.go`: `validInput(t)`, `inputAllowing`, `compileCatalog`, `testCatalog`, `validDecision`, `validIntent`, `sealIntent`/`resealIntents`, `encodeDecision`, `validateDocument`, `requireRejection` (asserts reason and `Details["field"]`), `digestText`. Fixtures no longer `panic`; they take `t`.
  - `decision_shape_test.go`: result provenance, catalog policy carried into the intent, abstention, stored-document failure fields, unknown properties, trusted input, decision binding table, `valid_until` boundary, `ValidationError.Error`.
  - `intent_binding_test.go`: intent binding to decision/tenant/situation/version, duplicate id, expiry boundary, UTC normalization, one-actionable-intent rule.
  - `intent_authority_test.go`: allowlist, catalog membership, declared risk vs label vs ceiling, compensation.
  - `intent_parameters_test.go`: identity parameters, preset authorship, model-writable override, parameter schema, evidence grounding.
  - `catalog_test.go`: fail-closed compilation table, compiled catalog independent of its source.
  - `rejection_order_test.go`: one table of "earliest failing check wins" pairs.
- `parse_decision_test.go` → `TestParseDecisionNamesTheStoredDocumentFailure` in `decision_shape_test.go` (now also asserts `reason`, and the `bound document` success case).
- `TestCompileIntentCatalogFailsClosed` (5 `t.Fatal` branches) → one table of 9 cases asserting the error message (no sentinel exists).

### Improved
- Every test and subtest is `t.Parallel()`; rejections assert reason and field, not reason alone (T4, T6).
- `TestValidateReportsTheEarliestFailingCheck` turns the old attempt-before-snapshot test into a 9-row order table (episode before attempt, attempt/fence before snapshot, decision before intent, intent binding before authority, type before declared risk, authority before parameters, schema before preset, preset before expiry), because rejection order is durable episode evidence.
- Boundaries are exact: intent `expires_at == Now` is expired, one nanosecond later is valid; decision `valid_until == Now` is expired.

### Added
- `TestValidateDecisionValidityWindowEndsAtValidUntil`: decision expiry (`isExpired` was 0%).
- `TestValidateRefusesDecisionsBoundToAnotherDispatch`: episode, situation, situation version, omitted situation (were untested).
- `TestValidateRefusesIntentsBoundToAnotherDecisionOrSituation`: intent decision, tenant, situation, version.
- `TestValidateRefusesDuplicateIntentIDs`, `TestValidateIntentLifetimeEndsAtExpiresAt`, `TestValidateNormalizesIntentExpiryToUTC`.
- `TestValidateAllowsOneActionableIntentBesideWatchesAndCompensations`: the at-most-one-actionable rule and its two exemptions.
- `TestValidateAllowsCompensationOnlyInReconsiderEpisodes`: the forged-`compensates` rejection in a diagnose episode (was uncovered).
- `TestValidateBindsIdentityParametersToTheEpisode`: `entity_id`, `target` and `expires_at` parameter binding including "no episode identity" (the old test covered `target` only).
- `TestValidateKeepsPresetFieldsPresetAuthored`, `TestValidateLetsTheModelOverrideOnlyWritableFields`, `TestValidateRefusesParametersOutsideTheCatalogSchema`, `TestValidateGroundsIntentEvidenceInTheDecisionFacts` (grounded, none, forged, mixed).
- `TestValidateCarriesCatalogPolicyIntoTheValidatedIntent`: rate limit, `RequiresApproval`, risk and expiry reach the result policy consumes.
- `TestCompiledCatalogIsIndependentOfItsSource`: domain-level copy isolation of presets (including nested slices) and schema.

### Speed
Nothing was slow. Dropping `spec.CompileFile` removed a YAML compile from the test path.

## Production code touched
- none.

## Invariants proven here
- 1 (raw events are evidence, never executable instructions): `TestValidateKeepsPresetFieldsPresetAuthored`, `TestValidateBindsIdentityParametersToTheEpisode` (including `attacker-controlled-target`), `TestValidateGroundsIntentEvidenceInTheDecisionFacts`, `TestValidateRefusesParametersOutsideTheCatalogSchema`, `TestValidateRefusesUnknownDecisionProperties`, `TestParseDecisionNamesTheStoredDocumentFailure` (ambiguous keys, wrong digest).
- 6 (models propose typed intents, never execute): `TestValidateRefusesIntentTypesTheEpisodeDoesNotAllow`, `TestValidateRefusesAllowedTypesTheCatalogDoesNotDeclare`, `TestValidateHoldsProposedRiskToTheCatalogAndTheCeiling`, `TestValidateAllowsCompensationOnlyInReconsiderEpisodes`, `TestValidateAllowsOneActionableIntentBesideWatchesAndCompensations`, `TestValidateCarriesCatalogPolicyIntoTheValidatedIntent`, `TestValidateReportsTheEarliestFailingCheck`.

## Open items
- A compensating intent bypasses `Input.AllowedIntentTypes` entirely: only `Reconsider` and catalog membership gate it. Pinned by `TestValidateAllowsCompensationOnlyInReconsiderEpisodes`; confirm intended (the README says "all Intent types ... are checked against both the catalog and attempt authority").
- Uncovered, unreachable-by-design branches (5% of the package): the empty-intent-set check and the non-object intent check run after the Decision and Intent schemas, which already reject both; the `canonicaljson.Marshal` error branches of the preset comparison and `materializeIntent`; the non-string evidence id branch (schema forbids it).
- Production files under `internal/decisions/internal/domain` still carry ticket comments (`P4`, `B10`, `T3`) against the no-comments rule; not changed in a test round.
