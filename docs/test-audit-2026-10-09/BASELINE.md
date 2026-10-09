# Baseline (2026-10-09, commit 4bb694c5)

Measured with `go test -short -race -count=1 -json -coverprofile ./...` on the
reference machine (Apple silicon laptop, 10 cores).

## Totals

| Measure | Value |
| --- | --- |
| Wall time, `-short -race` | 61.0 s (334 s user) |
| Packages | 135 (133 with statements) |
| Test files | 449, about 49 800 lines |
| Root test files | 29 (package `agenticstream`, about 3 000 lines) |
| Statement coverage, total | 75.6% |
| Packages below 70% | 21 |
| Packages below 80% | 48 |
| `paralleltest` findings | 580 |
| `tparallel` findings | 19 |
| `usetesting` findings | 11 |

## Slowest packages

| Time | Coverage | Package |
| --- | --- | --- |
| 58.3 s | 74.7% | `cmd/agentic-stream` |
| 37.5 s | 60.0% | `internal/replay` |
| 10.5 s | 68.9% | `internal/replay/internal/app` |
| 10.5 s | 79.5% | `internal/policy/internal/app` |
| 10.4 s | 78.4% | `internal/storage/internal/store` |
| 8.0 s | 81.4% | `internal/storage/storagetest` |
| 7.7 s | 81.0% | `internal/runtime/internal/app` |
| 6.7 s | 68.7% | `internal/replay/internal/transport` |
| 3.9 s | - | repository root |
| 3.8 s | 64.0% | `internal/storage` |

## Slowest tests

| Time | Package | Test |
| --- | --- | --- |
| 45.6 s | `cmd/agentic-stream` | `TestExperimentClosedLoopThroughServe` |
| 41.2 s | `cmd/agentic-stream` | `TestExperimentClosedLoopUnderAContinuousFeed` |
| 37.8 s | `cmd/agentic-stream` | `TestExperimentInterlockStopsEffects` |
| 34.5 s | `internal/replay` | `TestThermalChamberReplayIsDeterministic` |
| 23.7 s | `internal/replay` | `TestShadowReplayPersistsPairedComparisonWithoutEffects` |
| 23.4 s | `internal/replay` | `TestShadowReplayValidatesAnExecutableOpportunity` |
| 23.0 s | `internal/replay` | `TestWorkerAwareModesUseOnlySuppliedCapabilities` |
| 19.4 s | `cmd/agentic-stream` | `TestRunRepeatProvesDeterminism` |
| 17.9 s | `internal/replay` | `TestBenchSpecOpensOnTemperatureAndHeartbeatAlone` |
| 15.1 s | `internal/replay` | `TestThermalChamberRebootWrapAndBacklogStayBootScoped` |

Times of parallel tests overlap; a package's time is its wall time.

## Lowest coverage

| Coverage | Package |
| --- | --- |
| 60.0% | `internal/replay` |
| 63.9% | `internal/episodes/internal/app` |
| 64.0% | `internal/storage` |
| 64.5% | `internal/episodeledger` |
| 66.7% | `internal/cognition`, `internal/control/controltest`, `internal/interlock/internal/domain`, `internal/telemetry`, `internal/testsupport/executorconformance` |
| 67.1% | `internal/eventlog/internal/store` |
| 67.5% | `internal/episodeledger/internal/store` |
| 68.0% | `internal/notify/internal/store` |
| 68.2% | `internal/runtime/internal/store` |
| 68.7% | `internal/replay/internal/transport` |
| 68.8% | `internal/cognition/internal/store` |
| 68.9% | `internal/replay/internal/app`, `internal/spec/internal/store` |
| 69.3% | `internal/episodes/internal/store` |
| 69.4% | `internal/worker/internal/transport` |
| 69.9% | `internal/executor/remote/internal/domain` |
