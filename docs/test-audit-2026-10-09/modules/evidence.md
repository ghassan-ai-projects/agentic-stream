# evidence

Status: done
Round: 9

## Metrics

Measured with `go test -short -race -count=1` while other workers ran (compare back to back, not absolute).

| Package | Coverage before | after | Time before | after | Tests before (top-level / passing incl. subtests) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/evidence` (facade) | 100.0% | 100.0% | 1.63 s | 1.49 s | 4 / 9 | 8 / 13 |
| `internal/evidence/internal/app` | 81.5% | 98.1% | 2.59 s | 3.06 s | 17 / 43 | 43 / 150 |
| `internal/evidence/internal/domain` | 95.5% | 100.0% | 1.27 s | 1.14 s | 6 / 21 | 22 / 154 |
| `internal/evidence/internal/store` | 86.4% | 87.9% | 1.85 s | 2.18 s | 7 / 11 | 19 / 64 |
| `internal/evidence/internal/transport` | 94.3% | 94.3% | 1.56 s | 1.68 s | 3 / 3 | 10 / 34 |
| `internal/evidence/internal/wire` | 82.9% | 95.1% | 1.33 s | 1.22 s | 5 / 13 | 18 / 74 |

Test-hygiene findings: 50 → 0 (`paralleltest`, `tparallel`, `usetesting`, `thelper`). Repository lint: 0 issues; `dupl` at threshold 60: 0 issues. No `time.Sleep`. Slowest test 0.32 s. `-race -shuffle=on -count=3` passes. `app` and `store` run slightly longer in absolute terms because they hold about three times as many tests, each with its own migrated database; no test is above 0.35 s.

## Findings and changes

### Removed
- `app_test` (external package) and its facade-based fixtures: `server_fixture_test.go`, `server_test.go`, `server_bounds_test.go`. They reached the application only through the `evidence` facade and protobuf messages, duplicating `openLedgerDB`, type aliases and a protobuf call builder. The same behavior is now proven in package `app` against `app.New` and `domain.Envelope`; the protobuf and status mapping is proven in `transport`.
- `TestCapabilityAndEvidenceScope`: three scope refusals (tenant, token, entity) moved into the call-scope table; its success path is `TestCallReturnsTheBoundedResultAndReplaysItWithoutQuerying`.
- `TestDurableQueryFailureNeverReusesCallIdentity`: folded into `TestCallEnforcesResultBoundsAndRecordsStableFailureCodes` (retrying every failed call is asserted for all three failure kinds).
- `TestExpiredCapabilityFailsClosed`: now the `expired capability` rows of the call-scope and verify tables.
- `TestAdmissionRulesPreserveOrderAndBounds`, `TestAttemptAndReservationRules`, `TestQueryFailureCategories`, `TestScopeDefaultsAndValidity`, `TestCapabilityTimeChecks`, `TestConfiguredAuthorityAndLeaseDefaults` (domain): each was one test with 10 to 25 `t.Fatal` branches and unnamed cases; replaced by named tables (see below).
- Per-package copies of `ledgerTestCall`/`traceForLedger` in `wire`, `store`, `app`, `transport` collapsed to one builder per package; `PRAGMA foreign_keys = OFF` + `SetMaxOpenConns(1)` replaced by `storagetest.OpenTempWithoutForeignKeys` everywhere.
- Ticket-style naming (`TestLedgerOwnerLossAndSupersessionRefuseCompletion` mixed three behaviors) split into `TestLedgerOwnerLossRefusesReservationWithoutLeavingARow`, `TestLedgerOwnerLossRefusesCompletionAndFailure`, `TestLedgerRefusesToCompleteForAnAttemptThatIsNoLongerCurrentAndRunning`.

### Renamed or moved
- `uds_integration_test.go` → `private_socket_test.go` (`TestEvidenceToolsOverPrivateUDS` → `TestEvidenceToolsServeWorkersOverThePrivateSocket`). It also proves an out-of-scope call is `PermissionDenied` over the socket. The socket directory stays `os.MkdirTemp` (short path, unix socket limit is 104 bytes; `t.TempDir` is 95 here and longer in other sandboxes), with a true `//nolint:usetesting` reason.
- `app`: `ledger_test.go`/`ownership_test.go` → `ledger_reservation_test.go`, `ledger_completion_test.go`, `ledger_recovery_test.go`; `ledger_integrity_test.go` kept; new `fixtures_test.go`, `call_scope_test.go`, `call_results_test.go`, `capability_test.go`, `service_config_test.go`.
- `domain`: `authorization_test.go` (admission + attempt + reservation + failure categories) and `capability_test.go` → `fixtures_test.go`, `scope_test.go`, `authorization_test.go`, `episode_binding_test.go`, `reservation_test.go`, `refusal_test.go`.
- `store`: `store_test.go` (301 lines) → `fixtures_test.go`, `reservation_test.go`, `episode_state_test.go`, `transaction_test.go`.
- `wire`: `records_test.go` + `token_test.go` → `fixtures_test.go`, `token_test.go`, `arguments_test.go`, `envelope_test.go`, `fingerprint_test.go`, `events_test.go`, `identity_test.go`.
- `transport`: `server_test.go` → `fixtures_test.go`, `server_test.go`, `event_log_query_test.go`.

