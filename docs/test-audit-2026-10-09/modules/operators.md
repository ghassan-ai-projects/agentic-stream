# operators

Status: done
Round: 6

`internal/operators` has two Go packages with tests: the facade and `internal/domain`
(no app or store layer; the runtime is pure). The 845-line `operators_test.go` is
split by subject and its repetitive cases are tables.

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing incl. subtests) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/operators` | 100.0% | 100.0% | 1.46 s | 1.4 s | 1 / 1 | 2 / 2 |
| `internal/operators/internal/domain` | 85.4% | 98.5% | 1.41 s | 1.3 s | 20 / 48 | 45 / 121 |

Test-hygiene lint: 20 findings (`paralleltest`) → 0. Repository lint and `dupl` at threshold 60: 0.
Slowest test is well under 0.1 s; wall time is race start-up.

## Findings and changes

### Removed
- `TestSlope`'s only assertion was that some feature existed; replaced by
  `TestSlopeIsReportedInValueUnitsPerHour`, which asserts the value and unit (T4).
- The zero-value checks in the facade test (`feature.Completeness != ""`,
  `heartbeat.BootID != ""`): they asserted that a Go zero value is zero (T10).

### Renamed or moved

| Old test (`operators_test.go`) | New |
| --- | --- |
| `TestAggregateMean` | `TestMeanWindowReportsTheAverageOfItsSamples` (`window_test.go`) |
| `TestSlope` | `TestSlopeIsReportedInValueUnitsPerHour` |
| `TestLateEventCorrectsPreviouslyEmittedWindow` | `TestCorrectedWindowReemitsEveryInputIncludingTheLateOne` and the table `TestLateEventIsCorrectedOnlyWithinPolicyAndAllowedLateness` |
| `TestOnCloseWindowEmitsAtWatermarkSlideBoundary`, `TestEarlyAndCloseWindowEmitsProvisionalUpdates` | table `TestEmitModeDecidesWhichEventsEmitAFeature` plus `TestOnCloseWindowEmitsTheClosedAggregateAtTheWatermark` |
| `TestUnsupportedWindowConfigurationFailsClosed` | `TestRuntimeRefusesAWindowItCannotRunExactly` (`window_config_test.go`, now asserts the message) |
| `TestLatestAggregateUsesDeterministicEventTimeOrdering` | `TestLatestAndMaximumFollowEventTimeThenEventIDNotArrivalOrder` |
| `TestNumericQualityGate` | `TestOnlyValidFiniteNumericObservationsEnterAWindow` (`samples_test.go`) |
| `TestQualityAdmissionIsPerOperator` | same name (`samples_test.go`) |
| `TestMissingHeartbeatTimerUsesDetectionTime` | `TestMissingHeartbeatTimerIsTimedAtDetection` (`heartbeat_test.go`) |
| `TestMissingHeartbeatEventUsesDetectionTimeWhenLate` | table `TestHeartbeatEventReportsPresenceAtTheWatermark` |
| `TestHeartbeatTimerCarriesExplicitTenantAndPartition`, `TestApplyTimerHonorsCancellation`, `TestInvalidHeartbeatDoesNotRefreshLiveness` | same names (`heartbeat_test.go`) |
| `TestWindowStateBootBoundaryAndSequenceWrap` | `TestSequenceWrapWithinOneBootStaysInTheWindow`, `TestNewBootStartsAFreshWindowAndDiscardsTheOldBootsState` (`boot_test.go`) |
| `TestStaleBootCannotMutateWindow` | `TestStaleBootCannotMutateTheActiveWindow` |
| `TestHeartbeatTimerSkipsPreviousBootState`, `...SkipsBootlessStateAfterBootAdmission`, `...FeatureCarriesBootScopedStateKey` | `TestHeartbeatTimerSkipsThePreviousBootsState`, `...SkipsBootlessStateOnceABootIsAdmitted`, `...FeatureCarriesTheBootScopedStateKey` |

`window_aggregate_test.go` keeps its name (it proves `window.go`'s aggregates). Test files:
`fixtures_test.go` (harness and builders), `window_test.go`, `window_config_test.go`,
`samples_test.go`, `runtime_test.go`, `heartbeat_test.go`, `boot_test.go`,
`window_aggregate_test.go`.

### Improved
- T6: every test and subtest is parallel (all 20 findings were missing `t.Parallel()`).
- T9/T11: one `harness` (runtime plus partition state) and envelope builders replace
  the per-test copies of a 15-line envelope literal; tables replace three
  copy-pasted event loops.
- T4: error cases assert the message (`requireErrorContaining`), not `err != nil`.
- `context.Background()` replaced by `t.Context()`.

### Added
- `TestEmitModeDecidesWhichEventsEmitAFeature`: all four emit modes (including the
  empty default) over four events.
- `TestLateEventIsCorrectedOnlyWithinPolicyAndAllowedLateness`: seven rows, including
  the exact boundary, a non-correcting policy, no declared lateness, and a sample
  older than the window. `TestMalformedAllowedLatenessFailsTheLateEvent`.
- `TestRuntimeRefusesAWindowItCannotRunExactly` (11 rows), `TestRuntimeAcceptsTumblingAndSlidingWindows`,
  `TestRuntimeRefusesAnOperatorWhoseWindowIsUndeclared`,
  `TestUnsupportedOperatorKindAndAggregateFailClosedWhenEventsArrive`.
- `TestOnlyValidFiniteNumericObservationsEnterAWindow`: 18 rows (value types `float32`,
  `int`, `int64`, `json.Number`, infinite, text, missing field, non-string quality).
  `TestOperatorFieldMustNameOneDataKey`.
- Heartbeat: deadline boundaries on the event path and on the timer
  (`TestHeartbeatTimerFiresOnlyOnceTheDeadlineHasPassed`), fallback to event time,
  timer ordering by state key, one firing per operator declared on several inputs,
  timer identity validation (`TestTimerIdentityIsValidated`), no identity, nil partition
  state, malformed duration, trace continuation.
- Boot admission: bootless events refused after the first boot
  (`TestBootlessEventsAreAdmittedOnlyUntilTheFirstIdentifiedBoot`), a retired boot is never
  re-admitted (`TestRetiredBootIsNeverAdmittedAgain`), the 64-boot history fails closed
  (`TestBootHistoryIsBoundedAndFailsClosedWhenFull`), `IsTimerStateActive`
  (`TestTimerStateIsActiveOnlyForTheCurrentBoot`, was 0%).
- Runtime: undeclared event type, secondary input ignored, nil partition state,
  `ApplyEventAt` cancellation.
- `TestSlopeIsZeroWithoutSpreadInTime`, `TestFacadeCompletenessConstantsKeepTheirStoredText`
  (the three exported completeness strings are stored in snapshots).

### Speed
Nothing slow: no database. Package time is race start-up.

## Production code touched
- none

## Invariants proven here
- "Suppress noisy repeated cognition" starts upstream: invalid, stale-boot and
  non-finite evidence never reaches a Situation (`TestOnlyValidFiniteNumericObservationsEnterAWindow`,
  `TestStaleBootCannotMutateTheActiveWindow`, `TestInvalidHeartbeatDoesNotRefreshLiveness`).
- Invariant 8 (replay determinism) at the operator layer: `TestLatestAndMaximumFollowEventTimeThenEventIDNotArrivalOrder`,
  `TestHeartbeatTimerOrdersFeaturesByStateKey`.

## Open items
- `percentile`-style unsupported kinds fail at event time, not at `NewOperatorRuntime`
  (`TestUnsupportedOperatorKindAndAggregateFailClosedWhenEventsArrive` pins this).
  The spec compiler probably rejects them earlier; left as is.
- Uncovered: only error branches that need an unparsable duration after the constructor
  already validated it, and a defensive nil-state branch.
