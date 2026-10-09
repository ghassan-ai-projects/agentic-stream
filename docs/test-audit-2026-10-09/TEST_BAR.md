# Test Bar

Every test in the repository meets this bar by the end of the audit. Each rule
names how it is checked. A rule checked by "review" is checked against the
module report in [modules/](modules/) before the round is committed. Once the
audit closes, the mechanical rules move into `make ci-check` and
[.agents/context/testing.md](../../.agents/context/testing.md).

| ID | Rule | Checked by |
| --- | --- | --- |
| T1 | Tests live with the code they prove. No `_test.go` at the repository root. Repository-wide gates live in `internal/architecture`, one file per gate family, indexed in its README. | `TestNoTestsAtRepositoryRoot`, review |
| T2 | A behavior is proven once, at the lowest layer that owns it, plus at most one integration path through the layer above. | review |
| T3 | Names state behavior. A test name is a sentence about the subject (`TestLeaseExpiresAfterItsTTL`). A file name names the subject under test. No phase, round, ticket or wave numbers in file or test names. | review |
| T4 | Every test asserts an observable outcome and prints got and want on failure. Error tests assert which error: `errors.Is`/`errors.As`, or the domain message when no sentinel exists. `err != nil` alone is not an assertion. | review |
| T5 | Tests are deterministic. No `time.Sleep` to wait for work: wait on a channel, a condition or a bounded poll bound to `t.Context()`. Time comes from a virtual clock, identities from deterministic generators. | `-race -shuffle=on -count=3` on the module, `grep time.Sleep`, review |
| T6 | Tests are isolated and parallel. Every top-level test and subtest calls `t.Parallel()` unless it uses `t.Setenv`, `t.Chdir` or process-global state. Use `t.TempDir`, `t.Context`, `t.Cleanup`, `t.Setenv`. No mutable package-level test state. No network beyond loopback and Unix sockets under `t.TempDir`. | `paralleltest`, `tparallel`, `usetesting` |
| T7 | Tests are fast. `go test -short -race ./...` finishes in 35 s wall time on the reference machine (baseline 61 s). No package takes more than 15 s; no test more than 5 s. A test above 5 s is listed in the slow-test register below with its reason. Product acceptance tests (the experiment end-to-end suite, golden replay) are made faster, never skipped under `-short`. | timing report in each module file |
| T8 | Coverage proves behavior. Every package is at 70% statement coverage or above (floor raised from 60% at the end of the audit); the repository total is 80% or above. Every exported facade operation and every error branch that enforces a product invariant or architecture rule has a test. Coverage is never raised with assertion-free tests. | `scripts/check-coverage.py`, review |
| T9 | Fixtures are named and shared. Inputs come from `testdata/` or from named builders (`newApprovedCommand(t, ...)`). Helpers call `t.Helper()`. Setup repeated in two places moves into one helper; database tests use `storagetest.Open`. | `thelper`, `dupl`, review |
| T10 | No dead tests. Delete tests that repeat another test's assertion at the same layer, tests of removed behavior, tests asserting that a constant equals its literal (unless they pin a wire, digest or schema contract), and skips without a live reason. `t.Skip` is allowed only for `testing.Short()` or a missing optional tool, and says which. | review |
| T11 | Tests read top-down. Arrange, act, assert. Table-driven when three or more cases share a shape. A test body longer than about 50 lines extracts named steps. The no-comments rule of AGENTS.md holds inside modules: names say what a comment would. | review |
| T12 | Product invariants are traceable. Each of the ten invariants in [documentation/architecture/invariants.md](../../documentation/architecture/invariants.md) names the tests that prove it. | review of `invariants.md` |

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

Until the final round adds these linters to `.golangci.yml`, a round checks its
module with this configuration (save it outside the repository):

```yaml
version: "2"
linters:
  default: none
  enable: [paralleltest, tparallel, usetesting, thelper]
run:
  tests: true
```

```bash
golangci-lint run --config <path>/lint-tests.yml --max-issues-per-linter 0 --max-same-issues 0 ./internal/<module>/...
```

## Slow-test register (T7)

A test that stays above 5 s under `-short -race` after its round is listed here
with the reason it cannot be faster.

| Test | Package | Time | Reason |
| --- | --- | --- | --- |
| `TestExperimentClosedLoopUnderAContinuousFeed` | `cmd/agentic-stream` | 7.7-8.0 s | Product acceptance test: a real `serve`, 64 trace events plus a continuous feed through a race-instrumented pipeline, a debounced episode and a verified command. Spec gates are already shortened in the test copy (debounce and cooldown 2 s, slide 1 s, worker 2 s); the rest is ingestion. |
| `TestExperimentClosedLoopThroughServe` | `cmd/agentic-stream` | 6.1-7.1 s | Same; plus the recorded-replay checks. |
| `TestExperimentInterlockStopsEffects` | `cmd/agentic-stream` | 5.6-6.6 s | Same: it must ingest the whole trace and reach the policy denial. |
