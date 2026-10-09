# testsupport

Status: done
Round: 10

Covers `internal/testsupport/executorconformance` and `internal/testsupport/workerfake`.

- `executorconformance` is the contract of the `episodes.Executor` port. Each concrete
  executor runs it from its own package; the suite itself no longer builds, starts or
  connects any executor.
- `workerfake` is a validating `EpisodeWorker` that speaks the worker protocol like a
  real worker. It is the stand-in used by the remote executor, the experiment tests
  in `cmd/agentic-stream`, and the evidence integration test.

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top / sub) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `testsupport/executorconformance` | **66.7%** | 94.9% | 1.6 s | 1.5 s | 3 / 0 | 6 / 24 |
| `testsupport/workerfake` | 85.6% | 95.6% | 1.4 s | 1.3 s | 13 / 6 | 20 / 68 |

No test takes more than 0.01 s (race start-up is the rest). Before, `TestSeparateProcessWorkerConforms` re-executed the test binary and polled for its socket with `time.Sleep(10ms)` for up to 30 s.
Test-hygiene findings: `executorconformance` 4, `workerfake` 6 before; 0 after.

## Findings and changes

### Removed
- `TestFakeExecutorConforms`: the fixture executor already runs the suite in `internal/executor/fixture` (T2). The test existed here only because the suite had to be run somewhere.
- `TestStreamedWorkerConforms`, `TestSeparateProcessWorkerConforms`: they tested the remote executor, not the suite. Moved to `internal/executor/remote` (`TestRemoteExecutorConformsToTheExecutorPort`, `TestWorkerProcessOnAUnixSocketConformsToTheExecutorPort`). The package now has no `TestMain`, no `os/exec`, no sockets and no sleep.
- `workerfake`: `TestWorkerExecutionContextPreservesDeadlineAndCancellation` and `TestExecutionBoundsKeepEarlierDeadlineAndCancelBothTimers` tested the same function (`boundedExecutionContext`); one table now. `TestServerRejectsUnboundedWorkerBudgetBeforeStarted` repeated the budget case of `ValidateRequest` at the wire; kept as one row of `TestServerRejectsAnInvalidRequestBeforeStarting`. `TestWorkerHandlerPreservesWireStatuses` (internal call of `executeHandler`) is now rows of the wire-level `TestServerEnforcesTheStreamProtocolOnTheHandler`.
- `TestEvidenceSocketDialsOnlyUnix`: asserted `conn.Target() != ""`, which `grpc.NewClient` always satisfies (T4).

### Renamed or moved
- `TestRequestValidationCoversSizeVersionShapeAndDeadline` → `TestValidateRequestCodesEveryDefect`; `TestHandshakeChecksVersionsIdentityAndFeatures` → three tests in `handshake_test.go`; `TestStreamValidatorEnforcesSequenceIdentityAndSingleTerminal` → `stream_validation_test.go`; `server_test.go` (270 lines mixing wire and unit tests) → `server_test.go` (wire only), `execution_bounds_test.go`, `request_validation_test.go`, `stream_validation_test.go`, `handshake_test.go`, `limits_test.go`, `dial_test.go`, `fixtures_test.go`.

