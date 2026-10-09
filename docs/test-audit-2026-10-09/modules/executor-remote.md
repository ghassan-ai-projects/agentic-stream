# executor-remote

Status: done
Round: 10

`internal/executor/remote` adapts the streamed `EpisodeWorker` protocol to the
episode `Executor` port: `internal/app` (handshake, capability, stream
consumption), `internal/domain` (wire request, stream validator, budget accounting)
and `internal/transport` (the gRPC calls and the context-error mapping).

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top / sub) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `executor/remote` (facade) | 100.0% | 100.0% | 1.5 s | 1.7 s | 1 / 0 | 3 / 2 |
| `executor/remote/internal/app` | 80.5% | 97.6% | 1.5 s | 1.6 s | 11 / 4 | 15 / 30 |
| `executor/remote/internal/domain` | **69.9%** | 97.8% | 1.5 s | 1.5 s | 15 / 25 | 27 / 118 |
| `executor/remote/internal/transport` | 87.5% | 87.5% | 1.3 s | 1.3 s | 3 / 3 | 3 / 3 |

No test takes more than 0.11 s (the facade's separate-process worker test; before it lived in `executorconformance` and polled for the socket). Test-hygiene findings: `app` 14 before (13 `paralleltest`, 1 `thelper`), `domain` 2 before (`paralleltest`, `tparallel`), 0 after.

## Findings and changes

### Removed
- `TestRemoteExecutorEnforcesModelUsageCeiling`, `TestRemoteExecutorRequiresReportedCostUsage` (2 subtests), `TestRemoteExecutorRequiresBudgetTelemetry`, `TestRemoteExecutorSettlesCumulativeUsageWhenTerminalOmitsUsage`: each re-proved a budget or usage rule through a bufconn worker. The rules belong to `domain`, which now proves them case by case (`TestBudgetUsageNamesTheMetricAWorkerExceeds`, `TestStreamOutcomeRefusesStreamsThatDoNotEndInACompleteTerminal`, `TestStreamOutcomeReportsUsageTheWorkerCannotUnderstate`). `app` keeps one path through the layer: `TestExecuteReportsAWorkerBudgetBreachAsATypedError` (T2).
- `TestExecutorWithoutClientRefusesToRun` (facade): the nil-client and nil-request refusals are `app` behavior (`TestExecuteWithoutAWorkerClientRefusesToRun`, `TestExecuteRefusesRequestsItCannotBoundBeforeContactingTheWorker`).
- `TestRequestMappingPreservesValidationPrecedence`, `TestStreamAdmissionPreservesErrorPrecedence` (domain): kept in spirit, see Improved.
- Duplicate fixtures: `validWorkerRequest` was copied in `app` and `domain` and again as `FixtureRequest` in `executorconformance`; `requestWithBudget` in `app`; `testWorkerClient`/`testWorkerClientWithFeatures` bufconn plumbing. All replaced by `executorconformance.FixtureRequest`/`SetBudget`/`EditPayload` and `workerfake.ConnectClient`.

### Renamed or moved
- `TestStreamedWorkerConforms` (from `executorconformance`) → `TestRemoteExecutorConformsToTheExecutorPort` in the executor's own package, running `Run` and `RunCanceled`.
- `TestSeparateProcessWorkerConforms` (from `executorconformance`) → `TestWorkerProcessOnAUnixSocketConformsToTheExecutorPort` in the facade package. This is the transport-specific test of the executor (real UDS, separate process); the contract assertion is the shared `Run`.
- `request_test.go` + `stream_admission_test.go` + `stream_digest_test.go` → `wire_request_test.go`, `enum_mapping_test.go`, `stream_test.go`, `budget_test.go`, `handshake_test.go`, `decision_digest_test.go`, `fixtures_test.go` (T3, T11). In `app`: `executor_test.go` → `executor_test.go`, `handshake_test.go`, `evidence_capability_test.go`, `fixtures_test.go`.
- `TestTransportStatusesClassifyAsContextErrors` → `TestCancellationAndDeadlineStatusesMatchTheContextErrorsAndKeepTheirStatus`; its rule-ID comment (`A8`) is removed (the name and the architecture gate `TestEpisodeLifecycleImportsNoExecutorTransport` carry it).

### Improved
- T5: the process test waited for the worker socket with `time.Sleep(10ms)` in a poll loop. The child now prints `listening` after `ListenEvidenceSocket` returns; the parent blocks on that line (bounded by a one-minute context) or on early exit with the child's stderr. The binary is the test binary re-executed by `TestMain`; nothing is built with `go build`.
- T4: strings.Contains on `err.Error()` replaced by `errors.As` for `*BudgetExceededError` where a type exists, and by the domain message elsewhere.
- T6: every top-level test and subtest is parallel; no `time.Sleep`; `workerfake.SocketDir` for the Unix socket directory.
- Precedence tests now build real event sequences (`eventScript`) instead of poking `sawStarted`/`nextSequence`, except the admission-precedence test, which keeps the minimum state poke.
- Comments removed from tests.

### Added
- `domain` (69.9% → 97.8%), all new behavior tests:
  - `TestWireRequestBindsTheDurableEpisodeToTheWorkerRequest` (identity, digests, budget, keys, trace, dispatch policy).
  - `TestWireRequestRejectsWhatCannotBeBoundToTheEpisode` (24 cases: version, fence, JSON, each missing document, each malformed digest, provenance mismatch, kind, lane, risk, wall time, empty intent catalog, each reconsideration defect) and `TestWireRequestReportsTheEarliestFaultOfSeveral`.
  - `TestStreamAcceptRejectsEventsThatBreakTheEnvelope` (17 cases incl. foreign identity, gap, duplicate started, duplicate decision, decision identity, forged digest), `TestStreamBoundsEventCountAndBytes` (4096 events; 16 MiB single and cumulative), `TestStreamOutcome*` (terminal statuses, produced without decision, unspecified status, telemetry rules, cost settled as the maximum of reports).
  - `TestBudgetUsage*`: every metric (`model_calls`, `tool_calls`, `tool_result_bytes`, `total_tool_result_bytes`, `provider_retries`, `input_tokens`, `output_tokens`, `cost_microunits`) reported by events and by budget updates, consumption exactly at the ceiling, no limit means no enforcement, a worker that under-reports does not lower the count, cumulative usage may not go down.
  - `TestHandshake*`, `TestEvidenceConfigurationFailsClosed`.
- `app` (80.5% → 97.6%): `TestExecuteNegotiatesBeforeSendingTheEpisode` (wrong worker name, unoffered feature, request over the worker's limit: the episode never reaches the worker), `TestExecuteKeepsTheWorkersStatusOnAStreamFailure`, `TestExecuteRefusesRequestsItCannotBoundBeforeContactingTheWorker`, `TestAttemptCapabilityIssuerFailsClosedWithoutAFullScope` (14 cases), scope details (tenant, situation, trace, tools, bounds) in `TestAttemptCapabilityIssuerBindsRequestIdentityAndScope`.
- Facade: `TestExecutorWithEvidenceIssuesTheCapabilityForTheConfiguredEndpoint` (`NewExecutorWithEvidence` wires endpoint and factory), and the conformance runs.

### Speed
- Nothing was slow. The separate-process test dropped its poll sleep; it takes 0.06 s.

## Production code touched
- none

## Invariants proven here
- 6 (model cannot execute effects; worker boundary): the worker receives no evidence endpoint and no capability token unless evidence tools are configured (`TestExecuteGivesTheWorkerNoEvidenceAccessUnlessConfigured`); with them, a fresh capability per dispatch bound to the attempt (`TestExecuteIssuesAFreshScopedCapabilityPerDispatch`, `TestAttemptCapabilityIssuerBindsRequestIdentityAndScope`); an endpoint the runtime cannot authorize is refused before the worker is contacted (`TestExecuteRefusesEvidenceToolsItCannotAuthorizeBeforeContactingTheWorker`, `TestEvidenceConfigurationFailsClosed`, `TestAttemptCapabilityIssuerFailsClosedWithoutAFullScope`); the worker's only output is a Decision accepted solely with a matching terminal and a verified digest (`TestStreamOutcomeCarriesTheDecisionOnlyWithAProducedTerminal`, `TestStreamOutcomeRefusesStreamsThatDoNotEndInACompleteTerminal`, `TestWorkerDecisionDigestRefusesAmbiguousBytesAndMismatches`, `TestStreamAcceptRejectsEventsThatBreakTheEnvelope`); a worker cannot use another attempt's fence (same test). The worker side, `workerfake`, is listed in `testsupport.md`.
- 5: budget enforcement: `TestBudgetUsage*`, `TestExecuteReportsAWorkerBudgetBreachAsATypedError`, `TestExecuteRefusesRequestsItCannotBoundBeforeContactingTheWorker`.
- 8: stable identity, fence and attempt binding on every event: `TestStreamAcceptRejectsEventsThatBreakTheEnvelope`.

## Open items
- `stream.go` `acceptPayload` has a "duplicate terminal" branch that cannot run: `checkOrder` refuses any event after a terminal first (`TestStreamAcceptRejectsEventsThatBreakTheEnvelope/second_terminal` reaches "after terminal"). Dead production code; left alone.
- `transport.Worker.Open`'s success path is covered only through `app`, so the package stays at 87.5%.
- The deadline case of `TestExecuteReturnsContextEndingsTheRunnerClassifies` uses a 50 ms wall time: the worker blocks until its context ends, so the result is deterministic, but the test takes about 50 ms of real time. A virtual clock does not apply: the deadline is enforced by a gRPC/`context.WithTimeout` deadline.
