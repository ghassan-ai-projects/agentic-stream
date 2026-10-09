# Test audit summary (2026-10-09)

The audit is closed. Every test in the repository was read against the
[test bar](TEST_BAR.md); the mechanical rules are now gates in `make ci-check`.
The per-module findings are in [modules/](modules/), the open decisions for the
owner in the [findings table](README.md#findings-for-the-owner).

## Final numbers

Measured on 2026-10-09 after round 17, on the reference machine (Apple silicon
laptop, 10 cores), with the baseline's command
`go test -short -race -count=1 -json -coverprofile=... ./...`. The wall time is
the best of three consecutive runs (35.3 s, 35.6 s, 37.5 s). "Alone" means the
package run by itself; in the whole-suite run the packages compete for cores and
the longest tests stretch.

| Measure | Baseline ([BASELINE.md](BASELINE.md)) | Final | Target |
| --- | --- | --- | --- |
| Wall time, `-short -race` | 61.0 s (334 s user) | 35.3 s (207 s user) | ≤ 35 s |
| Slowest package | 58.3 s `cmd/agentic-stream` | 10.1 s alone, 17.5–19.2 s in the whole run | ≤ 15 s |
| Second slowest package | 37.5 s `internal/replay` | 4.9 s alone, 9.4–10.5 s in the whole run | |
| Slowest test | 45.6 s `TestExperimentClosedLoopThroughServe` | 7.3 s alone (`TestExperimentClosedLoopUnderAContinuousFeed`), 12.9–13.9 s in the whole run | ≤ 5 s or registered |
| Tests above 5 s alone | at least 10 | 3 (the experiment acceptance tests, in the [register](TEST_BAR.md#slow-test-register-t7)) | registered |
| Test files | 449 | 664 | |
| Test lines | about 49 960 | about 67 500 | |
| Top-level tests | 1 373 | 2 122 | |
| Root test files | 29 | 0 | 0 |
| Statement coverage, total | 75.6% | 85.6% | ≥ 80% |
| Packages below 70% | 21 | 0 | 0 |
| Packages below 80% | 48 | 7 | |
| Lowest package coverage | 60.0% (`internal/replay`) | 75.6% (`internal/runartifact/internal/app`) | ≥ 70% |
| `paralleltest` + `tparallel` + `usetesting` findings | 610 (580 + 19 + 11) | 0 | 0 |
| `time.Sleep` in tests | present | 0, gated by `TestTestsNeverSleep` | 0 |

Packages still below 80%: `internal/runartifact/internal/app` (75.6%),
`internal/runartifact/internal/transport` (76.6%),
`internal/worker/internal/transport` (78.8%), `internal/control/internal/store`
(79.2%), `internal/runartifact/internal/domain` (79.4%),
`internal/runtime/internal/composition` (79.6%) and
`internal/control/internal/app` (79.8%). The remaining lines are error branches
of operating-system calls and composition wiring that the unit layer cannot
reach without assertion-free tests (see each module report).

The wall time misses its 35 s target by 0.3–2.5 s run to run. The critical
path is `cmd/agentic-stream`: three product acceptance tests that ingest a whole
trace through a race-instrumented pipeline and may not be skipped under
`-short`. They are shortened as far as the spec gates allow.

## What changed, by round

| Round | Commit | Change |
| --- | --- | --- |
| 0 | `f8ea2ab9` | Opened the audit: bar, baseline, worker brief. |
| 1 | `5f289206` | Moved the 29 root test files into `internal/architecture`, one file per gate family, indexed in its README. |
| 2 | `592b586f` | `kernel`, `canonicaljson`, `sources`, `contractsv1`, `interlock`: every package at 90% or above. |
| 3 | `ff3a06ce` | `storage`, `telemetry`, `migrations`: storage 64 to 100%, store time 8.4 s to 4.0 s. |
| 4 | `27f1968f` | `spec`, `ingress`: compile 26.9 ms to 6.9 ms under `-race` (schema compiled once). |
| 5 | `6fb148d2` | `eventlog`, `engine`: every package at 87% or above, no sleeps. |
| 6 | `ede78cf3` | `operators`, `situations`, `cognition`: cognition 66.7 to 100%, 59 admission cases instead of 15. |
| 7 | `f29e6d29` | `episodes`: sleep replaced by the virtual clock, no test above 0.7 s. |
| 8 | `5f8d47a7` | `episodeledger`, `approvalledger`: the attempt state machine checked over all 81 pairs. |
| 9 | `a0e50cc1` | `evidence`, `decisions`: slowest test 0.32 s, every package at 87% or above. |
| 10 | `995b3c40` | `executor/*`, `worker`, `testsupport/*`: conformance no longer re-executes a process, slowest test 1.41 s to 0.34 s. |
| 11 | `046acfd7` | `policy`, `authority`: policy app 8.5 s to 3.4 s; the revalidation table (invariant 7) and the risk-class table. |
| 12 | `8a4cd57c` | `actions`, `actionport`, `watch`, `notify`: crash between effect and outcome proven, notify store 68 to 93%. |
| 13 | `46092904` | `device`, `control`: lease loss, epoch drain, cost ceiling and kill switch named, no sleeps. |
| 14 | `c2067055` | `replay`, `runartifact`: `replay` 31–36 s to 5.4–6.5 s (migrated-template opener, concurrent repeats). |
| 15 | `f592033b` | `runtime`, `api`: runtime app 6.0 s to 3.1 s, four sleeps replaced by waits on conditions. |
| 16 | `6df1a2bb`, `d91394a2` | `cmd/agentic-stream`: 51–58 s to 10 s; experiment tests 33–39 s to 5–8 s with durable-row waits and pre-seeded databases. |
| 17 | this change | Gates: test-hygiene linters in `make lint`, 70% coverage floor, `TestTestsNeverSleep`, `TestEveryInvariantNamesTestsThatExist`, the invariants page names its proving tests, the testing context holds the bar. |

## Gates added in round 17

| Gate | Enforces | Where |
| --- | --- | --- |
| `paralleltest`, `tparallel`, `usetesting` | T6: parallel, isolated tests using `t.Context`, `t.TempDir`, `t.Setenv` | `.golangci.yml`, `make lint` |
| `thelper` (already enabled), `dupl` | T9: helpers call `t.Helper()`, no copied test code | `.golangci.yml` |
| `MIN_COVERAGE = 70.0` | T8: every package at 70% or above | `scripts/check-coverage.py`, `make coverage-check` |
| `TestTestsNeverSleep` | T5: no `time.Sleep` in a `_test.go` file, allowlist empty | `internal/architecture/test_hygiene_test.go` |
| `TestEveryInvariantNamesTestsThatExist` | T12: each invariant lists proving tests that exist | `internal/architecture/invariants_test.go` |
| `TestNoTestsAtRepositoryRoot` | T1 | `internal/architecture/repository_test.go` |

The existing exclusions and thresholds of `.golangci.yml` are unchanged. Seven
`//nolint` directives remain in tests, each with its reason: five `usetesting`
(Unix socket path length) and two `paralleltest` (the telemetry tests replace
the process-wide tracer provider).

## Production changes made by the audit

Everything else in the audit is tests and documentation. The production changes
are small and behavior-preserving, each recorded in its module report:

- `spec` compiles the embedded SituationSpec schema once per process (round 4).
- `replay`: `RunNTimes` runs its independent repeats concurrently and keeps run
  order; replay databases can be opened through an injectable opener, set only
  by tests through `internal/replay/replaytest` (round 14).
- `runtime`: `PipelineConfig.MaintenanceInterval` (unset keeps 1 s) and a
  watch page-size constant (round 15).
- `storage` (round 17): `internal/storage/internal/store/stored_time.go`
  registers the `stored_time_ok` SQLite function in `init` instead of on the
  first `storage.Open`. The first run of the 70% gate failed with a data race
  inside `modernc.org/sqlite`: the driver's user-defined-function map is
  unsynchronized, so a connection opened outside `storage.Open` raced with the
  registration made by a concurrent first `storage.Open` (since round 14,
  `run --repeat` replays open their databases at the same time). Registering
  before `main` and before any test removes the window; the error still surfaces
  from `storage.Open`. Proven by
  `TestStoredTimeFunctionIsRegisteredBeforeAnyDatabaseIsOpened`.

## Remaining open items

- **Wall time** is 35.3–37.5 s against the 35 s target, and `cmd/agentic-stream`
  is 17.5–19.2 s in the whole-suite run against the 15 s package limit (10.1 s
  alone). Both come from the three acceptance tests of the
  [register](TEST_BAR.md#slow-test-register-t7). Further gains need a lighter
  trace for the acceptance tests or a cheaper race-instrumented ingestion path
  in `internal/engine` and `internal/storage` (see the round 14 finding).
- **Findings for the owner**: behavior questions and bugs found by the audit and
  deliberately not changed, in the [README table](README.md#findings-for-the-owner);
  issues [#51](https://github.com/ghassan-ai-projects/agentic-stream/issues/51),
  [#52](https://github.com/ghassan-ai-projects/agentic-stream/issues/52),
  [#53](https://github.com/ghassan-ai-projects/agentic-stream/issues/53) and
  [#54](https://github.com/ghassan-ai-projects/agentic-stream/issues/54) track
  four of them.
- **Duplicate socket-directory helpers**: five test files
  (`cmd/agentic-stream/experiment_e2e_test.go`,
  `internal/evidence/private_socket_test.go`,
  `internal/ingress/internal/transport/listener_test.go`,
  `internal/ingress/internal/app/live_serve_test.go`,
  `internal/worker/internal/transport/socketdir_test.go`) each carry their own
  `os.MkdirTemp` socket directory beside `workerfake.SocketDir`. Folding them
  into the shared helper needs an import edge from those test packages to
  `internal/testsupport/workerfake`.
- **Seven packages between 75% and 80%** (listed above); raise them with
  behavior tests as those modules change.
- **`make vulncheck`** flags the local go1.27.1 standard library; the finding
  concerns the toolchain, not the repository, and CI uses its pinned toolchain.
- **Deployment qualification** remains a separate release gate (AGENTS.md);
  green tests do not replace it.