### Improved
- T4: the validation tests asserted `== nil` only (`ValidateHandshake(bad) == nil`, `ValidateRequest(req) == nil`) and so passed for any error. Every case now names the gRPC code the protocol assigns (`InvalidArgument`, `FailedPrecondition`, `PermissionDenied`, `ResourceExhausted`, `DeadlineExceeded`), and `TestValidateRequestChecksProtocolBeforeIdentityBeforeBudget` pins the order the production comment calls "part of the contract".
- T6: every test and subtest parallel; `t.Context()` in place of `context.Background()`; no inline `os.MkdirTemp` (see Added).
- T9: the three copies of "serve a worker over bufconn and dial it" (the executorconformance test, `remote/app` test, this package's `newBufConn`) are one exported helper.
- `executorconformance.Run` was weak: it checked the status, identity and a digest and nothing else. It now also requires the Decision to be bound to the request's episode, attempt, fence, situation, situation version and snapshot digest (`checkDecisionBinding`). Every existing executor passed; a fixture executor that dropped a binding would have passed before.
- `executorconformance.CheckContextEnding` is unchanged and still used by the native tests.

### Added
- `executorconformance`: `RunCanceled(ctx, executor)`: an executor given a canceled context returns an error matching `context.Canceled` or the canceled Outcome of the request's attempt, and never a Decision. Native and remote run it; the fixture executor ignores its context and cannot (see Open items). `SetBudget` and `EditPayload` replace three private copies of "rewrite the request's budget/payload" (native `replaceBudget`, remote `requestWithBudget`, remote domain payload edits).
- `executorconformance` self-tests (the suite is a test oracle, so it is tested for false passes): `TestRunRejectsEveryBreachOfTheOutcomeContract` (11 broken executors: no executor, error, no outcome, failed outcome, foreign attempt, stale fence, decision not JSON, forged digest, decision of another attempt, another snapshot, no fence), `TestRunCanceledAcceptsBothWaysToReportACancellation`, `TestRunCanceledRejectsAnExecutorThatIgnoresCancellation`, `TestCheckContextEndingMatchesWhatTheRunnerRecords`, `TestEditPayloadAndSetBudgetRewriteOnlyTheDurablePayload`.
- `workerfake`: `Connect`/`ConnectClient(tb, server)` (in-memory worker connection) and `SocketDir(tb)` (a private short directory for a Unix socket, with the single `//nolint:usetesting` reason for macOS socket path length), both shared by `remote`, `worker` and this package's tests.
- `workerfake` tests: `TestServerEnforcesTheStreamProtocolOnTheHandler` (sequence gap, no terminal, event after terminal, decision of another attempt, oversize event, event count, handler panic, handler error without a status, status kept, cancellation kept), `TestServerWithoutAHandlerRefusesBeforeStarting`, `TestServerStopsAHandlerThatOutlivesTheWallTime`, `TestDialEvidenceSocketReachesAUnixListener` (waits for the connection to reach `Ready`, not just `Target() != ""`), `TestDialEvidenceSocketRefusesAnythingButACleanAbsolutePath`, `TestStreamValidatorCodesEveryDefectOfAnEvent` (14 cases) and `TestStreamValidatorStopsAtTheEventLimit`, `TestValidateRequestCodesEveryDefect` (26 cases incl. each evidence-endpoint rule), `TestStartedEventOpensTheStreamAsTheWorker`.

### Speed
- Nothing slow remains. The only sleep (the 10 ms poll for the child's socket) is gone with the process test, now `listening` on the child's stdout.

## Production code touched
- none (both packages are test support; `executorconformance/conformance.go` and the new `workerfake/bufconn.go` and `workerfake/socketdir.go` are the only non-`_test.go` changes, see above)

## Invariants proven here
- 6 (worker boundary: a worker cannot reach effects or credentials): `workerfake.Server` is the model of a worker and carries none: `ValidateRequest` refuses an evidence endpoint without a capability token and a capability token without an endpoint, and an endpoint that is not a clean absolute Unix path (`TestValidateRequestCodesEveryDefect/evidence_endpoint_*`, `capability_without_an_evidence_endpoint`), so a worker is only ever told one private socket and the token for it. `TestStreamValidatorCodesEveryDefectOfAnEvent` and `TestServerEnforcesTheStreamProtocolOnTheHandler` show that all a worker can emit is a bounded, sequenced, identity-bound stream with one Decision proposal and one terminal. The suite `executorconformance` is where the port is held to that: `TestRunRejectsEveryBreachOfTheOutcomeContract`.
- 5: `TestValidateRequestCodesEveryDefect/missing_budget` and the stream limits.

## Open items
- The Executor port does not say what a nil request or a context ignored mid-run must do (`fixture` panics on nil and ignores `ctx`; `native` and `remote` return errors). If it should, add `RunNilRequest` and fix the fixture; both are production changes.
- `workerfake` exports `Connect`, `ConnectClient`, `SocketDir`, which import `testing`. `internal/evidence/uds_integration_test.go` (round 9) still has its own socket directory helper and could use `SocketDir`.
- `cmd/agentic-stream/experiment_standins_test.go` still builds its worker server by hand (round 16).
