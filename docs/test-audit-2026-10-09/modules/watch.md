# watch

Status: done
Round: 12

Audited in round 12 with [actions](actions.md). Production code is unchanged.
Every install, fire, expiry and refusal path of a derived-trigger watch now has
a named, parallel test; the expiry boundary is checked to the nanosecond at the
store and through the service.

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/watch` (facade) | 80.0% | 100.0% | 1.8 s | 1.4 s | 2 / 0 | 2 / 0 |
| `internal/watch/internal/app` | 87.5% | 89.4% | 2.8 s | 2.4 s | 14 / 2 | 22 / 14 |
| `internal/watch/internal/domain` | 97.1% | 97.1% | 1.5 s | 1.2 s | 10 / 16 | 10 / 16 |
| `internal/watch/internal/store` | 77.5% | 77.5% | 2.1 s | 1.7 s | 9 / 4 | 10 / 8 |

No test is above 1 s (the slowest is the cancellation test, bounded by its 100 ms
deadline plus opening an on-disk database).

Hygiene lint: 8 findings before (7 paralleltest, 1 tparallel), 0 after. Repository
`golangci-lint`: 0 issues. `time.Sleep` in tests: none. `time.Now()`: none (the old
retry test's elapsed-time assertion is gone). `go test -race -shuffle=on -count=3`: passes.

Mutation checks: `expires_at >= ?` instead of `> ?` in the store's unexpired predicate fails
`TestAWatchIsVisibleUpToItsExpiryAndHiddenAtIt` (it survived the service tests, because a fire
runs `ExpireDue` first; the store test now pins it).

## Findings and changes

### Removed
- `TestWatchEffectorSkipsCELEvaluationErrorAndFiresWhenDataArrives`, `...FiresTamozFallbackFromEventFeatures`
  and the fire half of `...IsBoundedExpiringAndOneShot` as separate setups: kept as rows of
  `TestAWatchFiresOnlyWhenItsExpressionHoldsForTheEventFeatures` and
  `TestAnExpressionThatCannotBeEvaluatedIsASkippedNoFireUntilItsDataArrives`.
- The `elapsed < 100ms` assertion of the busy-retry test (T5): it measured wall time; the test now
  proves the lock was still held when `Expire` started and was released before it succeeded.
- `TestServiceInstallsFiresAndExpiresThroughTheFacade` asserted only `err == nil` after `Expire`
  (T4); replaced by a journey that reads the watch back.

### Renamed or moved
- `internal/app/watch_test.go` is `fire_test.go` and `expire_test.go`; `service_test.go` is
  `install_test.go` (constructor, ownership, interlock, route, idempotency, authorized dispatch);
  `fault_test.go` is `write_boundary_test.go`; helpers are in `fixture_test.go`.
- `internal/store/store_test.go` is `fixture_test.go`, `transaction_test.go`, `conditions_test.go`.
- `TestWatchEffectorExpireRetriesAfterSQLiteBusy` is `TestExpiringRetriesWhileTheDatabaseIsBusyAndSucceedsOnceItIsReleased`;
  `TestWatchExpireHonorsCancellationWhileContended` is `TestExpiringHonorsCancellationWhileTheDatabaseStaysBusy`.
- `TestServiceInstallsFiresAndExpiresThroughTheFacade` is `TestAWatchIsInstalledFiredAndReadBackThroughTheFacade`.

### Improved
- T6: all tests parallel (8 findings to 0).
- T5: a virtual clock (`sources.NewVirtual(fixtureNow)`) drives every expiry; the physical
  clock is used nowhere.
- T9: one `newService`/`newServiceAt`/`newServiceOwnedBy`, one `watchCommand`, one `readWatch`
  returning `{Status, Remaining, Fires}`.

### Added
App: `TestAWatchFiresOncePerEventAndOnlyWithinItsAllowance` (duplicate delivery absorbed, spent
watch disabled and not revived by a reinstall), `TestAWatchIgnoresEventsOfAnotherSituationOrTarget`,
`TestOneEventFiresEveryMatchingWatchOfItsTarget`, `TestAWatchFiresUpToItsExpiryAndNotAtIt`,
`TestExpiringLeavesActiveAndSpentWatchesAlone`, `TestATrippedInterlockRefusesBothInstallingAndFiring`,
`TestAWatchIsIdentifiedByTheIdempotencyKeyBeforeTheCommandID`,
`TestAuthorizedDispatchRefusesWithoutAPassingFinalCheckAndInstallsNothing`.
Store: `TestAWatchIsVisibleUpToItsExpiryAndHiddenAtIt`.
Facade: the journey reads the watch and its fire through `watch.Watch` and `InstalledWatchID`
and checks another tenant cannot read it (`Watch`, `InstalledWatchID` were uncovered).

### Speed
- Nothing was slow.

## Production code touched
- none.

## Invariants proven here
- 8: `TestAWatchFiresOncePerEventAndOnlyWithinItsAllowance` (a fire is recorded once per event
  identity), `TestReinstallingTheSameWatchIsIdempotentAndAConflictingOneIsRefused`,
  `TestAWatchIsIdentifiedByTheIdempotencyKeyBeforeTheCommandID`,
  `TestAFireWriteFailureRecordsNothingAndSpendsNoAllowance` (fire record and allowance commit together).
- 7: `TestATrippedInterlockRefusesBothInstallingAndFiring`, `TestInstallRefusesWithoutRuntimeOwnership`,
  `TestAuthorizedDispatchRefusesWithoutAPassingFinalCheckAndInstallsNothing`.
- 1: expression syntax and size limits are domain rules (`TestValidateExpressionRefusesForbiddenSyntaxAndInvalidCEL`,
  `TestConditionFromCommandRefusesInvalidPayloadsInPrecedenceOrder`): a watch payload is data checked
  against a bounded grammar, never executable code.

## Open items
- `assertGuards(ctx, tx, tenantID, target)` in `internal/app/install.go` ignores `tenantID` and
  `target`. Dead parameters; left alone (production, behavior-preserving rounds only).
- `TestExpiringRetriesWhileTheDatabaseIsBusyAndSucceedsOnceItIsReleased` releases the lock from a 50 ms
  real timer. The retry backoff (25 ms, then doubling) is a real timer inside `storage.RetrySQLiteBusy`
  and `BEGIN` itself fails busy, so no hook inside the attempt is reachable; the margin is 15 times
  below the retry budget. It is bounded by `t.Context()`, not a sleep.
- Store coverage is 77.5%: the rest is database-failure wrapping.
