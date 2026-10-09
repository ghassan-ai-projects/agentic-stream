# Test Bar

Every test in the repository meets this bar by the end of the audit. Each rule
names how it is checked. A rule checked by "review" is checked against the
module report in [modules/](modules/) before the round is committed. The audit
is closed: the mechanical rules are gates in `make ci-check` (a check in
**bold** below fails the build) and are summarized in
[.agents/context/testing.md](../../.agents/context/testing.md).

| ID | Rule | Checked by |
| --- | --- | --- |
| T1 | Tests live with the code they prove. No `_test.go` at the repository root. Repository-wide gates live in `internal/architecture`, one file per gate family, indexed in its README. | **`TestNoTestsAtRepositoryRoot`** (`internal/architecture`), review |
| T2 | A behavior is proven once, at the lowest layer that owns it, plus at most one integration path through the layer above. | review |
| T3 | Names state behavior. A test name is a sentence about the subject (`TestLeaseExpiresAfterItsTTL`). A file name names the subject under test. No phase, round, ticket or wave numbers in file or test names. | review |
| T4 | Every test asserts an observable outcome and prints got and want on failure. Error tests assert which error: `errors.Is`/`errors.As`, or the domain message when no sentinel exists. `err != nil` alone is not an assertion. | review |
| T5 | Tests are deterministic. No `time.Sleep` to wait for work: wait on a channel, a condition or a bounded poll bound to `t.Context()`. Time comes from a virtual clock, identities from deterministic generators. | **`TestTestsNeverSleep`** (`internal/architecture`), `-race -shuffle=on -count=3` on the module (`make coverage-check` shuffles), review |
| T6 | Tests are isolated and parallel. Every top-level test and subtest calls `t.Parallel()` unless it uses `t.Setenv`, `t.Chdir` or process-global state. Use `t.TempDir`, `t.Context`, `t.Cleanup`, `t.Setenv`. No mutable package-level test state. No network beyond loopback and Unix sockets under `t.TempDir`. | **`paralleltest`, `tparallel`, `usetesting`** (`make lint`) |
| T7 | Tests are fast. `go test -short -race ./...` finishes in 35 s wall time on the reference machine (baseline 61 s). No package takes more than 15 s; no test more than 5 s. A test above 5 s is listed in the slow-test register below with its reason. Product acceptance tests (the experiment end-to-end suite, golden replay) are made faster, never skipped under `-short`. | `make coverage-check` runs the suite; timing is measured in [SUMMARY.md](SUMMARY.md) and each module file |
| T8 | Coverage proves behavior. Every package is at 70% statement coverage or above (floor raised from 60% in round 17); the repository total is 80% or above. Every exported facade operation and every error branch that enforces a product invariant or architecture rule has a test. Coverage is never raised with assertion-free tests. | **`scripts/check-coverage.py`** (per-package floor, `make coverage-check`), review for the 80% total and the rest |
| T9 | Fixtures are named and shared. Inputs come from `testdata/` (data private to one module), `examples/` (specs and traces shared by modules) or from named builders (`newApprovedCommand(t, ...)`). Nothing reads from `docs/`, the dated archive. Helpers call `t.Helper()`. Setup repeated in two places moves into one helper; database tests use `storagetest.Open`. | **`thelper`, `dupl`** (`make lint`), **`TestNothingExecutableReadsFromTheDocsArchive`** (`internal/architecture`), review |
| T10 | No dead tests. Delete tests that repeat another test's assertion at the same layer, tests of removed behavior, tests asserting that a constant equals its literal (unless they pin a wire, digest or schema contract), and skips without a live reason. `t.Skip` is allowed only for `testing.Short()` or a missing optional tool, and says which. | review |
| T11 | Tests read top-down. Arrange, act, assert. Table-driven when three or more cases share a shape. A test body longer than about 50 lines extracts named steps. The no-comments rule of AGENTS.md holds inside modules: names say what a comment would. | review |
| T12 | Product invariants are traceable. Each of the ten invariants in [documentation/architecture/invariants.md](../../documentation/architecture/invariants.md) names the tests that prove it. | **`TestEveryInvariantNamesTestsThatExist`** (`internal/architecture`), review of `invariants.md` |

## Layer expectations (T2)

| Layer | Proves | Uses |
| --- | --- | --- |
| `internal/domain` | Pure rules: every branch, boundary and rejection. | Values only. No database, file, clock or network. |
| `internal/store` | SQL: inserts, conflict handling, reads, ordering, transaction joins. | `storagetest.Open`. |
| `internal/app` | Use-case ordering, error precedence, fences, idempotency. | Fakes of ports, or `storagetest.Open` when the transaction is the behavior. |
| `internal/transport`, `internal/wire` | Framing, codecs, protocol errors. | In-memory pipes, Unix sockets in `t.TempDir`. |
| Facade | Wiring and the public contract. Does not re-test domain rules. | The configured facade over `storagetest.Open`. |
| `cmd/agentic-stream` | Product acceptance flows and CLI contracts. | Real binaries and processes, kept few and fast. |

## Test-hygiene lint configuration

`paralleltest`, `tparallel`, `usetesting` and `thelper` are enabled in
[.golangci.yml](../../.golangci.yml) and run in `make lint` and `make lint-ci`
over test files like any other code. A test that cannot be parallel (process-
global state such as `t.Setenv`) or cannot use `t.TempDir` (Unix socket path
length) says why with `//nolint:<linter> // <reason>`.

## Slow-test register (T7)

A test that stays above 5 s under `-short -race` after its round is listed here
with the reason it cannot be faster. Times are for the package run alone
(`go test -short -race -count=1 ./cmd/agentic-stream`, 2026-10-09, after round
17); in the whole-suite run the same tests take 9-14 s because ten packages
compete for the same cores. Every other test in the repository is below 5 s
alone; the next slowest are `TestExperimentShadowReplayComparesTheCandidate`
(1.9 s), `TestRunMigratesAFreshIsolatedDatabaseItself` (3.4 s, cold migrations
by design) and `TestThermalChamberReplayIsDeterministic` (2.5 s).

| Test | Package | Time alone | Reason |
| --- | --- | --- | --- |
| `TestExperimentClosedLoopUnderAContinuousFeed` | `cmd/agentic-stream` | 7.3 s | Product acceptance test: a real `serve`, 64 trace events plus a continuous feed through a race-instrumented pipeline, a debounced episode and a verified command. Spec gates are already shortened in the test copy (debounce and cooldown 2 s, slide 1 s, worker 2 s); the rest is ingestion. |
| `TestExperimentClosedLoopThroughServe` | `cmd/agentic-stream` | 6.0 s | Same; plus the recorded-replay checks. |
| `TestExperimentInterlockStopsEffects` | `cmd/agentic-stream` | 5.7 s | Same: it must ingest the whole trace and reach the policy denial. |

`cmd/agentic-stream` takes 10.1 s alone, inside the 15 s package limit; in the
whole-suite run it is the critical path at 17.5-19 s.
