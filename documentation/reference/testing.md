# Testing reference

Choose checks that match your change. This page lists the repository targets,
what each kind of test demonstrates, and the minimum checks for documentation
and code changes.

## Standard targets

| Target | Purpose |
| --- | --- |
| `make build` | compile `cmd/...` into `bin/agentic-stream` |
| `go test ./...` | all package tests |
| `make test` | race, shuffle, one run, coverage profile |
| `make test-short` | short race-enabled suite |
| `make coverage-check` | short race-enabled suite used by CI, with the 70% per-package coverage floor |
| `make test-race` | race-enabled tests without shuffle/coverage wrapper |
| `make test-coverage` | HTML coverage report |
| `go vet ./...` | static analysis |
| `make lint` | golangci-lint, including the test-hygiene linters (`paralleltest`, `tparallel`, `usetesting`, `thelper`) |
| `make proto-check` | generated worker stubs match source |
| `make ci-check` | local CI-equivalent gate |
| `make docs-check` | public docs structure/surface checks |
| `make plant-wire-sync` | copy the plant wire contract from `PLANT_WIRE_CONTRACT_DIR` byte-identically and run its guard tests (digest, version, safety members, byte identity) |
| `git diff --check` | whitespace/error check |

The streamsim tests run the real streams-simulator plant wire node against the
host: `TestServeHostsTwoSessionsOfTheStreamsimNodeWithoutARestart` (two sessions
without a restart), `TestThePlantWireEffectorCommandsAPacedStreamsimSession`
(both bench commands verified by the twin's readings) and
`TestTheFanExperimentClosesTheLoopThroughAPacedStreamsimTwin` (the fan
experiment's simulated rung through `serve`: streamsim's own
`fan-experiment/simulate.plan.json` with the zone heat load moved to 5 s and
Agentic Stream's policy digest filled in, paced to the wall clock; the bench
spec opens a situation, a Tamoz-shaped worker proposes `set_indicator alert`
and then `select_thermal_mode bounded_cooling`, both reach the twin as
`show_state` and `set_cooling {level_percent: 45}` and are verified by its
readings, with no quarantined event; about 45 s). The same loop runs in the
default suite against a fake bench node:
`TestTheBenchLoopSendsTheFanItsBoundedLevelAndItsReadingVerifiesIt` (the fan
command verified by its reading) and
`TestAnUnknownPlantWireOutcomeIsReconciledByTheDevicesOwnReading` (an unknown
outcome reconciled from a reading). The streamsim tests are skipped unless
`STREAMSIM_BIN` names a streams-simulator binary built after the streamsim
checkout's HEAD commit; the checkout is the binary's grandparent directory, or
`STREAMSIM_REPO` when the binary is built elsewhere:

```bash
go build -o /tmp/streamsim ./cmd/streamsim   # in the streams-simulator checkout
STREAMSIM_BIN=/tmp/streamsim STREAMSIM_REPO=<streams-simulator checkout> \
  go test ./cmd/agentic-stream -run Streamsim
```

The test kills the node at the test deadline, so a hung node cannot stall the run.

The rule-promotion campaign driver (`internal/testsupport/campaign`,
`TestCampaignDrivesTheConsumerThroughItsRuns`) also needs `CAMPAIGN_DIR`, a
folder made by `streamsim suite campaign`. `CAMPAIGN_ARM` picks the arm (`0`,
`A`, `B` (default) or `C`), `CAMPAIGN_RUNS` how many runs to drive (default 1),
`CAMPAIGN_OUT` where the database, run folders, verdicts, `responses.jsonl` and
`campaign-score.json` are kept (default a temporary folder) and
`CAMPAIGN_REPLAY` a `responses.jsonl` to replay instead of asking the canned
worker. `CAMPAIGN_TAMOZ_SOCKET` forwards every episode and draft to a running
Tamoz worker on that socket instead, and `CAMPAIGN_LOG_EXCHANGES=1` then logs
each forwarded request and every event of its response to
`worker-exchanges.jsonl` beside `responses.jsonl`:

```bash
STREAMSIM_BIN=/tmp/streamsim STREAMSIM_REPO=<streams-simulator checkout> CAMPAIGN_DIR=<campaign> \
  CAMPAIGN_ARM=B CAMPAIGN_RUNS=2 CAMPAIGN_OUT=<out> go test ./cmd/agentic-stream -run Campaign -timeout 90m
```

Optional local tools such as `deadcode` and `govulncheck` are reported as
skipped by the Makefile when not installed.

## Evidence categories

- Unit tests prove local package rules.
- Integration tests prove sockets, storage, worker, notification, and telemetry
  boundaries.
- Pipeline E2E tests prove event-to-effect composition.
- Replay tests prove deterministic/no-effect behavior.
- Conformance tests prove executor/worker protocol behavior.

Green tests do not replace deployment qualification, backup/restore rehearsal,
or external effector review.

## Documentation changes

Documentation-only changes must run `make docs-check` and `git diff --check` at
minimum. Run the broader suite when examples, contract references, Makefile,
CI, or runtime commands change.

## Next reads

- [Quality](../governance/quality.md)
- [Release evidence](../governance/release.md)
- [Contributing](../../CONTRIBUTING.md)
