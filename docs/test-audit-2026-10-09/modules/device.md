# device

Status: done
Round: 13

`internal/device` is the effect boundary and the reference adapter module. Its
tests now prove, per layer: record framing (`wire`, `transport`), catalog and
materialization rules (`domain`), the session lifecycle and effector contracts
(`app`), and the wiring over a real Unix socket (facade). Largest test file went
from 493 lines to 203 (`fixtures_test.go`); no test file above 200 lines except
that one, and none of the phase-numbered names is left.

## Metrics

Time is `go test -short -race -count=1` wall time per package while other
workers ran. Tests are passing tests plus subtests (top-level in brackets).

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `device` (facade) | 93.3% | 100.0% | 1.9 s | 1.6 s | 18 (3) | 15 (4) |
| `device/internal/app` | 82.8% | 89.7% | 3.7 s | 2.5 s | 51 (35) | 79 (48) |
| `device/internal/domain` | 83.8% | 92.3% | 1.3 s | 1.1 s | 42 (14) | 67 (16) |
| `device/internal/transport` | 87.7% | 92.3% | 1.4 s | 1.3 s | 16 (10) | 19 (11) |
| `device/internal/wire` | 89.9% | 98.6% | 1.6 s | 1.1 s | 21 (7) | 31 (7) |

Test-hygiene lint: 61 findings in the module (40 in `app`) to 0.
`golangci-lint` (repository config) 0 issues; `dupl` at threshold 60: 0.
`go test -race -shuffle=on -count=3 ./internal/device/...` passes. Slowest test:
0.4 s (a database-backed session test). `time.Sleep`: none.

## Findings and changes

### Removed
- `TestGatewayEffectorDispatchesThermalRoute` (`app/effect_routing_test.go`):
  it dispatched `set_indicator`, the same route and assertions as the dispatch
  test; the second route (`select_thermal_mode`) is now a row of
  `TestGatewayEffectorSendsOnlyAfterAuthorizationAndLeavesVerificationPending`.
- `TestOpenDeviceSessionRequiresFirmwareAllowList`: now one row of
  `TestSessionRefusesAnIncompleteConfiguration`.
- `TestDeviceDecodeRejectsTrailingDataBeforeMessageSchema` (`wire`): now the
  "trailing record before an unsupported type" row of `TestDecodeFailsClosed`.
- Digest, LED and fan assertions of `TestPhysicalArduinoCatalogMaterializesAndValidatesLEDAndFanCommands`
  (`domain`): duplicated `TestThermalCapabilityCatalogDigest` and
  `TestMaterializeProducesBoundedDeviceCommands`; what was unique (safe-stop
  materialization) is `TestMaterializeSafeStopIsFixedByTheCatalogAndBoundToItsDigest`.
- `TestUDSTransportClosesAfterOversizedFrame`: merged with
  `...RejectsOversizedFrameBeforeDecoding` into
  `TestUDSTransportRejectsAnOversizedFrameAndClosesTheLink` (same setup, one
  sequence of assertions).
- Facade `TestEffectProfilesFenceReplayAndLiveLinks` re-tested every domain
  profile rule (T2); it is now `TestEffectProfileValidationReadsTheConfigurationItIsGiven`,
  which proves only the wiring (link or options count as a link, replay, shadow,
  live-actuation and owner flags are passed through) with the domain message
  asserted.

### Renamed or moved
- `app/phase04_test.go` → `session_reconciliation_test.go` (T3); its five
  `TestSerialSession*` tests are renamed by behavior.
- `app/gateway_effector_test.go` (493 lines) → `gateway_effector_dispatch_test.go`,
  `gateway_effector_verification_test.go`, `gateway_effector_safe_stop_test.go`,
  `gateway_effector_unknown_outcome_test.go`, `gateway_effector_authorization_test.go`
  (the retired word "serial" is gone from test names).
- `app/session_test.go` → `session_exchange_test.go`, `session_handshake_test.go`;
  its fake transport and builders → `fake_transport_test.go`, `fixtures_test.go`,
  `device_control_test.go`.
- `app/state_admission_test.go` → `session_epoch_fence_test.go`;
  `exchange_test.go` → `session_receipt_cache_test.go`;
  `session_admission_test.go` → `session_admission_internal_test.go`.
- `transport/uds_test.go` → `uds_contract_test.go`, `uds_frames_test.go`.
- `domain/materialize_test.go` loses the invalid-catalog table to `catalog_test.go`;
  shared builders → `fixtures_test.go`. Facade and `wire` builders → `fixtures_test.go`,
  `device_peer_test.go`.

### Improved
- T4: error tests assert which refusal (domain message) instead of `err != nil`:
  materialize (12 fail-closed rows), catalog loading (21 rows), codec decode (11)
  and encode (5), UDS send validation (4), handshake (4), session configuration (8).
  `TestPhysicalArduino...` pinned a digest with a cross-repo copy; the copy check
  is now a `t.Skip` ("REAL_WORLD_SENSOR_ROOT is not set") instead of a passing
  test that only logged.