### Improved
- T6: every test and subtest is parallel; `t.Context()`, `t.Cleanup`, no package-level mutable state.
- T4: refusals assert the kind (`domain.ErrorKind`) or the message the domain defines; gRPC tests assert the code and the fixed message.
- T11: table-driven where three or more cases share a shape. The old out-of-scope table of 19 rows is 27 rows and now also proves that no provider query and no ledger row exists after a refusal.
- `TestSignedPayloadRefusesMalformedAndTamperedTokens`: tamper by rewriting a middle or signature character, not by flipping the last base64 bit (see Open items).

### Added
Capability scoping (a call can read only what its capability grants):
- `TestAuthorizeScopeRefusesEveryDimensionOutsideTheCapability` (domain): episode, attempt, fence, tenant, situation, version, entity, tool, trace, trace state, runtime epoch, range before/after/inverted/missing/unreadable, signed-range overflow, and that a scope mismatch is reported before the range.
- `TestBindArgumentsRequiresTheEntityAgreedByScopeAndRequest`, `TestBindArgumentsNeverExpandsTheGrantedBudgets`, `TestBindArgumentsCarriesTheRequestedTimeRange`, `TestCallDeadlineIsCappedAtCapabilityExpiry`.
- `TestCallRefusesEveryRequestOutsideTheCapabilityBeforeReserving` (app): 27 refusals, none reaches the provider or writes a ledger row.
- `TestCallAdmitsTheExactGrantAndPassesOnlyAuthenticatedDimensionsToTheProvider`: a request asking 500 rows / 50000 bytes against a 10 / 1024 grant reaches the provider as 10 / 1024.
- `TestEventLogQueryReadsOnlyWhatTheCallScopeAllows` (transport): same entity and window only, another entity, another tenant, row budget, earlier and later windows.
- `TestCallRefusesOutOfScopeRequestsWithoutQueryingTheProvider`, `TestWorkerSeesResultsAndRefusalsOverGRPC` (bufconn; no TCP).

Recovery of an interrupted call:
- `TestLedgerRecoverTxInterruptsPriorEpochCallsOnly`, `TestLedgerRecoveryRollsBackWithTheCallersTransaction`, `TestLedgerRecoveryRefusesWithoutOwnershipOrConfiguration`, `TestLedgerReclaimsExpiredLeasesWithTheConfiguredVirtualClock`, `TestLedgerReclaimInterruptsCallsOfAForeignEpoch`, `TestLedgerReclaimRefusesWithoutOwnershipOrConfiguration` (app).
- `TestRecoverInterruptsOnlyRunningCallsOfAPriorEpoch`, `TestReclaimExpiredInterruptsOnlyExpiredAndForeignEpochRunningCallsOnly`, `TestUnreadableLeaseTextIsNeverOwnedAndIsReclaimed` (store).
- `TestRecoveryJoinsTheCallersTransactionAndKeepsOwnerFailures` (facade).
- `TestCallCannotCommitAResultOnceTheAttemptIsSuperseded`, `TestCallRefusesStaleEpisodesBeforeQuerying`, `TestCallRefusesWhenTheRuntimeOwnerIsLost`, `TestCallRefusesADuplicateWhileTheFirstIsStillRunning` (channels, no sleeps), `TestCallRefusesAReusedCallIdentityWithADifferentRequest`.
- `TestCallDeadlineBoundsTheProviderQuery`, `TestCallDiscardsAResultThatArrivesAfterItsDeadline` (20 ms real deadline on the provider context).

