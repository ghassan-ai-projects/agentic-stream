# situations

Status: done
Round: 6

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing incl. subtests) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/situations` | 100.0% | 100.0% | 1.47 s | 1.4 s | 1 / 1 | 3 / 3 |
| `internal/situations/internal/domain` | 72.3% | 93.8% | 1.43 s | 1.4 s | 6 / 6 | 32 / 49 |

Test-hygiene lint: 3 findings → 0. Repository lint and `dupl` at threshold 60: 0.

## Findings and changes

### Removed
- `TestSituationSkipsNilFactWhenBuildingFeatures` asserted only that no version appeared;
  the rule (a nil fact falls back to the operator default) is now proved where it lives,
  in `TestCELFeaturesDefaultsEveryOperatorOutputThatHasNoFact`.
- Facade zero-value check (`current.Version != 0`): asserted a Go zero value (T10).

### Renamed or moved
- `situations_test.go` (domain) split by subject: `TestSituationTransitionsOnFeature` →
  `TestSituationClimbsPhasesAsItsConditionsHold` (`lifecycle_test.go`);
  `TestSituationPublishesCompletenessChangeFromSourceHealth` →
  `TestCompletenessChangeOfALiveSituationPublishesANewVersion` (`lifecycle_test.go`).
- `materialize_test.go` is unchanged (already parallel, internal package).
- Facade `TestFacadeBuildsAnEngineAndTheCELFeatureView` → `TestFacadeBuildsAnEngineForACompiledSpec`,
  `TestFacadeCELFeaturesDefaultsEveryOperatorOutput`, `TestFacadeResolvedPhaseKeepsItsStoredText`.

New files: `fixtures_test.go`, `lifecycle_test.go`, `reducers_test.go`, `restore_test.go`,
`version_immutability_test.go`, `expressions_test.go`, `cel_features_test.go`.

### Improved
- T6: all tests parallel; `context.Background()` → `t.Context()`.
- T9: one `vibrationSpec` and `vibration(...)`/`apply(...)` builder instead of three inline specs.

### Added
- Hysteresis (the MVP acceptance item): `TestTransitionWaitsForItsMinimumDuration`
  (held for the full duration, one nanosecond short, a lapse restarts the clock),
  `TestValuesBetweenTheOpenAndCloseThresholdsDoNotFlapThePhase`,
  `TestConditionStartRecordsTheInstantAPendingTransitionFirstHeld`,
  `TestFeaturesBelowTheOpenConditionPublishNothing`.
- Occurrence close: `TestOccurrenceResolvesWhenItsCloseConditionHolds` (resolved, severity 0, stays resolved).
- Identity and versioning: `TestEachPublishedVersionNamesItsPredecessorAndPhase`,
  `TestSituationIdentityIsStableForTheSameDeploymentTenantPartitionAndEntity`.
- Reducers: `TestLatestEventTimeFactKeepsTheNewestObservation` (older, equal, newer),
  `TestSetUnionReducerAccumulatesEvidenceWithoutDuplicates`,
  `TestPublishedEvidenceIsSortedAndFactsHideInternalBookkeeping`,
  `TestFeatureWithoutAReducerLeavesNoFact`, `TestTimerFeatureMetadataIsPublishedAsTimerProvenance`.
- Engine state: `Restore` identity validation (was 0%), restored situation continues its version,
  `CurrentState` returns a copy plus its canonical blob and digest, unknown keys, `Reset`.
- Failure: CEL syntax or type errors fail the feature, empty conditions never hold, a spec
  without a valid digest cannot publish, a Situation violating the snapshot contract is never
  published (`TestSituationThatViolatesTheSnapshotContractIsNeverPublished`).

### Speed
Pure domain; nothing slow.

## Production code touched
- none

## Invariants proven here
- Invariant 3 (a Situation version is immutable after publication), lowest owning layer
  (`internal/situations/internal/domain`):
  - `TestPublishedSituationVersionNeverChangesAfterLaterFeatures`
  - `TestMutatingAPublishedVersionDoesNotReachTheEngine`
  - `TestVersionNumbersOnlyGrowAndEachStepNamesItsPredecessor`
  - `TestMaterializationKeepsCanonicalEvidenceAndPrivateState` (state mutation after publication)
  - `TestMaterializationPinsTheDigestsThatEmbedInstants`
  - `TestCurrentStateIsACopyWithItsCanonicalStateAndDigest`
- MVP acceptance, hysteresis: the four hysteresis tests above.

## Open items
- Reopening an occurrence after it resolves: UBIQUITOUS_LANGUAGE says "reopening needs a
  cooldown", but the engine never reopens (a resolved Situation stays resolved; pinned in
  `TestOccurrenceResolvesWhenItsCloseConditionHolds`). Owner decision: fix the language or
  the engine.
- Chained transitions (from the engine round): one feature can satisfy several transitions
  in a row (`candidate -> watch -> warning`, each with `minDuration: 0s`). `evaluate` bumps
  `Version` once per transition but `materialize` runs once, so only the last version is
  published and the first published version of a Situation can be 2, with `PreviousVersion`
  1 and `PreviousPhase` naming a phase that was never published. Conclusion: this is how the
  code is written and storage accepts it (`situation_versions` only requires
  `previous_version < version`, no contiguity), and the design (TECHNICAL_DESIGN section 10.1)
  says nothing about collapsing; but UBIQUITOUS_LANGUAGE.md says "every change publishes a
  new version", which this contradicts. I read it as unintended or at least undocumented, not
  as a deliberate rule. Pinned by `TestChainedTransitionsOfOneFeaturePublishOnlyTheFinalVersion`;
  no production change. Owner decision: either publish one version per transition (changes
  version numbering and downstream replay goldens) or document that versions count state
  changes and publication collapses them.
- Remaining uncovered statements are marshal-error branches.
