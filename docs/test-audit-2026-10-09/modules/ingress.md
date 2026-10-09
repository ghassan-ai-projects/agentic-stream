# ingress

Status: done
Round: 4

## Metrics

Measured with `go test -short -race -count=1 -cover -json`, other workers running. Package time is dominated by building and linking the test binary under `-race`; the work in this module is correctness and determinism, not speed.

| Package | Coverage before | after | Time before | after | Top-level tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/ingress` | 81.8% | 100% | 1.42 s | 1.78 s | 2 | 4 |
| `internal/ingress/internal/app` | 82.8% | 90.3% | 1.95 s | 2.25 s | 14 | 21 |
| `internal/ingress/internal/domain` | 84.5% | 97.4% | 1.13 s | 1.15 s | 9 | 17 |
| `internal/ingress/internal/store` | 85.7% | 92.9% | 1.34 s | 1.55 s | 3 | 5 |
| `internal/ingress/internal/transport` | 81.9% | 88.6% | 1.20 s | 1.18 s | 10 | 18 |

Passing tests and subtests: 38 top-level + 16 subtests → 65 top-level + 74 subtests. Test files: 11 → 17. Hygiene lint: 53 findings across spec and ingress before the round (51 `paralleltest`, 2 `thelper` in `internal/app/helpers_test.go`; the split by module was not recorded) → 0 in both. `time.Sleep` uses: 4 → 0. Stress: `go test -race -count=30` on the app and transport packages passes; `-shuffle=on -count=3` passes.

## Findings and changes

### Removed
- No test was deleted outright; `rules_test.go` was split (below). The malformed line of the reconnect test now travels in the same connection as the first event so its quarantine is ordered before the delivery the test waits on.
- The `t.Skipf` on `EPERM` in the live socket tests (T10): a sandbox that forbids Unix sockets should fail the test loudly, not skip the only live-socket coverage.
- Comments inside tests (T11), including the finding references (`A-049 F1`, `F4`) in names and comments: the names now state the behavior.

### Renamed or moved
- `internal/domain/rules_test.go` → `admission_test.go`, `identity_test.go`, `checkpoint_test.go`, `live_test.go` (T3: file names name the subject).
- `internal/domain/simulator_convert_test.go` → `simulator_event_test.go`.
- `internal/transport/socket_test.go` → `server_test.go` (+ new `listener_test.go`); `internal/transport/live_test.go` merged into `server_test.go` (it tested `server.go` framing).
- `TestLiveUDSSourceQuarantinesMalformedLinesAndCountsValidLines` → `TestProcessLineQuarantinesMalformedLinesAndPassesValidOnesToTheSink`; `TestLiveUDSSourceAcceptsReconnects` → `TestServeLiveAcceptsReconnectingClientsAndQuarantinesMalformedInput`; `TestLiveUDSSourcePropagatesSinkDeadlineWithActiveParent` → `TestServeLiveReportsASinkDeadlineWhileTheContextIsActive`; `TestOversizedLiveLineIsQuarantinedAsItsBoundedPrefix` → `TestProcessLineQuarantinesAnOversizedLineAsItsBoundedPrefix`; `TestJSONLReplayAppendsEvents` → `TestJSONLReplayAppendsEventsAndResumesFromItsCheckpoint`; `TestJSONLReplayQuarantinesOversizedLine` → `TestJSONLReplayQuarantinesAnOversizedLineAndContinues`; `TestSimulatorJSONLReplay…` → `TestSimulatorReplay…`; `TestSimulatorChannelFieldMappingEdges` → `TestConvertEventMapsTheChannelValueToItsConfiguredField` (table); `TestBoundedTraceLinesPreserveBoundaries` → `TestBoundedReaderPreservesLineBoundaries`; `TestServiceReplaysAndResumesThroughTheFacade` → `TestServiceReplaysJSONLAndResumesThroughTheFacade`.
- Shared fixtures (`vibrationLine`, `writeTrace`, `quarantine`, `countRows`, `registerBuiltinSchema`) in `helpers_test.go` replace five copies of the same 300-character envelope literal and the repeated schema-registration block (T9).

### Improved
- T5: all four `time.Sleep` calls are gone. The transport tests bind the listener with `listenSocket` and run `server.serve` on it, so a client can dial immediately and no readiness wait exists; the client-limit test drops the 50 ms sleep because accept order is connection order and the client count is incremented before the next accept. The app tests that need the real `ServeLive` wait with a ticker-driven dial loop bound to `t.Context()` with a 5 s cap (no sleep).
- T6: socket paths are short temp directories (`os.MkdirTemp` plus cleanup, the repo convention for Unix sockets) instead of fixed `/tmp/...-<UnixNano>.sock` names; every test and subtest is parallel; `t.Context()` replaces `context.Background()`.
- T4: error tests assert which failure (`load checkpoint`, `save checkpoint`, `open trace file` with `fs.ErrNotExist`, `unknown event field`, `runtime_config must be first`, the quarantine reason map by event ID, `sink is required`, `clean absolute Unix path`, `unmarshal checkpoint`, `occupied or stale`, `already active`). The quarantine tests assert the exact `event_id`→`reason_code` map instead of a row count.
- `TestCheckpointWriteFailureLeavesTheReplayResumable` also asserts the event log holds the first run's event once.

### Added
- Domain: `TestSimulatorTraceRefusesInvalidControlRecords` (12 rules of `runtime_config`, `trace_end` and `model_activation`), `TestSimulatorTraceAcceptsAModelActivationBetweenEvents`, `TestConvertEventRefusesAnEventThatBreaksTheRecordGrammar` (12 rules), `TestConvertEventNamesTheEventTypeFromThePrefixAndChannel`, `TestAdmitEnvelopeRejectsWithTheReasonAndWhetherTheLineDecoded`.
- App: `TestJSONLReplayAppendsALargeTraceAcrossBatches` (crosses the 100-event batch twice), `TestJSONLReplayQuarantinesAnEnvelopeOfAnotherTenant`, `TestJSONLReplayCountsBlankLinesAndStripsLineTerminators`, `TestJSONLReplayNamesAMissingTrace`, `TestProcessLineQuarantinesAnEnvelopeThatBreaksTheContract` (was 0% `rejectEnvelope`), `TestProcessLineReportsAFailedQuarantineWrite`, `TestServeLiveRefusesWithoutASinkOrWithAnUnsafePath`, `TestSimulatorReplayRefusesATraceThatBreaksTheGrammar` (nothing appended or checkpointed when a trace lacks `runtime_config` or `trace_end`), `TestSimulatorReplayNamesAMissingTrace`.
- Store: `TestStorageFailureIsNeverAnEmptyCheckpoint` (load and save), `TestSaveLineUpdatesOneRowPerConnectorAndKeepsItsKind`.
- Transport: `TestListenerIsOwnerOnlyAndRemovesItsSocketOnClose`, `TestListenerRefusesAStaleSocketFile`, `TestServeNumbersEachConnectionSeparately`, `TestServeRefusesAnUnsafeSocketPath`, `TestReadLiveLineAcceptsAFinalLineWithoutANewline`, `TestShutdownIsNormalOnlyWhileTheContextIsDone`, `TestServeRetriesATimedOutAcceptAndStopsOnAnyOtherAcceptFailure` (accept retry on timeout; was 20% `retryAccept`).
- Facade: `TestServiceReplaysASimulatorTraceThroughTheFacade` (`ReplaySimulator` was 0%).

### Speed
- Nothing was slow (all packages 1–2 s, dominated by `-race` build). The deterministic waits remove the only wall-clock dependency; the module's tests no longer sleep.

## Production code touched
- none

## Invariants proven here
- 1 (raw events are evidence, never instructions): `TestJSONLReplayQuarantinesMalformedAndSchemaInvalidLines`, `TestJSONLReplayQuarantinesAnEnvelopeOfAnotherTenant`, `TestProcessLineQuarantinesAnEnvelopeThatBreaksTheContract`, `TestSimulatorTraceRefusesGrammarViolations`.
- 2/8 (duplicate and replayed input does not double-append; stable identities): `TestJSONLReplayAppendsEventsAndResumesFromItsCheckpoint`, `TestCheckpointWriteFailureLeavesTheReplayResumable`, `TestJSONLReplayQuarantineIDsAreConnectorScoped`.
- Bounded input: `TestJSONLReplayQuarantinesAnOversizedLineAndContinues`, `TestOversizedClientFrameKeepsOnlyTheBoundedPrefix`, `TestClientLimitClosesTheSeventeenthConnection`, `TestListenerRefusesUnsafeExistingPaths`.

## Open items
- Finding, not fixed (behavior-preserving round): `cleanLiveListener.closeAndRemove` (`internal/ingress/internal/transport/listener.go`) claims to remove the socket file "only if it is still the one this listener created", but `net.UnixListener.Close` unlinks the path first, regardless of identity, so a file that replaced the socket is deleted and `removeSocketFile` is unreachable (0% covered). Verified with a scratch test (replace the socket with a regular file, close the listener, the file is gone). Fix would be `SetUnlinkOnClose(false)` in `listenSocket`; it changes behavior, so it needs its own change with a test.
- The two socket helpers (`os.MkdirTemp` with a `//nolint:usetesting` reason) cannot use `t.TempDir()` because a Unix socket path is limited to about 100 bytes and `t.TempDir()` paths with these test names are longer.
