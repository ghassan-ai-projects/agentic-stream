# control

Status: done
Round: 13

`internal/control` is the runtime control plane: owner lease, epoch drain/kill,
the final dispatch readiness gate and cost control. The facade tests now state
the four behaviors by name: a lost lease fences the old epoch
(`TestALostLeaseFencesTheOldEpochsRenewalsAndWrites`), a killed epoch refuses
admission, work and decisions and supersedes its in-flight episodes
(`TestAKilledEpochIsTerminalAndRefusesAdmissionWorkAndDecisions`,
`TestKillingAnEpochRecordsItAndSupersedesItsInFlightEpisodesAtomically`), a cost
ceiling refuses a reservation (`TestACeilingRefusesAReservationThatWouldExceedItAndAZeroEstimateUnderAnyCeiling`),
and the kill switch stops every new reservation
(`TestTheKillSwitchRefusesEveryNewReservationButNotTheSettlementOfHeldOnes`).
Lease tests use `sources.NewVirtual`, not a mutated `time.Time` variable.

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `control` (facade) | 95.8% | 100.0% | 2.9 s | 1.9 s | 27 (22) | 37 (25) |
| `control/controltest` | 66.7% | 100.0% | 1.6 s | 1.4 s | 1 (1) | 2 (2) |
| `control/internal/app` | 71.8% | 79.8% | 2.0 s | 1.6 s | 9 (9) | 12 (12) |
| `control/internal/domain` | 100.0% | 100.0% | 1.3 s | 1.1 s | 10 (7) | 10 (7) |
| `control/internal/store` | 70.8% | 79.2% | 2.3 s | 1.7 s | 21 (10) | 24 (13) |

Test-hygiene lint: 15 findings (all in the facade) to 0. `golangci-lint` 0 issues;
`dupl` at 60: 0. `-race -shuffle=on -count=3` passes. Slowest test 0.65 s
(race start-up of a migrated database). `time.Sleep`: none.

## Findings and changes

### Removed
- `TestZeroEstimateIsRejectedByTenantCeiling`: merged into
  `TestACeilingRefusesAReservationThatWouldExceedItAndAZeroEstimateUnderAnyCeiling`.
- `TestEpochControlDecisionPathFailsClosedWhenMisconfigured` and the tail of
  `TestEpochControlStateMachine` (ten `err == nil` checks): one table in
  `TestAnEpochControlThatCannotNameAnEpochFailsClosed` asserting the
  "epoch control is not configured" refusal.
- `TestRuntimeRecoveryRechecksLeaseBeforeCommitting` and
  `TestRecoveryCannotCommitAnExpiredOwnerLease`: the same fence reached two ways,
  now two rows of `TestRecoveryCannotCommitOnceTheOwnerLeaseIsLost`.

### Renamed or moved
- `maintenance_test.go` → `epoch_state_test.go` (split into three behaviors:
  uncontrolled, draining, killed).
- `recovery_fence_test.go` → `owner_recovery_test.go`; `cost_reservation_test.go`
  → `cost_kill_switch_test.go`.
- `internal/app/app_test.go` (234 lines) → `owner_test.go`, `epoch_test.go`,
  `cost_test.go`, `unconfigured_test.go`, `fixtures_test.go`.
- `internal/store/store_test.go` (248 lines) → `owner_test.go`, `epoch_test.go`,
  `cost_test.go`, `dispatch_test.go`, `fixtures_test.go`.

### Improved
- T5/T6: all 15 unparallel tests are parallel; `time.Now()` in the dispatch gate
  test replaced by a fixed instant; lease expiry advances a virtual clock.
- T4: not-configured refusals, settlement conflicts and missing reservations now
  assert the message instead of `err != nil`.
- T9: shared `fixtures_test.go` (`openOwnerDB`, `ownerOn`, `fenced`,
  `assertCostTotals`, `seedEpisode`); `fenced(t, db, owner.Assert, epoch)`
  replaces two near-identical helpers flagged by `dupl`.

### Added
- `TestOwnerClaimsRenewsAndReleasesTheLeaseBeforeAnotherEpochClaimsIt` (a released
  lease can no longer be asserted), `TestAnOwnerThatDoesNotRenewLosesTheLeaseWhenItExpires`,
  `TestAnUnconfiguredOwnerRefusesToClaimAndFencesNothing`.
- `TestRecoveryThatFailsRollsBackTheNewClaimAndKeepsTheOldOwner`.
- `TestKillingAnEpochReleasesTheCostReservationOfAnAdmittedEpisodeThatNeverStarted`
  now reserves through the ledger (global and tenant totals asserted to 0).
- `TestDispatchGateWithoutADatabaseRefusesDispatch` (message asserted).
- app: `TestClaimingTheSameEpochAgainRenewsTheLeaseForTheSameInstance`,
  `TestAReservationNeedsTheGlobalLimitRowButNotATenantOne`,
  `TestSettlementThatReachesACeilingTripsItsKillSwitch`,
  `TestApplyingCeilingsMergesTheGlobalLimitAndLeavesUnsetValuesAlone`,
  and a table over every unconfigured operation (nil owner, nil epochs, closed
  transaction, unconfigured dispatch).
- store: `TestAssertInterlockReadsTheDurableInterlockInOneTransaction` (0% before),
  `TestUnstartedReservedEpisodesListsOnlyAdmittedEpisodesOfTheEpochHoldingAReservation`
  (running, unreserved, settled and other-epoch episodes excluded),
  `TestSupersedeEpochEndsOnlyTheInFlightEpisodesOfTheEpoch`, and the tripped
  kill-switch refusal in `TestCostLimitsReservationsAndSettlement`.
- controltest: `TestSetCostLimitNamesTheScopeItCouldNotWrite` (the error wrap).

### Speed
No slow tests. Facade times fell about 1 s because every test now runs in
parallel.

## Production code touched
- none.

## Invariants proven here
- 7 (final revalidation before dispatch): `TestDispatchGateRefusesOnceTheInterlockTripsAndNeverWritesTheInterlock`,
  `TestAssertInterlockReadsTheDurableInterlockInOneTransaction`.
- 5 (finite budget): the cost ceiling, kill-switch and settlement tests above,
  plus rollback atomicity (`TestATenantCeilingRefusalRollsBackTheGlobalAccountingOfTheReservation`,
  `TestSettlementRollsBackAllAccountingWhenTheTenantWriteFails`).
- Single-writer fence behind invariants 4 and 8: `TestALostLeaseFencesTheOldEpochsRenewalsAndWrites`,
  `TestRecoveryCannotCommitOnceTheOwnerLeaseIsLost`.

## Open items
- `control/internal/app` (79.8%) and `internal/store` (79.2%) miss only
  wrap-only database error returns; not covered with assertion-free tests.
- The episode seed (`seedEpisode`) is copied in the facade and the store tests.
  A shared `controltest.SeedEpisode` was written and removed: the architecture
  gates (`TestModuleSQLStaysInStore`, durable-mutation ownership) forbid `INSERT
  INTO episodes` in a non-test file, and the repository convention is a seed per
  test package. This joins the known duplication list.
