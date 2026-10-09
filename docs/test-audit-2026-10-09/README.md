# Test audit (2026-10-09)

Status: in progress (rounds 0, 1, 3 of 17 done)

A full audit of the repository's tests: organize them so the repository is easy
to read, make them faster, raise coverage where it proves behavior, and remove
what proves nothing. Every test ends at the [test bar](TEST_BAR.md).

| Document | Content |
| --- | --- |
| [TEST_BAR.md](TEST_BAR.md) | The bar, rules T1–T12, and the slow-test register |
| [BASELINE.md](BASELINE.md) | Coverage, timing and lint numbers before the audit |
| [WORKER_BRIEF.md](WORKER_BRIEF.md) | What a round's worker does and how it reports |
| [modules/](modules/) | One report per module: findings, changes, metrics |

## Method

- An orchestrator plans each round, briefs a worker, reviews the diff against
  the bar, runs the full gate and commits the round. Investigation helpers
  answer read-only questions.
- Rounds go bottom-up through the dependency layers, so a module's tests are
  audited after the modules it uses.
- Rounds touching disjoint modules may run at the same time; each is committed
  on its own.
- Each round ends with a commit `test(<module>): ...` and an updated row below.
- The last round turns the mechanical rules into gates: the test-hygiene
  linters in `.golangci.yml`, the coverage floor at 70%, and the updated
  testing context.

## Goals

| Measure | Baseline | Target |
| --- | --- | --- |
| Root test files | 29 | 0 |
| `-short -race` wall time | 61 s | ≤ 35 s |
| Total coverage | 75.6% | ≥ 80% |
| Packages below 70% | 21 | 0 |
| `paralleltest` + `tparallel` + `usetesting` findings | 610 | 0 |

## Tracker

| Round | Modules | Status | Coverage before → after | Time before → after | Commit |
| --- | --- | --- | --- | --- | --- |
| 0 | audit folder, bar, baseline | done | - | - | - |
| 1 | repository root → `internal/architecture` | done | - | 2.8 s → 1.6 s | 5f289206 |
| 2 | `kernel`, `canonicaljson`, `sources`, `contractsv1`, `interlock` | todo | | | |
| 3 | `storage`, `telemetry`, `migrations` | done | every package ≥ 81% (storage 64 → 100, telemetry 66.7 → 100) | store 8.4 s → 4.0 s, storagetest 5.4 s → 3.5 s | see git log |
| 4 | `spec`, `ingress` | todo | | | |
| 5 | `eventlog`, `engine` | todo | | | |
| 6 | `operators`, `situations`, `cognition` | todo | | | |
| 7 | `episodes` | todo | | | |
| 8 | `episodeledger`, `approvalledger` | todo | | | |
| 9 | `evidence`, `decisions` | todo | | | |
| 10 | `executor/*`, `worker`, `testsupport/*` | todo | | | |
| 11 | `policy`, `authority` | todo | | | |
| 12 | `actions`, `actionport`, `watch`, `notify` | todo | | | |
| 13 | `device`, `control` | todo | | | |
| 14 | `replay`, `runartifact` | todo | | | |
| 15 | `runtime`, `api` | todo | | | |
| 16 | `cmd/agentic-stream` | todo | | | |
| 17 | gates: linters, coverage floor, invariant map, docs | todo | | | |

## Findings for the owner

Behavior questions a round found but did not change, because the decision is
not a test decision.

| Round | Finding | Where |
| --- | --- | --- |
| 3 | `telemetry.RecordError` promises no payload leakage, but `span.RecordError(err)` stores the error text in an `exception` span event; only the status description is fixed. Current behavior is pinned by a test. | [telemetry.md](modules/telemetry.md) |
| 3 | `storage.OpenFresh` could copy a migrated template instead of migrating: saves about 1.3 s per isolated replay under `-race`, about 0.06 s in production. Not done; revisit after the replay round. | [storage.md](modules/storage.md) |