Codecs (round trip and malformed input):
- `TestSignedTokenRoundTripsEveryClaim`, `TestSignedTokenClaimsAreTheV1Contract`, `TestSignedPayloadRefusesMalformedAndTamperedTokens` (13 cases), `TestSignedPayloadRefusesCorrectlySignedClaimsThatAreNotBase64`, `TestDecodeScopeRefusesUnreadableClaims`.
- `TestDecodeEvidenceGetArgumentsAcceptsOnlyTheClosedV1Schema` (11 inputs), `TestDecodeEnvelopeCopiesEveryRequestFieldWithoutGrantingAuthority`, `TestDecodeEnvelopeKeepsTimestampPresenceAndValiditySeparate`, `TestResultMessageEchoesTheRequestIdentityAndHashesTheExactBytes`.
- `TestCallFingerprintEncodingIsTheStoredV1Contract` (kept) and `TestCallFingerprintChangesWithEveryRequestDimension` (17 dimensions).
- `TestEncodeEventsKeepsLexicalFieldOrderAndExactBytes`, `TestEncodeEventsOfNoRowsIsAnEmptyArray`, `TestEncodeEventsRefusesPayloadsJSONCannotCarry`, `TestDecodedEventRoundTripsThroughTheResultEncoding`, `TestDecodeEventRefusesPayloadsThatAreNotJSONObjects` (`EncodeEvents` and `DecodeEvent` were 0%).

Ledger and configuration:
- `TestLedgerRefusesToReuseACallIdentityForAnotherRequestOrToken`, `TestLedgerRefusesToReserveWithoutCompleteConfiguration`, `TestLedgerReservesOnlyLiveAttemptsOfTheCurrentEpisode`, `TestLedgerTerminalCallsAcceptNoSecondOutcome`, `TestLedgerRefusesOutcomesWithoutConfiguration`.
- `TestNewRefusesConfigurationThatLacksASafetyDependency` (19 rows), `TestNewCopiesTheKeyRingSoLaterEditsCannotChangeAuthority`, `TestUnconfiguredOperationsRefuseInsteadOfSkippingASafetyCheck`, `TestServiceReportsItsRuntimeBindingAndClock`.
- `TestIssuedCapabilityVerifiesToTheGrantedScope`, `TestIssueDefaultsTheKeyLifetimeAndMintsADistinctTokenIdentity`, `TestIssueRefusesScopesItCannotBindToAWorker`, `TestVerifyRefusesTokensOutsideTheConfiguredAuthorityAndWindow`, `TestVerifyHonorsTheConfiguredClockSkew`, `TestVerifyRefusesAnIncompleteSignedScope`.
- Domain: `TestValidateScopeRequiresACompleteUnambiguousScope` (22 fields), `TestPrepareScopeDefaultsTheValidityWindowAndRequiresATrace`, `TestCheckIssuableBoundsTheLifetimeAndNamesTheAuthority`, `TestCheckValidityEnforcesTheWindowWithBoundedSkew` (exact skew and expiry boundaries), `TestValidateEnvelopeRefusesIncompleteOrOversizedRequests`, `TestExistingReservationRefusesReuseAndCorruption`, `TestResultViolationNamesTheExceededBudget` (exact-budget boundaries).
- Store: `TestStoreResultCompletesOnlyUnderTheReservingLease` (owner, token, epoch, request, expired lease), `TestFailRecordsTheStableCodeOnlyForTheReservingLease`, `TestReservationReadIsScopedToTheFullCallIdentity`, `TestInsertReservationRefusesARepeatedCallIdentity`, `TestStoredResultReadsBackWithItsExactBytesDigestAndCounts`, `TestTerminalCallsAcceptNoFurtherResultOrFailure`, `TestEpisodeOfAnotherTenantOrWithoutARowIsNeverLoadedAtReservation`, `TestStoreAndJoinedTransactionReportWhetherTheyAreUsable`, `TestOwnerAssertionRunsInTheSameTransactionWithTheStoreEpoch`.

