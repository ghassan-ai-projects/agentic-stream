# Testing Context

## Authoritative Commands

Use these commands unless the task is documentation-only:

- `go test ./...`
- `go test -race -shuffle=on -count=3 ./internal/<module>/...` for the module you changed
- `go test ./internal/architecture` (the repository-wide gates)
- `go vet ./...`
- `make build`
- `make lint` (includes the test-hygiene linters `paralleltest`, `tparallel`, `usetesting` and `thelper`)
- `make ci-check`
- `git diff --check`
- `pre-commit run --all-files` if `pre-commit` is installed

## Make Targets And CI

- `make ci-check` runs `proto-check -> tidy -> build -> vet -> lint-ci -> coverage-check -> deadcode -> vulncheck -> docs-check`. CI splits it: `.github/workflows/ci.yml` runs `make ci-check-go` (everything but `proto-check`) on every change, and `.github/workflows/proto.yml` runs `make proto-check` only when the protocol, its stubs or their generation inputs change. `coverage-check` runs the short race suite once and enforces the per-package coverage floor of 70% (`scripts/check-coverage.py`).
- In restricted environments, `golangci-lint` can fail because it writes outside the workspace cache.
- `make deadcode` (`scripts/check-deadcode.sh`) runs `deadcode ./...` without `-test` and fails when a production function is reachable only from tests. Test-support packages are exempt: `internal/testsupport/...` and packages whose name ends in `test` (`controltest`, `spectest`, ...). Fix a finding by wiring the function into production, deleting it, or moving it to test support (an `export_test.go` for a package's own tests).
- `deadcode` and `govulncheck` are optional locally when the tools are missing; the Makefile reports that explicitly. CI installs the pinned versions, so the gates always run there.

## Golden Replay

`TestGoldenTracesMatchTheirRecordedResults` (`internal/replay`) replays every
example trace and compares events processed, version count, the versions hash
and each entity's phase sequence with `internal/replay/testdata/golden/*.json`.
A trace that publishes no version must say `expect_empty` in its golden file.
When a reviewed behavior change moves these results, regenerate with
`go test ./internal/replay -run TestGoldenTracesMatchTheirRecordedResults -update`
and review the diff of the golden files in the same change.

## Repository-Wide Gates

Tests that check the whole repository (import layering, SQL ownership, facade shape, file size, kernel purity) live in `internal/architecture`, one file per gate family, indexed in its [README](../../internal/architecture/README.md). The repository root holds no tests; `TestNoTestsAtRepositoryRoot` fails when one appears. A test that proves one module's behavior lives in that module.

## The Test Bar

Every test meets the rules T1-T12, which name how each is checked. In short:

- **Place (T1, T2).** A test lives with the code it proves, at the lowest layer that owns the behavior, plus at most one integration path through the layer above. Nothing at the repository root; repository-wide gates live in `internal/architecture`.
- **Name (T3).** The name is a sentence about the subject (`TestLeaseExpiresAfterItsTTL`); the file is named for the subject under test. No phase, round, ticket or wave numbers.
- **Assert (T4).** Every test checks an observable outcome and prints got and want. Error tests assert which error (`errors.Is`, `errors.As`, or the domain message), never `err != nil` alone. `TestErrorAssertionsNameTheErrorTheyExpect` ratchets the remaining assertions that accept any error: a file may only lower its count.
- **Deterministic (T5).** A test never sleeps to wait for work: `TestTestsNeverSleep` fails on any `time.Sleep` in a `_test.go` file. Wait on a channel, a condition, or a bounded poll bound to `t.Context()`; take time from a virtual clock (`sources.NewVirtual`) and identities from deterministic generators.
- **Isolated and parallel (T6).** Every top-level test and subtest calls `t.Parallel()`; use `t.TempDir`, `t.Context`, `t.Cleanup`, `t.Setenv`. A test that cannot be parallel or cannot use `t.TempDir` (process-global state, Unix socket path length) says why in `//nolint:paralleltest // <reason>`. No mutable package-level test state; no network beyond loopback and Unix sockets.
- **Fast (T7).** `go test -short -race ./...` finishes in 35 s on the reference machine; no package takes more than 15 s and no test more than 5 s, unless it is in the slow-test register. Product acceptance tests are made faster, never skipped under `-short`.
- **Covered (T8).** Every package is at 70% or above, the repository at 80% or above. Every exported facade operation and every error branch that enforces an invariant or architecture rule has a test. Coverage is never raised with assertion-free tests.
- **Shared fixtures (T9).** Inputs come from `testdata/`, `examples/` or a named builder, never a local `docs/` directory; helpers call `t.Helper()`; setup repeated in two places moves into one helper.
- **No dead tests (T10).** Delete tests that repeat another assertion at the same layer, test removed behavior or assert that a constant equals its literal. `t.Skip` only for `testing.Short()` or a missing optional tool, and it says which.
- **Read top-down (T11).** Arrange, act, assert; table-driven when three or more cases share a shape; a body over about 50 lines extracts named steps; no comments inside a module.
- **Traceable (T12).** The ten product invariants name their proving tests in [invariants.md](../../documentation/architecture/invariants.md); `TestEveryInvariantNamesTestsThatExist` checks that each listed test exists. A change that touches an invariant updates that list.

## Layers

| Layer | Proves | Uses |
| --- | --- | --- |
| `internal/domain` | Pure rules: every branch, boundary and rejection | Values only; no database, file, clock or network |
| `internal/store` | SQL: inserts, conflicts, reads, ordering, transaction joins | `storagetest.Open` |
| `internal/app` | Use-case ordering, error precedence, fences, idempotency | Fakes of ports, or `storagetest.Open` when the transaction is the behavior |
| `internal/transport`, `internal/wire` | Framing, codecs, protocol errors | In-memory pipes, Unix sockets |
| Facade | Wiring and the public contract, not the domain rules | The configured facade over `storagetest.Open` |
| `cmd/agentic-stream` | Product acceptance flows and CLI contracts | Real binaries and processes, kept few and fast |

## Seeding Fixtures

- Databases: `storagetest.OpenTemp(t)` opens a migrated database in `t.TempDir` and closes it with the test; `storagetest.Open(ctx, path)` is the same for a path you choose. `storagetest.OpenTempWithoutForeignKeys(t)` is for tests that seed rows without their parents. Replaying every migration costs about a second per database under `-race`; the template is migrated once per migration set. Call `storage.Open` only where the test is about opening or migrating itself (`internal/storage/**`).
- CLI tests (`cmd/agentic-stream`): `newMigratedDatabasePath(t)` and `seedMigratedDatabase(t, path)` pre-seed `--db` from the template so each invocation skips the migrations.
- Replay: `replaytest.WithDatabaseOpener(ctx, storagetest.Open)` makes replays start from the migrated template instead of `storage.OpenFresh`; the facade tests use `replay.WithSeededDatabases(t.Context())`. Keep a few tests on the real opener.
- Worker sockets: `workerfake.SocketDir(t)` returns a private 0700 directory with a path short enough for a Unix socket (`t.TempDir` paths exceed the 104-byte macOS limit). Use `workerfake.Server` as the model of a worker and `executorconformance` for executor contracts.
- Process-global state: a test that calls `t.Setenv` or `t.Chdir` cannot be parallel; the experiment end-to-end tests in `cmd/agentic-stream` set their process environment once instead so they can run together.
- Time and identity: `sources.NewVirtual` for the clock, deterministic generators from `internal/sources` for identities.
- Fixture files: a spec or trace that several modules use lives in `examples/<name>/` (`examples/predictive-maintenance/predictive-maintenance.situation.yaml`, `examples/thermal-chamber/zone-thermal.situation.yaml`, traces under `examples/<name>/testdata/`); data private to one module lives in its `testdata/` (the runtime walkthrough spec). Never read from `docs/` (a local, git-ignored directory): `TestNothingExecutableReadsFromTheDocsArchive` fails on any Go string literal or `Join` element naming it, and on any Makefile line using it. Keep the path in one named constant per test package.
- Domain data (event schemas, simulator channels, intent catalog) comes from the JSON files named in AGENTS.md, never a Go literal.

## Test Quality Bar For Changes

For production-code changes:

- write a failing or expectation-setting test before implementation when feasible
- add or update `*_test.go` files in every modified package
- cover new exported behavior and meaningful branches
- use table-driven tests where that improves clarity
- avoid network calls in unit tests

If test-first is not feasible:

- say why explicitly in the plan or handoff
- still add the proving test in the same change
- make sure the test demonstrates the behavior change, not just execution

For documentation-only changes:

- run the narrowest useful checks
- still run `git diff --check`
- explain skipped commands in the final handoff

## Failure Handling

- Fix failures caused by your changes before stopping.
- If a command fails for an existing repo issue or environment restriction, say so clearly and do not misreport it as validated.
