# Test audit (2026-10-09)

Status: in progress (rounds 0–7 of 17 done)

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
| 2 | `kernel`, `canonicaljson`, `sources`, `contractsv1`, `interlock` | done | every package ≥ 90% (interlock domain 66.7 → 100, contractsv1 73.3 → 100) | unchanged (≈1.2 s each, race start-up) | 592b586f |
| 3 | `storage`, `telemetry`, `migrations` | done | every package ≥ 81% (storage 64 → 100, telemetry 66.7 → 100) | store 8.4 s → 4.0 s, storagetest 5.4 s → 3.5 s | ff3a06ce |
| 4 | `spec`, `ingress` | done | every package ≥ 85% (spec store 68.9 → 86.5, spectest 71.4 → 85.7) | ≈1–2 s each; spec compile 26.9 → 6.9 ms under `-race` (schema compiled once) | 27f1968f |
| 5 | `eventlog`, `engine` | done | every package ≥ 87% (eventlog store 67.1 → 92.2, app 71.5 → 94.9) | ≤ 3.3 s each, no sleeps | 6fb148d2 |
| 6 | `operators`, `situations`, `cognition` | done | every package ≥ 83% (situations domain 72.3 → 93.8, cognition 66.7 → 100, store 68.8 → 83.2) | ≈1.3–2.8 s each; cognition app proves 59 cases instead of 15 | ede78cf3 |
| 7 | `episodes` | done | every package ≥ 84% (app 63.9 → 84.3, store 69.3 → 93.0) | no test above 0.7 s, sleep replaced by virtual clock | f29e6d29 |
| 8 | `episodeledger`, `approvalledger` | todo | | | |
| 9 | `evidence`, `decisions` | done | every package ≥ 87% (evidence app 81.5 → 98.1, decisions domain 84.5 → 95.3) | slowest test 0.32 s | see git log |
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
| 2 | `interlock.Assert` documents that a missing row fails closed wrapping `ErrTripped`; it wraps `sql.ErrNoRows` instead (still fails closed). Fix the comment or the code. | [interlock.md](modules/interlock.md) |
| 2 | canonicaljson accepts a native `float64` above 2^53 but refuses the same value as a raw JSON integer; both pinned, needs a design decision. `containsSurrogate` has an unreachable branch. | [canonicaljson.md](modules/canonicaljson.md) |
| 4 | Bug: `cleanLiveListener.closeAndRemove` (`internal/ingress/internal/transport/listener.go`) should remove the socket file only if it is still the listener's, but `net.UnixListener.Close` unlinks the path first, so a replacing file is deleted and `removeSocketFile` is unreachable. Fix: `SetUnlinkOnClose(false)` in `listenSocket`, as its own change. | [ingress.md](modules/ingress.md) |
| 5 | A single event can chain several Situation transitions and publish only the last version, so published versions can start at 2. Confirm this is intended (situations round checks it). `RunGlobal` counts an event already in the inbox as processed. | [engine.md](modules/engine.md) |
| 7 | An executor outcome with a non-terminal status and no decision (for example `running`) makes `RunOnce` return an error and leaves the attempt `running`. With cost control on, `Settle` for an episode that never reserved errors after the attempt ran. | [episodes.md](modules/episodes.md) |
| 7 | Production files under `internal/episodes/internal/**` still carry ticket comments (`ISSUE-061`, `P8`) against the no-comments rule. | [episodes.md](modules/episodes.md) |
| 6 | Confirmed: one feature can satisfy several transitions; `Version` is bumped per transition but only the last is published, so the first published version can be 2. `situations/UBIQUITOUS_LANGUAGE.md` says every change publishes a new version. Decide which is right. Pinned by `TestChainedTransitionsOfOneFeaturePublishOnlyTheFinalVersion`. | [situations.md](modules/situations.md) |
| 6 | `RecordCostRejectionReason` and `RecordSchedulerExpiryReason` append a reason but leave the outcome `admitted`. The cognition language says reopening an occurrence needs a cooldown, but the engine never reopens one. | [cognition.md](modules/cognition.md) |
| 9 | Security: `wire.SignedPayload` decodes the evidence token signature with non-strict base64, so three other final characters verify as the same token (the MAC still holds; authority does not widen). Fix: `base64.RawURLEncoding.Strict()`. | [evidence.md](modules/evidence.md) |
| 9 | A provider result after the call deadline returns `DeadlineExceeded` but leaves the reservation `running` until lease reclaim. Compensating intents bypass `AllowedIntentTypes` (only the reconsider flag and catalog membership gate them). Confirm both. | [evidence.md](modules/evidence.md), [decisions.md](modules/decisions.md) |