### Speed
Nothing was slow. The existing tests each opened a migrated database from the template (about 0.1 s under `-race`); new tests keep that cost per test but run in parallel. One test waits on a real 20 ms provider deadline (`TestCallDeadlineBoundsTheProviderQuery`, `...DiscardsAResultThatArrivesAfterItsDeadline`) because the deadline is a context timer; the duplicate-call test synchronizes on channels.

## Production code touched
- none.

## Invariants proven here
- 1 (raw events are evidence, never executable instructions): `TestDecodeEvidenceGetArgumentsAcceptsOnlyTheClosedV1Schema` (a worker cannot add query dimensions or fields to the JSON arguments), `TestEventLogQueryReadsOnlyWhatTheCallScopeAllows`, `TestEncodeEventsKeepsLexicalFieldOrderAndExactBytes` and `TestDecodedEventRoundTripsThroughTheResultEncoding` (event payloads travel as data records), `TestCallAdmitsTheExactGrantAndPassesOnlyAuthenticatedDimensionsToTheProvider`.
- 6 (models read evidence and propose; they cannot execute effects): `TestCallRefusesEveryRequestOutsideTheCapabilityBeforeReserving` (ungranted tool `shell.exec`, other tenant/entity/attempt/fence/range), `TestAuthorizeScopeRefusesEveryDimensionOutsideTheCapability`, `TestCallCannotCommitAResultOnceTheAttemptIsSuperseded`, `TestWorkerSeesResultsAndRefusalsOverGRPC`, `TestEvidenceToolsServeWorkersOverThePrivateSocket`, `TestUnclassifiedErrorsNeverLeakDetailsToTheWorker`.
- Durability and fencing (supports invariants 1 and 6, no separate number): `TestLedgerRecoverTxInterruptsPriorEpochCallsOnly`, `TestLedgerRecoveryRollsBackWithTheCallersTransaction`, `TestLedgerConcurrentReservationHasOneWinner`.

## Open items
- Finding (token integrity): `wire.SignedPayload` decodes the signature with the non-strict `base64.RawURLEncoding`, so the last character of the signature has unused low bits and 3 other final characters verify as the same token (checked: 1 original plus 3 alternatives accepted). The MAC is still verified, so authority does not widen, but a capability has several valid string forms. Fix: `base64.RawURLEncoding.Strict()` in `checkSignature` and `SignedPayload`. Not changed (production behavior); tests tamper with a middle character instead.
- Finding (call lifecycle): when the provider returns after the call deadline (or after cancellation without error), `runQuery` returns `DeadlineExceeded` but does not fail the reservation, so the call stays `running` until `ReclaimExpired` and a retry meets `AlreadyExists`. Pinned only as "not completed" in `TestCallDiscardsAResultThatArrivesAfterItsDeadline`; confirm the intended lifecycle.
- The episode/attempt seed SQL (`INSERT INTO episodes ...`) is still repeated once per package (`app`, `store`, `transport`, facade): sharing it needs a test-support package and entries in the `internal/architecture` layer and import tables, which belong to another module. Suggest `internal/evidence/evidencetest` in the gates round.
- `store` stays at 87.9%: the uncovered statements are `RowsAffected` and encoding error branches that SQLite does not produce.
- Production files under `internal/evidence/internal/**` still carry unexplained comments (for example in `app/server.go`); not changed in a test round.
