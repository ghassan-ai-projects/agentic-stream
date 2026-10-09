# policy

Status: done
Round: 11

Audited in round 11 with [authority](authority.md). Production code is
unchanged. The earlier, unverified partial work of this round was not used.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running (load average 5 to 9). Tests are top-level tests /
passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/policy` (facade) | 85.2% | 100.0% | 1.5 s | 1.4 s | 6 / 3 | 11 / 6 |
| `internal/policy/internal/app` | 79.5% | 81.9% | 8.5 s | 3.4 s | 31 / 49 | 31 / 63 |
| `internal/policy/internal/domain` | 92.9% | 97.6% | 1.3 s | 1.2 s | 19 / 32 | 31 / 79 |
| `internal/policy/internal/store` | 80.2% | 89.9% | 1.7 s | 2.5 s | 8 / 0 | 38 / 11 |

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 58 findings before
(54 paralleltest, 4 tparallel), 0 after. Repository `golangci-lint`: 0 issues.
`dupl` at threshold 60: 0. `time.Sleep`, `context.Background()`, `t.Skip` in
tests: none. `go test -race -shuffle=on -count=3`: passes. Architecture gates
and `make docs-check`: pass.

Mutation checks (each reverted): `>` to `>=` in `MateriallySuperseded`,
`After` to `Before` in `ApprovalDisposition`, `<` to `<=` in the hourly
dispatch counter SQL, `AssertInterlock` replaced by `return nil`; every one
fails named tests.

## Why `internal/app` took 8.5 s, and the fix

- Cause 1: not one test was parallel, and the package had about 80 database
  tests (every subtest opens its own migrated database, about 0.1 s each under
  `-race`), so they ran back to back. Fixed by `t.Parallel()` everywhere
  (nothing is shared between tests): the same work now runs on all cores.
- Cause 2: most approval tests (about 35 test cases) built the full runtime
  pipeline and HTTP handler (`openApprovalHTTP`) although they only needed the
  database, the policy service and the approval id. Those that exercised
  policy rules through HTTP now use a direct `pendingApproval` fixture (database and service only), and
  HTTP keeps one integration path per concern (commit once, 403 mapping, runtime
  clock, concurrency, atomic rollback, owner and epoch fences).
- Cause 3: the fixture seeded 12 rows in 12 autocommit transactions; it is one
  transaction now (app and store fixtures).
- Cause 4: `TestCommittedApprovalDispatchesWithoutNewSensorInput` slept in a
  10 ms loop. It now polls on a ticker bound to a context with a 5 s timeout.
  It took 1.1 to 1.4 s because `Pipeline.Start` maintained the outbox on a
  fixed 1 s ticker. Round 15 added `PipelineConfig.MaintenanceInterval` (unset
  keeps 1 s) and this test now sets 10 ms: 1.24 s to 0.26 s, and the package
  from about 2.7 s to about 1.7 s.

## Findings and changes

### Removed
- `TestGatewayFailsClosedWhenInterlockTripped`, `TestGatewayDeniesConsequentialIntentWhenCompletenessIsProvisional`,
  `TestEvaluateIntentDeniesHighRiskDespiteRequiresApproval`,
  `TestEvaluateIntentHonorsTheCatalogApprovalPolicy`, `TestGatewayRiskFreshnessAndExpiry`
  (app): five single-scenario tests of the same pass; merged into the one
  revalidation table below (T2).
- `TestRiskRulesRemainAuthoritative` (domain): it computed its expectations
  with a switch that mirrored the rule; replaced by an explicit expectation
  table (DUP-001 below).
- `TestRuleBoundaries`, `TestIntentDigestChecksOriginalDocument`,
  `TestTypedIntentRetainsDigestInput`, `TestApprovalPrincipalAndSignatureRules`,
  `TestFreshnessAndApprovalPrecedence`, `TestDefinitionAndAssertionCanonicalBytes`,
  `TestWithdrawnApprovalIsNeverWithdrawnAgainByThePolicyPath` (domain): each
  proved five to ten unrelated rules in one body with `t.Fatal("rule name")`.
  Split into the tables listed under Added.
- `internal/policy/internal/domain/instant_pins_test.go`: moved into
  `definition_test.go` and `commands_test.go`, next to the code each pin guards.
- `TestApprovalLedgerAndReadProjections`, `TestJoinedTransactionPersistsOnlyOnCallerCommit`
  (store, about 100 and 70 lines): one scenario touching 25 methods with no
  assertion on most; replaced by one test per behavior.
- The `HTTPRejectsInvalidDecisions...` cases `relay inactive`, `approver inactive`,
  `principal`, `decision` and the `health`, `stale`, `interlock` cases of
  `TestHTTPResolutionRechecksCurrentStateAndTrustedClock`: the same rules are
  now proven once, directly, with the result and reason asserted instead of
  "HTTP 200 and no command" (T2, T4).
- `var _ = sql.ErrNoRows` (an import keeper) and a stray blank line.

### Renamed or moved
- app `policy_test.go` → `evaluation_test.go`; `policy_command_test.go` →
  `interlock_test.go`; `config_test.go` → `fixture_test.go` (all test helpers
  in one place); `approval_order_test.go` → `approval_resolution_test.go`;
  new `approval_dispatch_test.go`, `pending_intent_test.go`.
- All `Gateway` test names (the retired word, see UBIQUITOUS_LANGUAGE) became
  sentences about the Service or the behavior; the "P4"/"A-041 F1" ticket
  comments on tests are gone (T3).
- facade `api_test.go` → `definition_test.go`; `policy_test.go` →
  `service_test.go`; new `intent_views_test.go`.
- domain tests now sit in the file named for the source they prove: `routing_test.go`
  (routing.go: risk route, freshness, approval disposition, compensation),
  `approval_checks_test.go` (the principal and signature checks in routing.go),
  `governance_documents_test.go`, `documents_test.go`, `rules_test.go`,
  `definition_test.go`, `commands_test.go`, `approval_notice_test.go`,
  `workflow_test.go`, `principals_test.go` (the old `routing_test.go` held
  document parsing and the old `rules_test.go` held routing).
- store `transaction_test.go`, `reads_test.go` → `commands_test.go`,
  `approval_lifecycle_test.go`, `approval_principals_test.go`,
  `approval_context_test.go`, `intent_reads_test.go`, `intent_views_test.go`,
  `pending_test.go`, `principal_writes_test.go`, `tx_test.go`.
- `internal/policy/APPROVAL_INTEGRATION.md`: two live links updated to the new
  test files (dated archives under `docs/` keep their historical file names).

### Improved
- T6: every test and subtest is parallel (58 findings to 0).
- T5: the sleep loop became a ticker bound to a timeout context.
- T4: `t.Fatal(rec.Code, ...)` and `t.Fatal(result, err)` without got/want became
  messages that state what was expected; error tests assert the sentinel
  (`ErrApprovalUnauthorized`, `ErrApprovalNotFound`, `sql.ErrTxDone`,
  `interlock.ErrTripped`) or the domain message.
- T9: one fixture family per layer (`openPolicyFixture`, `openPendingApproval`,
  `scalar`, `exec`, `execIn`, `inTx`); data changes between "proposed" and
  "evaluated" are named SQL constants (`tripInterlock`, `newerMaterialVersion`,
  ...) instead of closures.
- T11: no test body above about 50 lines.

### Added
App (79.5% to 81.9%):
- `TestAnIntentWhoseStoredRecordsDoNotVerifyIsDeniedBeforeAnyCommand` (5 cases:
  rejected decision, decision digest, intent digest, non-intent bytes, row/document
  identity): invariant 1, stored documents are evidence, not instructions.
- `TestACompensatingIntentMustNameACommandOfItsOwnTenant` (missing, foreign
  tenant, own tenant).
- `TestResolvingAnApprovalRequiresActiveAuthorizedPrincipalsAndAValidSignature`
  (7 refusals, each leaving the approval pending, no assertion bound, no command).
- `TestResolvingAnApprovalRevalidatesTheIntentAgainstCurrentState`,
  `TestADeniedApprovalIsFinalAndNeverCommands`,
  `TestResolvingAnApprovalAgainReportsItAlreadyResolved`.
- `TestHTTPResolutionUsesTheRuntimeClockNotTheCallers`.

Domain (92.9% to 97.6%): `TestRiskRouteForEveryRiskClassAndCatalogApproval`,
`TestApprovalDispositionOrdersResolvedThenStaleThenExpired` (11 cases; pins
that a decline of a superseded intent is still authorized),
`TestCompensationFailureNamesAMissingOrForeignTarget`,
`TestAnIntentRowMustMatchItsSignedDocument`,
`TestIntentValidationPrecedenceAndBinding`,
`TestChangingAnySignedFieldChangesTheSigningBytes`,
`TestAssertionSignatureBindsTheExactBytesAndKey`,
`TestAnApprovalNotificationMustBeBoundToItsTenantAndSource` (`CheckBinding` was
0%), `TestAnOutcomeAuditsItsDetailAndOtherwiseItsStableReason` (`AuditReason`
was 0%), `TestAPrincipalKeyIsAnEd25519KeyOrAbsent`,
`TestAPrincipalIsActiveUnlessTheDocumentSaysOtherwise` (`EffectiveStatus` was 0%).

Store (80.2% to 89.9%): `TestTheHourlyDispatchBudgetIsPerTenantTypeAndHour`,
`TestApprovalAuthorityNeedsAnActiveRoleGrantForTheEntityAndRisk` (6 cases),
`TestTheApprovalDeltaIsTheTriggersDeltaObjectOrEmpty`,
`TestTheNextPendingIntentIsTheTenantsOldestUnevaluatedOneAndTiesBreakById`,
`TestReplacingGovernanceNeverTakesOverAnotherTenantsPrincipal`,
`TestAGrantToAnUndeclaredPrincipalIsRefusedAndRolledBack`,
`TestEveryJoinedWriteRollsBackWithTheCallersTransaction`,
`TestTheCurrentSituationVersionsCompletenessIsReadFromThatVersion` (pins that a
current version without a snapshot reads as empty completeness, which makes a
consequential intent fail closed), `TestAnUnparseableIntentExpiryIsFlaggedNotFatal`,
and the tenant-scoping tests for approvals and intent views.

Facade (85.2% to 100%): `TestGovernanceSummaryCountsOnlyTheTenantsStoredGovernance`
(`GovernanceSummary` was 0%), `TestApplyPrincipalsRequiresTheRuntimeOwnerFence`
(no check, no epoch, lease held elsewhere, each leaving governance empty),
`TestIntentReadsAreTenantScopedAndNameWhatTheyReadWhenNothingIsStored`
(`Intent` and `DecisionIntents` were 0%), `TestParsePrincipalsNamesTheDocumentWhenItIsInvalid`,
`TestNewBuildsAServiceFromACompleteConfiguration`.

### Speed
See "Why `internal/app` took 8.5 s". Store went from 1.7 s to 2.5 s because it
holds four times the tests (each on its own database); no test is above 0.3 s.

## Production code touched
- none.

## Invariants proven here
- 7 (policy revalidates every Intent against current state immediately before
  dispatch). The single table is `TestEvaluationRevalidatesTheIntentAgainstCurrentState`
  (app): stale Situation version, a non-material newer version that stays
  fresh, expired intent, tripped interlock, episode that produced no decision,
  provisional source health on a consequential intent, risk class requiring
  approval (R2), catalog `requires_approval` on R1, and R3/R4 denial that the
  catalog flag cannot soften; each asserts the result, the reason and that no
  command or outbox row exists. At the human gate:
  `TestResolvingAnApprovalRevalidatesTheIntentAgainstCurrentState` (newer
  material version withdraws the approval, expired approval, tripped interlock,
  incomplete source health: an approved human decision still yields no command),
  `TestApprovalStalenessPrecedesExpiryAndAuthorization`,
  `TestResolvingAnApprovalRequiresActiveAuthorizedPrincipalsAndAValidSignature`
  (revoked authority, disabled principals, bad signature),
  `TestAnApprovalExpiresExactlyAtItsDeadline`, `TestADecisionEpochThatIsKilledOrUnboundDeniesTheIntentWithoutACommand`,
  `TestHTTPResolutionOfAKilledDecisionEpochDeniesTheIntent`,
  `TestHTTPResolutionAfterTheRuntimeOwnerLeaseLapsedIsRefusedAndConsumesNothing`.
  Layers below: domain `TestFreshnessFailureChecksLifecycleThenVersionThenHealthThenExpiry`,
  `TestApprovalDispositionOrdersResolvedThenStaleThenExpired`; store
  `TestTheHourlyDispatchBudgetIsPerTenantTypeAndHour`.
- DUP-001 (the risk class and approval requirement rule): the rule lives in
  `contractsv1.RouteFor` (its own table in `internal/contractsv1`). Policy has
  exactly one table of expectations over it,
  `TestRiskRouteForEveryRiskClassAndCatalogApproval` (every risk class, with and
  without the catalog flag, plus unknown and malformed classes, asserting route
  and reason), and one parity test that the digested policy document and
  `SourceHealthIncomplete` derive from the same table,
  `TestThePolicyDocumentIsDerivedFromTheSameRiskTable`. App tests only prove
  ordering and effects per route, not the table.
- 1 (events and stored documents are evidence): `TestAnIntentWhoseStoredRecordsDoNotVerifyIsDeniedBeforeAnyCommand`,
  `TestDecisionValidationPrecedenceAndBinding`, `TestIntentValidationPrecedenceAndBinding`.
- 8 (stable identities, idempotency): `TestAnAutomaticCommandIsPreparedOnceAndReplayedWithoutDuplicates`,
  `TestACommandIsStoredOnceAndAReplayLearnsTheWinner`, `TestACommandIsPublishedToTheOutboxOnce`,
  `TestHTTPApprovalAndDenialCommitOnce`, `TestConcurrentHTTPRepliesCreateOneCommand`,
  `TestHTTPApprovalRollsBackWhenAnyPublicationWriteFails`,
  `TestEvaluationRunsItsFencesOnTheCallersTransactionAndLeavesCommitToIt`,
  `TestACommittedApprovalDispatchesWithoutNewSensorInput`.
- 10 (explainable): `TestAnInterlockDenialKeepsTheStableReasonAndAuditsItsCause`,
  `TestAnApprovalWithUnreadableExpiryIsExpiredWithAnAuditReason`.

## Open items
- The test fixtures in `app/fixture_test.go` and `store/fixture_test.go` share about
  60 lines (document sealing and the row seed). `dupl` at threshold 60 does not
  flag them, but a shared seeding helper would need a new
  test-support package and an architecture-gate edit; not done in this round.
- `scalar[T]` (query one value in a test) is copied into policy app, policy
  store, cognition, episodes (twice) and an `actions` helper. One
  `storagetest.Scalar` would replace them; a repository-wide change for round 17.
- Resolution at exactly the approval deadline is proven in the domain
  (`ApprovalDisposition`) and, for the evaluate path, in app
  (`TestAnApprovalExpiresExactlyAtItsDeadline`), not through `ResolveApproval`
  at the app layer: mutating the comparison in `ApprovalDisposition` to be
  exclusive is caught by the domain test only.
- A stale approval ends with status `denied` and `withdrawn_at` set (the approval
  ledger has no `withdrawn` status). Pinned by
  `TestResolvingAnApprovalRevalidatesTheIntentAgainstCurrentState`; whether it
  should read `withdrawn` is a design question, not a test one.
