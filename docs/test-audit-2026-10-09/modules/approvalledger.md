# approvalledger

Status: done
Round: 8

Audited in round 8 with [episodeledger](episodeledger.md). Production code is
unchanged.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/approvalledger` (facade) | 90.9% | 100.0% | 1.7 s | 1.6 s | 6 / 5 | 6 / 0 |
| `internal/approvalledger/internal/app` | 76.0% | 92.0% | 1.5 s | 1.6 s | 4 / 0 | 11 / 3 |
| `internal/approvalledger/internal/domain` | no test files | no test files (constants only; pinned through the store schema test) | - | - | 0 | 0 |
| `internal/approvalledger/internal/store` | 73.2% | 85.7% | 1.4 s | 2.0 s | 4 / 0 | 15 / 15 |

Hygiene lint: 7 findings before, 0 after. Repository `golangci-lint`: 0 issues.
`dupl` at threshold 60: the existing production pair `PendingOfIntent` /
`LatestApprovedOfIntent` (`store/approval_lookup.go`), which is not in AGENTS.md's
known-duplication list; untouched (production, and the two differ only in query,
status and message). No test duplicates. `time.Sleep`, `time.Now()`, `t.Skip`:
none. `go test -race -shuffle=on -count=3`: passes. Architecture gates: pass.

Mutation checks: dropping `AND status = 'pending'` from `ExpirePending`, `<` to
`<=` in `SupersededApprovals`, and dropping the `approval_id DESC` tie-break each
fail named tests.

## Findings and changes

### Removed
- `TestEveryTransitionStartsFromPending` (store): covered one wrong-state repeat
  (`WithdrawPending` on an approved row) and asserted that five calls returned no
  error. Replaced by `TestOnlyAPendingApprovalCanTransition` (4 start states x 6
  transitions compare the whole row before and after) and
  `TestEachTransitionRecordsItsOwnColumnsAndStableReason`.
- `TestLatestApprovedOfIntentPicksOneRowAndBreaksDecidedAtTiesByLargerID` and the
  lookup half of `TestPendingLookupsReadTheUnresolvedApprovalOnly` (facade): the
  tie-break is SQL, so it is proven in store
  (`TestTheLatestApprovedApprovalIsTheLastDecidedAndTiesTakeTheLargerId`); the
  facade keeps one lookup journey including `LatestApprovedOfIntent` (was 0%).
- `TestApprovalsReadsEveryRequestOfAnIntent` (store) one-row version, and the
  `ExpireIntent` call with no assertion at the end of the app test.

### Renamed or moved
- facade `lifecycle_test.go` → `fixture_test.go` (helpers) + `lifecycle_test.go`;
  `supersession_test.go` → `withdrawal_publication_test.go`;
  `approval_lookup_test.go` → `pending_lookup_test.go`.
- `TestApprovalTransitionsPreserveTerminalStateAndAssertionBinding` →
  `TestAnApprovalKeepsItsDecisionAndAssertionBindingThroughLaterTransitions`;
  `TestSupersededWithdrawalAndNotificationShareTransaction` → `...ShareTheCallersTransaction`;
  `TestWithdrawalReadsClockAfterEachOrderedMutation...` kept.
- app `app_test.go` → `fixture_test.go` + `supersession_test.go`;
  `approval_views_test.go` → `approvals_test.go`; lifecycle reasons →
  `lifecycle_test.go`.
- store `store_test.go` → `fixture_test.go`; `approval_lookup_test.go` →
  `lookup_test.go`; `approval_views_test.go` → `views_test.go`; new
  `transition_test.go`, `supersession_test.go`, `schema_test.go`.

### Improved
- T6: facade and store tests parallel; the shared-database subtests of the old
  lookup test now have one database each.
- T4: the old app test `TestAFailedPublicationFailsTheWithdrawalAndNilPublisherIsRefused`
  proved the nil publisher but not that nothing was withdrawn first; split into
  `TestAFailedPublicationFailsTheWithdrawalAndNamesTheApproval` and
  `TestAWithdrawalWithoutAPublisherIsRefusedBeforeAnythingIsWithdrawn` (asserts the approval
  stays pending). `TestSupersededApprovalsAreWithdrawnThenPublishedInOrder` now also
  asserts the publisher runs on the withdrawal's own transaction and that the
  replacement version's approval is untouched.
- T9: one seeding helper per layer (`seedSituation`, `seedIntent`/`seedDecision`,
  `approvalDB`) instead of copies per file.

### Added
Store (73.2% to 85.7%): `TestARequestedApprovalIsPendingWithItsDocumentAndSingleUseNonce`,
`TestAnIntentHasOnePendingApprovalAtATimeAndApprovalIdsAreUnique`,
`TestEachTransitionRecordsItsOwnColumnsAndStableReason` (6 transitions),
`TestOnlyAPendingApprovalCanTransition`, `TestATransitionOfAnUnknownApprovalChangesNothing`,
`TestAnApprovalStatusOutsideTheLifecycleIsRefusedByTheSchema`,
`TestApprovalStatusesMatchTheSchemaCheck`, `TestAPendingApprovalBindsItsExpiryAndNonceUntilItIsDecided`,
`TestSupersededApprovalsAreThePendingOnesBoundToAnOlderVersionOfTheSituation`
(only pending, only strictly older, only that Situation, id order, null trace context),
`TestEveryApprovalRequestOfAnIntentIsReadOldestFirstWithHowItEnded`,
`TestAnIntentWithoutApprovalRequestsHasNoApprovalViews`, `TestReadingApprovalsFromAClosedDatabaseNamesTheRead`.

App (76.0% to 92.0%): `TestOnlyApprovalsBoundToAStrictlyOlderVersionAreSuperseded` (3 rows),
`TestWithdrawingNothingNeverCallsThePublisher`, `TestAWithdrawnApprovalCannotBeApprovedAfterwards`,
`TestAnExpiredApprovalCannotBeWithdrawnOrResolved`, `TestAnApprovalReadFailureNamesTheIntent`,
`TestAnIntentsApprovalIsLookedUpWhilePendingAndBoundToItsNonce`.

Facade (90.9% to 100%): `TestEveryApprovalOfAnIntentIsExplainableThroughTheFacade`
(`Approvals` was 0%: an expired and a withdrawn approval read back with their
reasons and times).

### Speed
- Nothing was slow.

## Production code touched
- none.

## Invariants proven here
- 8 (stable identities, idempotency): a decided approval is never revived and a
  repeated transition changes nothing (`TestOnlyAPendingApprovalCanTransition`,
  `TestAWithdrawnApprovalCannotBeApprovedAfterwards`, `TestAnExpiredApprovalCannotBeWithdrawnOrResolved`);
  one pending approval per intent and unique approval ids
  (`TestAnIntentHasOnePendingApprovalAtATimeAndApprovalIdsAreUnique`); the nonce is single-use
  (`TestAPendingApprovalBindsItsExpiryAndNonceUntilItIsDecided`); withdrawal and its
  notification commit or roll back together
  (`TestSupersededWithdrawalAndNotificationShareTheCallersTransaction`,
  `TestWithdrawalReadsClockAfterEachOrderedMutationAndRollsBackOnPublishFailure`).
- 10 (explainable from durable records): `TestEveryApprovalOfAnIntentIsExplainableThroughTheFacade`,
  `TestEveryApprovalRequestOfAnIntentIsReadOldestFirstWithHowItEnded`,
  `TestEachTransitionRecordsItsOwnColumnsAndStableReason` (expired: `approval_expired`; withdrawn:
  `approval_withdrawn` + `situation_version_conflict` + `withdrawn_at`),
  `TestLifecycleTransitionsUseTheStableReasons`.
- Expiry: the ledger never reads a clock; `Expire` takes the instant from the
  caller, so every expiry test passes a literal instant (the withdrawal clock test uses
  `sources.NewVirtual`).

## Open items
- `ExpireIntent` (an approval expiring because its intent expired) writes only
  `status = 'expired'`: no `decided_at` and no `reason`, unlike `Expire`. An
  approval ended this way cannot say when or why from its row (invariant 10).
  `TestEachTransitionRecordsItsOwnColumnsAndStableReason/expire_with_the_intent`
  pins the current columns; the fix is a design decision (production change).
- `Resolve` passes the caller's status string to SQL: any value the schema allows is
  stored, including `pending`, which stamps `decided_at` and the approver but leaves the
  approval pending. Only `approved` and `denied` are meaningful. `domain` has no
  status validation; the policy module is the only caller.
- A transition of a decided or unknown approval is a silent no-op that returns nil, by
  design (README); callers cannot tell a refused transition from an applied one.
  Pinned by `TestOnlyAPendingApprovalCanTransition` and
  `TestATransitionOfAnUnknownApprovalChangesNothing`.
- `internal/approvalledger/internal/domain` has no test file: it holds constants and
  value types only; the statuses are pinned against the schema CHECK in the store.
