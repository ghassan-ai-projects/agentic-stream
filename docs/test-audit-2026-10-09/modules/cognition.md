# cognition

Status: done
Round: 6

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing incl. subtests) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/cognition` | 66.7% | 100.0% | 1.40 s | 1.4 s | 1 / 1 | 3 / 3 |
| `internal/cognition/internal/app` | 71.0% | 84.1% | 2.24 s | 2.8 s | 10 / 15 | 29 / 59 |
| `internal/cognition/internal/domain` | 82.8% | 95.1% | 1.42 s | 1.3 s | 11 / 32 | 39 / 91 |
| `internal/cognition/internal/store` | 68.8% | 83.2% | 2.30 s | 1.9 s | 15 / 20 | 33 / 37 |

Test-hygiene lint: 19 findings (12 in `store/engine_test.go`, 4 in `app/scheduling_test.go`,
3 in `app/admission_audit_test.go`) → 0. Repository lint and `dupl` at threshold 60: 0.

App wall time rose by about 0.6 s because it now proves 59 cases, not 15, each on its own
database (about 0.15 s alone, longer when 30 tests share cores under `-race`). Every
database test is parallel; the 100-row queue fill is one transaction.

## Findings and changes

Layout: before, durable-path trigger tests lived in `store` (through the facade) and again in
`app`, with two copies of the situation-row fixture and two copies of the reconsideration
fixture. Now `store` proves SQL through `store.Join(tx)`, `app` proves use cases once on a real
database, `domain` proves pure rules, and the facade proves wiring.

### Removed
- `store/engine_test.go` and `app/scheduling_test.go`: two overlapping suites. Their 15 cases
  are merged into `app/trigger_evaluation_test.go` and `app/queue_timing_test.go`
  (`TestDebounceAndCooldownSetNotBefore...` and `TestDebounceAndCooldownTogether` were the same
  test at different layers).
- `store/rollback_test.go`: a copy-paste of the app reconsideration fixture; its behavior
  (caller rollback leaves nothing) is `TestReconsiderationRollsBackWithTheCallersTransaction`.
- `domain/boundaries_test.go`: the name was a grab-bag; its tests moved by subject.
- `TestSchedulingTimingAndReplacementRules` mixed five unrelated asserts in one test (T11); split.

### Renamed or moved

| Old | New |
| --- | --- |
| `domain/boundaries_test.go` `TestSchedulingTimingAndReplacementRules` | `timing_test.go`: `TestGlobalCapacityIsExhaustedOnlyWithoutAReplaceableItem`, `TestOnlyAnOlderVersionIsSupersededByANewerOne`, `TestSchedulerDedupeKeyIdentifiesSituationVersionAndTrigger`, `TestExplanationsAppendToTheEvaluationsReasons` |
| `TestTimingKeeps...`, `TestTimingRefuses...` | `timing_test.go` (same names; new rows and messages) |
| `TestReconsiderationEvidenceAndDigestRefusal` | `correction_test.go`: `TestOnlyACorrectedVersionUnderTheReconsiderPolicyIsReconsidered`, `TestReconsiderationEvidenceIsCanonicalAndCarriesTheCorrection`, `TestDecodeCorrectionRefusesUnreadableSnapshots`, `TestCorrectionMustMatchItsPersistedDigest` |
| `TestDeltaUsesPreviousFactsAndConditionDefaults` | `delta_test.go` (same name) |
| `store/engine_test.go` (11 tests) | `app/trigger_evaluation_test.go`, `app/queue_timing_test.go` |
| `app/scheduling_test.go` `TestCapacityExhaustionDefers`, `TestCoalescingPendingItem` | `app/explainability_test.go` (`deferred`, `coalesced`), `app/capacity_test.go` |
| `app/admission_audit_test.go` | `app/evaluation_reasons_test.go` (cost refusal, expiry) and `store/queue_test.go` (`TestOnlyPendingItemsOfTheTenantCountTowardCapacity`) |
| `app/scheduler_test.go` | `app/scheduler_item_test.go` (built on the shared harness: 90 lines of seed SQL → 25) |
| `store/evaluation_event_test.go`, `store/notifications_test.go` | `store/events_test.go` |
| `internal/cognition/service_test.go` | rewritten (`package cognition_test`) |

### Improved
- T6: all tests and subtests parallel; `context.Background()` → `t.Context()`.
- T5: the old suites used `sources.Physical()` and `time.Now()`; the harness uses a virtual
  clock, and versions carry virtual event times.
- T9: `app/fixtures_test.go` (harness, `candidateVersion`, queue filler),
  `app/reconsideration_fixtures_test.go` (one executed-command and correction fixture),
  `store/fixtures_test.go` (seeds). Duplicate fixtures: 4 → 2 (one per layer).
- T4: error tests assert the message.
- Found and fixed a test defect: the old fixture inserted `situations.current_version` once
  and never updated it, so no test could see a supersession notification. The harness now
  advances `current_version`, and `TestEveryCognitiveOpportunityIsExplainableFromDurableRecords`
  asserts the `situation.superseded` announcement.

### Added
- `app/explainability_test.go` `TestEveryCognitiveOpportunityIsExplainableFromDurableRecords`
  (invariant 10; see below).
- App: `TestFullQueueStillAdmitsAVersionThatReplacesItsOwnPendingItem`,
  `TestQueueJustBelowCapacityStillAdmits`, `TestSupersessionLeavesOtherSituationsAndTriggersAlone`,
  `TestReEvaluatingAVersionDoesNotAnnounceItAsSuperseded`, `TestProcessMarksTheVersionReasonedAndOnlyMaterialVersionsMaterial`,
  `TestProcessNamesTheTriggerWhoseEvaluationFailed`, service construction refusals,
  `TestReconsiderationFollowsOnlyTheLatestApprovedIntentAtOrBeforeThePreviousVersion` (the
  `InvalidatedCommands` SQL), `TestCorrectionsAreNotReconsideredUnlessThePolicySaysSo`,
  `TestCorrectionWhoseSnapshotDigestDoesNotMatchIsRefused`,
  `TestReconsiderationRefusesASpecWithoutADigest`, `TestTriggerEvaluationsAreReadBackWithTheirReasons`.
- Domain (new files `delta_test.go`, `correction_test.go`, `timing_test.go`): trigger compile
  refusals per expression, wrong-typed results, integer or double scores, UTC evaluation time,
  stable trigger identity per (deployment, situation, version, name), delta per change kind,
  first-version delta, uncertainty = 1 − confidence, `mapsEqual`, correction identity ignores the
  correction version (a re-delivered correction is deduplicated), reconsideration evaluation and
  item, rejected reconsideration reason, prior-document rejections (8 rows), `FindTrigger`,
  `NewSchedulerItem`, `ParseOptionalDuration`.
- Store: `queue_test.go` (pending counts, upsert, latest admission, unreadable time),
  `evaluations_test.go` (upsert, overwrite, bad digest, announce once, reasons, marks),
  `history_test.go` (last reasoned, version load and six refusal cases), `supersession_test.go`
  (replacement, coalescing scope, announcement, approval withdrawal with a real approval),
  `reconsideration_test.go`, `evaluation_reads_test.go` (ordering, tenant isolation, undecodable
  reasons), `tx_test.go`.
- Facade: `TestProcessedVersionIsReadableThroughTheFacadeReaders`,
  `TestFacadeRecordsWhyAnAdmittedItemWasRefusedOrExpired` (the readers and `RecordSchedulerExpiryReason` were 0%).

### Speed
See the table: the added cases cost about 0.6 s of app wall time; the old store/app overlap and
`sources.Physical()` waits were removed. No test is above 1.5 s under contention.

## Production code touched
- none

## Invariants proven here
- Invariant 10 (every admitted, deferred, coalesced, rejected, canceled, expired opportunity is
  explainable from durable records), lowest owning layer `internal/cognition/internal/app`:
  `TestEveryCognitiveOpportunityIsExplainableFromDurableRecords` with subtests `ignored`,
  `admitted`, `deferred`, `coalesced`, `rejected`, `refused by cost control`, `expired`. Supporting:
  `TestFullQueueStillAdmitsAVersionThatReplacesItsOwnPendingItem`,
  `TestCostRefusalIsAddedToTheEvaluationInTheCallersTransactionOnly`,
  `TestSchedulerExpiryIsAddedToTheEvaluationWithoutTouchingTheQueue`,
  `TestUnreadablePriorDocumentsRejectOnlyThatReconsideration`,
  `TestEachTriggerGateRecordsItsOutcomeAndReasonDurably`. Episode cancellation on supersession
  is owned and tested by `episodeledger` (`SupersedeCoalesced`); cognition proves the call and
  the `situation.superseded` announcement.
- MVP acceptance, debounce and cooldown: `TestTimingKeepsAnItemUsefulForExpiresAfterOnceItMayStart`
  (domain, 9 rows), `TestDebounceAndCooldownDecideWhenAnAdmittedItemMayStart` (app, durable
  `not_before`), `TestFirstAdmissionHasNoCooldownToWaitFor`. Material-delta suppression:
  `TestMaterialityFollowsTheSpecsDeclaration`, the `material delta false` row of
  `TestEachTriggerGateRecordsItsOutcomeAndReasonDurably`. Hysteresis lives in
  `internal/situations` (see [situations.md](situations.md)).
- Invariant 4/9 neighbours (idempotent, replay-safe admission): `TestReconsiderationAdmissionIsReplayDeduplicated`,
  `TestReEvaluatingTheSameVersionUpsertsInsteadOfDuplicating`,
  `TestReconsiderationRollsBackWithTheCallersTransaction`.

## Open items
- `RecordCostRejectionReason` and `RecordSchedulerExpiryReason` append to the evaluation's
  reasons but leave its `outcome` as `admitted`, so a cost-refused or expired opportunity reads
  as admitted with a trailing reason. Explainable (invariant 10 holds), but a reader must scan
  reasons to learn it was refused. Owner decision whether the outcome should change.
- A cooldown that has already elapsed still stores a past `not_before`
  (`cooldown already elapsed` row of the timing table). Harmless; pinned.
- Uncovered: marshal-error branches and `InvalidatedCommands` row-scan errors.