- T6: every test and subtest is parallel; `os.MkdirTemp("/tmp", ...)` in the
  facade and UDS tests replaced by `workerfake.SocketDir` (import is allowed: the
  architecture import rules do not cover test files).
- T5: the gateway facade test's emulated device now tracks its output, so the
  end-to-end verification asserts `succeeded` (it accepted `succeeded` or
  `failed`). Lease loss uses a virtual clock. Wait bounds are 5 s `select`s, no sleeps.
- T9/T11: one `openSessionOn`, `stateFor`, `queueState`, `indicatorCommand`,
  `assertRestartedSessionRefusesOrdinaryCommands` instead of eight copies of the
  session-config literal and three copies of the restart sequence.

### Added
- `TestSessionRefusesAnIncompleteConfiguration`: every `OpenSession` precondition
  (transport, catalog, epoch, authority, both allow-lists, owner instance).
- `TestSessionDefaultsTheOwnerInstanceToTheAuthoritys`, `TestSessionExposesTheHandshakeBoundIdentity`,
  `TestANilOrClosedSessionIsNotOpen`.
- `TestSessionRefusesAnIdempotencyKeyReusedForADifferentCommand`,
  `TestSessionRefusesAMaterializedCommandForAnotherBootBeforeSending`.
- `TestOpeningAGatewayEffectorRequiresALinkAndACatalog`,
  `TestOpeningAGatewayEffectorReportsAHandshakeFailureAndClosingItClosesTheLink`,
  `TestNewGatewayEffectorRefusesAnIncompleteConfigurationWithoutOwningTheLink`.
- `TestAnUnconfiguredGatewayEffectorRefusesEveryOperation`,
  `TestGatewayEffectorRefusesACommandOutsideTheCatalogBeforeSending`,
  `TestFailClosedEffectorRefusesEveryRouteItDoesNotOwn`.
- Verification: matching output yields `succeeded` (new rows), and
  `TestVerificationRefusesACommandOutsideTheCatalogBeforeQueryingTheDevice`.
- Domain: nil catalog, unsupported protocol, missing command identity, non-numeric
  bounded parameter, hard minimum, safe-stop target/boot refusals, safe-stop
  identity per boot, invalid safe-stop catalog entries, `Digest` of a missing
  catalog (`materialize_test.go`, `catalog_test.go`).
- `wire`: encode refusals (`TestEncodeFailsClosed`), non-object and missing-type frames.
- `transport`: `Dial` to an absent socket, closed and nil transports refuse every
  operation, validation refusals are not "possibly sent".

### Speed
Nothing slow to start with (the package times are race start-up plus one
migrated database per session test). `app` went 3.7 s → 2.5 s because tests now
run in parallel; the socket tests open no extra processes.

## Production code touched
- none.

## Invariants proven here
- 6 (a model cannot execute effects): `TestEveryEffectorChecksAuthorizationBeforeDispatch`,
  `TestGatewayEffectorRefusesAnUnauthorizedCommandBeforeMaterializingOrSending`,
  `TestMaterializeIgnoresModelSuppliedParameters`, `TestMaterializeFailsClosed`,
  `TestMaterializeReEnforcesHardBoundsOnPresetsMisauthoredAboveThem`.
- 7 (revalidation immediately before dispatch): the final authorization check
  precedes materialization and sending (same tests).
- 8 (stable identities, idempotency, unknown outcomes):
  `TestSessionAnswersADuplicateIdempotencyKeyFromItsReceiptCache`,
  `TestSessionRefusesAnIdempotencyKeyReusedForADifferentCommand`,
  `TestAMalformedReceiptAfterSendingIsAnUnknownOutcomeThatSurvivesRestart`,
  `TestCancellationAfterSendingStillPersistsTheReconciliationBarrier`,
  `TestLosingTheOwnerLeaseAfterSendingMakesTheOutcomeUnknownAndPersistsTheBarrier`,
  `TestSafeStopRejectionThatCannotBeRecordedIsUnknownAndSurvivesRestart`.
- 9 (replay never performs effects): `TestEffectProfileValidationReadsTheConfigurationItIsGiven`
  (replay and shadow refuse an emulator or physical profile) and the domain
  `TestCheckEffectProfile`.
- Device-specific safety: `TestSafeStopOutranksTheBarrierAndRefusesLaterOrdinaryWorkEvenAfterRestart`,
  `TestKilledEpochRefusesTheDeviceHandshakeAndClosesTheLink`.

## Open items
- Production error text still says "serial" (`gateway_effector.go`, `session*.go`:
  "serial effector session and catalog are required", "device rejected serial
  command"), a word the module language retires. Tests assert the current text.
  Renaming is a production change for the owner.
- `Session.Exchange` returns a `deviceExchangeError`, not an
  `actionport.UnknownOutcomeError`; the effector converts it. The lease-loss
  test therefore asserts the cause (`ErrRuntimeOwnerBusy`) at the session layer.
- Remaining uncovered lines in `app` (89.7%) are wrap-only database error
  returns of the authority; not covered with assertion-free tests.
