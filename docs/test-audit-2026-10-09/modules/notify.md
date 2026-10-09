# notify

Status: done
Round: 12

Audited in round 12 with [actions](actions.md). Production code is unchanged.
`internal/notify/internal/store` rose from 68.0% to 93.0%: its untested methods
were real behavior (the winner of a raced insert, the retirable count, tenant
isolation) plus every operation's error context.

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/notify` (facade) | 100.0% | 100.0% | 1.7 s | 1.8 s | 14 / 8 | 14 / 8 |
| `internal/notify/internal/app` | 73.5% | 81.8% | 1.5 s | 1.9 s | 6 / 0 | 10 / 5 |
| `internal/notify/internal/domain` | 89.9% | 89.9% | 1.3 s | 1.2 s | 16 / 34 | 16 / 44 |
| `internal/notify/internal/store` | 68.0% | 93.0% | 1.5 s | 2.0 s | 7 / 0 | 13 / 16 |

No test is above 0.55 s. Hygiene lint: 0 findings before and after. Repository
`golangci-lint`: 0 issues. `time.Sleep`, `time.Now()`, `t.Skip` in tests: none (the one
`time.Now()` in `lifecycle_contract_test.go` is now the fixed `sealNow`).
`go test -race -shuffle=on -count=3`: passes. The experiment goldens pin
(`TestExperimentNotificationGoldensAreUnchanged`) is untouched and passes, so the
real-world-sensor experiment's notification contract is unchanged.

## Findings and changes

### Removed
- Nothing was dead; the facade keeps one journey per family (reading, appending, lifecycle,
  retention) and the detailed rules moved down, so the facade tests stayed.

### Renamed or moved
- Facade `notify_test.go` (304 lines) is `fixture_test.go`, `reading_test.go`, `append_test.go`,
  `retention_test.go`, and its lifecycle tests joined `lifecycle_events_test.go`.
- App `app_test.go` is `fixture_test.go`, `append_test.go`, `read_test.go`, `prune_test.go`.
- Store `store_test.go` is `fixture_test.go`, `transaction_test.go`, `append_test.go`,
  `read_test.go`, `retention_test.go`, `failure_test.go`.
- `TestStoreReportsConfiguration` is `TestAStoreIsConfiguredOnlyWithItsDatabase`;
  `TestJoinedTransactionKeepsCallerOwnership` is `TestAJoinedTransactionStaysUnderTheCallersOwnership`;
  `TestInsertFindAndReadRowsInCursorOrder` is split into `TestAnIdentityIsInsertedOnceAndFoundByItsCursorAndDigest`
  and `TestRowsAreReadAfterACursorInOrderUpToTheLimitWithinOneTenant`.

### Improved
- T4: the poison test now covers three corruptions (digest mismatch, undecodable JSON with a good
  digest, decodable but not an event) and asserts the neighbors on both sides are delivered, the
  cursor advances past the skipped record and the counter is cleared with exactly one audit.
- T4: domain lifecycle validation cases are subtests, so a failing row names itself.
- T5: the one `time.Now()` is a fixed instant.

### Added
Store (68.0% to 93.0%): `TestTheWinnerOfARacedInsertIsReadByDigestAndCursor` (`StoredSHA`,
`NotificationCursor`, were 0%), `TestCursorsAreIndependentPerTenant`,
`TestRetiringTwiceKeepsTheFirstTombstone`, `CountNotificationsBefore` inside the retention test,
`TestEveryOperationNamesItselfWhenTheDatabaseFails` (16 operations on a closed database, each
error asserted to carry its own context), tenant isolation in reads and finds.

App (73.5% to 81.8%): `TestAppendingTheSameEventAgainReturnsItsCursorAndADifferentPayloadIsRefused`
(`existingCursor` conflict branch was 50%), `TestAnIncompleteLifecycleRequestIsRefusedBeforeAnythingIsStored`,
`TestAPageLimitOutsideItsBoundsIsRefusedWithoutAnAudit`,
`TestPruneAndPrunableRefuseRetentionBelowTheFloor`,
`TestPrunableCountsOnlyNotificationsOlderThanTheRetentionWithoutRetiringThem` (`Prunable` was 0%).

### Speed
- Nothing was slow.

## Production code touched
- none.

## Invariants proven here
- 8: `TestAppendingTheSameEventAgainReturnsItsCursorAndADifferentPayloadIsRefused` (app),
  `TestAppendDuplicatesAndConflictsDoNotConsumeCursors` (facade),
  `TestCursorsAreGaplessAcrossReleasedAllocations`, `TestRacedInsertReleasesItsCursorOnlyForAnIdenticalWinner`,
  `TestAppendRefusesRetiredIdentityAfterPrune`, `TestPruneIsOneTransaction`.
- 10: `TestRefusalsAreAuditedWithTheRequestedAndOldestCursors` (expired cursor, slow subscriber),
  `TestAMalformedRecordFailsThePageUntilItsBudgetIsSpentThenIsSkippedAndAudited`,
  `TestNotificationsAreCursorResumableAndAuditExpiredCursor`: every refusal and skip leaves an audit row.

## Open items
- `internal/notify/internal/domain/lifecycle_identity.go` lines 24-36 are a production structural
  twin at `dupl` threshold 60; not touched.
- Remaining uncovered store code is `RowsAffected` error handling that SQLite cannot produce.
- App is at 81.8%: the uncovered code is database-failure wrapping in `retire` and `publish`.
