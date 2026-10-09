# eventlog

Status: done
Round: 5

Audited in round 5 with [engine](engine.md). Production code is unchanged.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running (a package costs about 1.1 s of that in compile and
race start-up). Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/eventlog` (facade) | 77.8% | 100.0% | 1.9-2.5 s | 1.8 s | 14 / 5 | 10 / 5 |
| `internal/eventlog/internal/app` | 71.5% | 94.9% | 1.6 s | 2.3 s | 7 / 0 | 26 / 9 |
| `internal/eventlog/internal/domain` | 79.4% | 100.0% | 1.2 s | 1.1 s | 11 / 4 | 17 / 32 |
| `internal/eventlog/internal/store` | 67.1% | 92.2% | 1.6-1.9 s | 3.3 s | 7 / 3 | 28 / 22 |

The store and app packages grew because the layers now carry the tests the
facade used to carry (T2) and the error branches that were never exercised.
Slowest test after: `TestQuarantineCountsRetriesAndRejectsOnceTheBoundIsSpent`
0.65 s. No package is near 15 s; no test near 5 s.

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 20 findings in the
two modules before (11 in eventlog, 9 in engine), 0 after. Repository `golangci-lint`: 0
issues, also at `dupl` threshold 60. `time.Sleep` and `t.Skip`: none.
`go test -race -shuffle=on -count=3`: passes. `go test ./internal/architecture/...`: passes.

## Findings and changes

### Removed
- `boundaries_test.go` (facade), dissolved. Its four tests duplicated lower layers (T2):
  - `TestAppendRollsBackEarlierRecordsWhenLaterAdmissionFails`: the app test `TestAppendRejectsInvalidEnvelopeAndRollsBackEarlierRecords` already proves the rollback; it now also proves the valid event can be appended again afterwards.
  - `TestAppendValidatesTraceBeforeIgnoringDuplicate`: moved to app as `TestAppendValidatesTraceContextEvenForADuplicate`. The app test it replaced (`TestAppendRejectsInvalidTraceContextBeforeDuplicateCheck`) used a fresh log and never reached the duplicate, so it did not prove its name (T4).
  - `TestReadFiltersBeforeLimitAndPreservesLogOrder` and `TestReadStopsOnCallbackFailureAndReleasesConnection`: SQL behavior, moved to store (below).
- `TestQuarantineIsBoundedAndReleasable`, `TestQuarantineRejectsEventIDHashConflict`, `TestReleasedQuarantineCanBeValidatedAndRedrivenOnce` (facade): each asserted app and store rules through raw SQL on `event_quarantine` and `event_gaps`. Replaced by app and store tests that assert through `Quarantined`, and by one facade wiring test, `TestQuarantinedEnvelopeIsListedReleasedAndRedrivenOnceThroughTheFacade`.
- `TestQuarantineConflictAndBoundFailClosed` (app): its five mixed assertions became separate named tests; `Quarantine(ctx, "", ...)` is now part of the required-fields table.
- `TestUseCaseInputsFailClosed` (domain): checked one missing input per function; replaced by `TestQuarantineAndReleaseRequireTenantEventAndTime`, a six-row table naming each missing input. `TestEncodeEventBodyDigestsPayloadBytes`'s error half became `TestEncodeEventBodyNamesWhichPartCannotBeEncoded`.
- Duplicate `registerTemperatureSchema` (same body as `registerSchema`) in the facade.

### Renamed or moved
- `internal/eventlog/schema_test.go` → `schema_validation_test.go` (names its subject; the file now holds only schema-validation wiring).
- `internal/eventlog/internal/domain/event_test.go` → `quarantine_test.go` (quarantine identity, conflict, input rules, operator status) plus new `event_body_test.go` (event body encoding, read limit, payload decoding).
- `internal/eventlog/internal/store/store_test.go` → `fixture_test.go`, `events_test.go`, `read_test.go`, `quarantine_test.go`, `evidence_test.go`, `event_schema_test.go`.
- `internal/eventlog/internal/app/app_test.go` → `fixture_test.go`, `append_test.go`, `read_test.go`, `quarantine_test.go`.
- `TestReadFiltersBeforeLimitAndPreservesLogOrder` and `TestReadStopsOnCallbackFailureAndReleasesConnection` (facade) → store: `TestReadRecordsFiltersBeforeLimitAndPreservesLogOrder`, `TestReadRecordsStopsOnCallbackFailureAndReleasesTheConnection`.
- `TestReadEntityWindowIsScopedOrderedAndStoppable` (facade) → store `TestReadEntityEventsIsScopedByTenantEntityAndInclusiveTimeBounds` (with the bounds, which the facade test never touched); the facade keeps a wiring version, `TestReadEntityWindowIsScopedToTheEntityOrderedByEventTimeAndStoppable`.
- Facade `TestAppendAndRead` → `TestAppendedEventsReadBackInLogOrderWithTheirTraceContext`; `TestAppendDuplicateIgnored` → `TestRedeliveredEventIsReportedAsADuplicateAndNotLoggedTwice`; the three schema tests became one table, `TestAppendUnderRequiredSchemasAppliesTheRegisteredBuiltInCatalog`.

### Improved
- T6: every top-level test and subtest is parallel (the facade's `defer cleanup()` helper `newTestLog` is gone; databases close with the test).
- T4: failures print got and want; error tests assert the wrapped context (`errors.Is`, or the domain message where no sentinel exists). `TestQuarantineRejectsEventIDHashConflict` only checked `err == nil`; the app test now asserts the conflict message and that the rejection (`status = rejected`, `reason = event_id_hash_conflict`) was committed before the error was reported.
- T4: `TestQuarantineLifecycleSQL` (store) tolerated any result; replaced by tests that assert counts, statuses and the stored digest.
- T9: one `fixture_test.go` per layer (`newStore`, `validEnvelope`, `appendOne`, `readAll`, `inUnit`; `newHarness` in app). Table-driven statement-failure tests share `checkUnitFailures`.
- T11: the quarantine lifecycle is split by behavior (bound, conflict, required fields, raw, redrive success, redrive refusal, idempotent redrive).

### Added
Domain (100%): `TestQuarantineReadsItsHeaderFromTheEnvelopeFields`, `TestQuarantineIdentityFollowsPayloadBytes` (reordered keys keep identity; changed value changes it), `TestQuarantineAndReleaseRequireTenantEventAndTime` (six rows), `TestOperatorStatusShowsRedrivenOnlyForReleasedRecords`, `TestEncodeEventBodyNamesWhichPartCannotBeEncoded`, `TestDecodePayloadRoundTripsAndRefusesNonObjects`, `TestCheckPayloadMatchesEachDeclaredJSONType` (20 rows over every supported schema type), `TestEnumRefusesNonStringValuesAndNamesTheAllowedOnes`.

Store (67.1% → 92.2%): `TestInsertEventKeepsTheFirstDeliveryOfAnEventIDPerTenant`, `TestCurrentPositionIsTheTenantsGreatestPosition`, `TestInsertEventStoresEveryOptionalEnvelopeField`, `TestLateArrivingEventKeepsItsEventTimeAndTakesTheNextLogPosition`, `TestReadEntityEventsBreaksEventTimeTiesByLogPosition`, `TestReadEntityEventsPropagatesTheVisitorFailureAndNamesQueryFailures`, `TestUpsertQuarantineCountsRepeatedDeliveriesOfTheSamePayload`, `TestUpsertQuarantineRefusesADifferentPayloadUnderTheSameEventID`, `TestQuarantineRejectsTheEleventhDeliveryOfThePayload` (boundary: 10 stay quarantined, the 11th rejects), `TestQuarantineConflictAndOverflowAreRecordedOnce`, `TestReleaseAllowsRedriveOnceAndOnlyForQuarantinedRecords`, `TestReleaseRefusesRejectedAndUnknownRecords`, `TestReleasedEnvelopeRefusesAnUndecodablePayload`, `TestQuarantinedListsTheTenantsRecordsNewestFirstWithinTheBound` (the 500-record bound), `TestEvidenceEvents*` (3, `EvidenceEvents` was 0%), `TestLoadEventSchemaReturnsOnlyAnActiveRegisteredSchema` (a retired schema is refused), and statement-failure tables for events and quarantine (a dropped table inside a rolled-back unit shows each statement names its failure).

App (71.5% → 94.9%): `TestAppendRefusesAnEnvelopeOfAnotherTenant`, `TestOutOfOrderEventsAreLoggedInArrivalOrderWithTheirOwnEventTime`, `TestAppendStampsTheRecordWithTheInjectedClock`, `TestAppendUnderRequiredSchemasAdmitsOnlyConformingPayloads`, `TestAppendUnderRequiredSchemasRefusesAnUnregisteredOrCorruptSchema`, `TestReadEntityEventsWrapsTheWindowRead`, `TestEveryOperationNamesItsFailureWhenStorageIsGone` (7 operations), `TestReadRebuildsTheFullEnvelopeOfEachRecord`, `TestReadRefusesAStoredDocumentItCannotDecode`, `TestQuarantineCountsRetriesAndRejectsOnceTheBoundIsSpent` (exactly one gap however many deliveries follow), `TestQuarantineRefusesAPayloadItCannotEncode`, `TestQuarantineRawKeepsTheBytesAsDataUnderTheGivenEventID`, `TestRedriveOfAnEnvelopeThatStillFailsAdmissionLeavesTheLogAndTheReleaseUntouched`, `TestRedriveOfARawQuarantineIsRefusedByEnvelopeValidation`, `TestRedriveOfAnAlreadyLoggedEventAppendsNothingAndStillCompletes`, `TestEvidenceEventsLocateLoggedEventsOnly`.

Facade (77.8% → 100%): `TestEventLogWithoutADatabaseRefusesToReportItsPosition`, `TestEventLogStampsRecordsWithItsClock` (injected and defaulted clock), `TestValidateEnvelopeChecksTheDurableRegistryOnlyWhenRequired` (`ValidateEnvelope` was 0%), facade `EvidenceEvents` and `Quarantined` wiring (both 0%).

### Speed
Nothing in this module was slow. The old tests were already about 1.2 s per package of compile and race start-up. The only new cost is the 500-row bound test, built with one recursive-CTE insert instead of 500 units (2.1 s → 0.2 s).

## Production code touched
- none. `internal/eventlog/README.md`: the "Evidence and limits" sentence quoted per-layer coverage percentages that were already stale; it now states the bar without numbers.

## Invariants proven here
- 2 (event time explicit, late data not rewritten): `TestLateArrivingEventKeepsItsEventTimeAndTakesTheNextLogPosition` and `TestReadEntityEventsBreaksEventTimeTiesByLogPosition` (store); `TestOutOfOrderEventsAreLoggedInArrivalOrderWithTheirOwnEventTime` (app). Mutation check: ordering the entity window by position fails the first.
- 8 (stable identities, idempotency): `TestInsertEventReportsDuplicatesAsMinusOneAndPositionsAdvance`, `TestInsertEventKeepsTheFirstDeliveryOfAnEventIDPerTenant` (store; a mutation that rewrites the payload on conflict fails both); `TestAppendReportsDuplicatesAsMinusOne`, `TestAppendValidatesTraceContextEvenForADuplicate` (app); quarantine identity: `TestQuarantineIdentityIsStableAndFallbackDerived`, `TestQuarantineIdentityFollowsPayloadBytes` (domain).
- 8 (quarantine/redrive): `TestQuarantineRejectsTheEleventhDeliveryOfThePayload`, `TestQuarantineConflictAndOverflowAreRecordedOnce`, `TestQuarantineCountsRetriesAndRejectsOnceTheBoundIsSpent`, `TestQuarantineReportsADifferentPayloadUnderTheSameEventIDAfterRejectingTheRecord`, `TestReleasedEnvelopeIsRedrivenIntoTheLogExactlyOnce`, `TestRedriveOfAnEnvelopeThatStillFailsAdmissionLeavesTheLogAndTheReleaseUntouched`, `TestRedriveOfAnAlreadyLoggedEventAppendsNothingAndStillCompletes`.
- 1 (raw events are data): `TestQuarantineRawKeepsTheBytesAsDataUnderTheGivenEventID`, `TestRedriveOfARawQuarantineIsRefusedByEnvelopeValidation`.

## Open items
- `validateJSONSchemaType` answers a `null`-typed property that receives a non-null value with "schema uses unsupported JSON type", because `null` is only handled when the value is nil. The behavior is pinned in `TestCheckPayloadMatchesEachDeclaredJSONType`; the message is misleading but the decision to change it is not a test decision.
- `persistQuarantine`'s `updated == 0` branch (the conflict detected by the upsert after the digest pre-check passed) is only reachable under a concurrent writer; not covered. Same for the `RowsAffected`/`LastInsertId` driver-error branches.
