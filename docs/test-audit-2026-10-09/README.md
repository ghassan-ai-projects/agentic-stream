# Test audit (2026-10-09)

Status: closed after round 17 (rounds 0–10 merged in PR #50, 11–13 in PR #55, 14–17 on branch `test-audit-rounds-14-17`). See [SUMMARY.md](SUMMARY.md).

A full audit of the repository's tests: organize them so the repository is easy
to read, make them faster, raise coverage where it proves behavior, and remove
what proves nothing. Every test ends at the [test bar](TEST_BAR.md).

| Document | Content |
| --- | --- |
| [TEST_BAR.md](TEST_BAR.md) | The bar, rules T1–T12, and the slow-test register |
| [BASELINE.md](BASELINE.md) | Coverage, timing and lint numbers before the audit |
| [SUMMARY.md](SUMMARY.md) | Final numbers against the baseline, gates added, open items |
| [WORKER_BRIEF.md](WORKER_BRIEF.md) | What a round's worker does and how it reports |
| [ROUND_PROMPT.md](ROUND_PROMPT.md) | The shared prompt given to every round worker |
| [SLOW_TESTS.md](SLOW_TESTS.md) | Static investigation of the slowest tests, for rounds 14 and 16 |
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

## Closed

The audit is closed; the final numbers and the open items are in
[SUMMARY.md](SUMMARY.md). Nothing is stashed or pending. New work that touches a
test follows the bar in [TEST_BAR.md](TEST_BAR.md), enforced by `make lint`,
`make coverage-check` and `internal/architecture`.

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
| 8 | `episodeledger`, `approvalledger` | done | every package ≥ 85% (episodeledger facade 64.5 → 100, store 67.5 → 86.1) | no test above 0.6 s; attempt state machine checked over all 81 pairs | 5f8d47a7 |
| 9 | `evidence`, `decisions` | done | every package ≥ 87% (evidence app 81.5 → 98.1, decisions domain 84.5 → 95.3) | slowest test 0.32 s | a0e50cc1 |
| 10 | `executor/*`, `worker`, `testsupport/*` | done | every package ≥ 78% (remote domain 69.9 → 97.8, conformance 66.7 → 94.9, worker transport 69.4 → 78.8) | slowest test 1.41 s → 0.34 s; conformance no longer re-execs a process | 995b3c40 |
| 11 | `policy`, `authority` | done | every package ≥ 81% (policy facade 85.2 → 100, policy store 80.2 → 89.9) | policy app 8.5 s → 3.4 s (serial tests, per-test pipeline, 12 autocommits, sleep) | 046acfd7 |
| 12 | `actions`, `actionport`, `watch`, `notify` | done | every package ≥ 77% (notify store 68.0 → 93.0, actionport domain 80 → 100, actions app 81.9 → 89.4) | no test above 1 s; crash-between-effect-and-outcome proven | 8a4cd57c |
| 13 | `device`, `control` | done | every package ≥ 79% (controltest 66.7 → 100, device 93.3 → 100, control store 70.8 → 79.2) | slowest test 0.65 s; no sleeps | 46092904 |
| 14 | `replay`, `runartifact` | done | every package ≥ 75% (replay 60.0 → 100, replay app 68.1 → 86.2, transport 67.9 → 83.3, runartifact store 71.3 → 84.0) | `replay` 31–36 s → 5.4–6.5 s; slowest test 32.6 s → 1.5 s alone | c2067055 |
| 15 | `runtime`, `api` | done | every package ≥ 79% (runtime store 68.2 → 83.5, api transport 85.0 → 96.8, runtime app 80.3 → 89.8) | runtime app 6.0 s → 3.1 s; policy approval test 1.24 s → 0.26 s; sleeps 4 → 0 | f592033b |
| 16 | `cmd/agentic-stream` | done | 74.3 → 83.1% | package 51–58 s → 9.6–11 s; experiment tests 33–39 s → 5–8 s | 6df1a2bb |
| 17 | gates: linters, coverage floor, invariant map, docs | done | floor 60 → 70%, total 75.6 → 85.6% | gates in `make lint`, `make coverage-check` and `internal/architecture` | see git log |

## Findings for the owner

Behavior questions a round found but did not change, because the decision is
not a test decision.

| Round | Finding | Where |
| --- | --- | --- |
| 3 | `telemetry.RecordError` promises no payload leakage, but `span.RecordError(err)` stores the error text in an `exception` span event; only the status description is fixed. Current behavior is pinned by a test. | [telemetry.md](modules/telemetry.md) |
| 3 | `storage.OpenFresh` could copy a migrated template instead of migrating: saves about 1.3 s per isolated replay under `-race`, about 0.06 s in production. Not done; revisit after the replay round. | [storage.md](modules/storage.md) |
| 2 | `interlock.Assert` documents that a missing row fails closed wrapping `ErrTripped`; it wraps `sql.ErrNoRows` instead (still fails closed). Fix the comment or the code. | [interlock.md](modules/interlock.md) |
| 2 | canonicaljson accepts a native `float64` above 2^53 but refuses the same value as a raw JSON integer; both pinned, needs a design decision. `containsSurrogate` has an unreachable branch. | [canonicaljson.md](modules/canonicaljson.md) |
| 4 | Bug: `cleanLiveListener.closeAndRemove` (`internal/ingress/internal/transport/listener.go`) should remove the socket file only if it is still the listener's, but `net.UnixListener.Close` unlinks the path first, so a replacing file is deleted and `removeSocketFile` is unreachable. Fix: `SetUnlinkOnClose(false)` in `listenSocket`, as its own change. Tracked in [#52](https://github.com/ghassan-ai-projects/agentic-stream/issues/52). | [ingress.md](modules/ingress.md) |
| 5 | A single event can chain several Situation transitions and publish only the last version, so published versions can start at 2. Confirm this is intended (situations round checks it). `RunGlobal` counts an event already in the inbox as processed. | [engine.md](modules/engine.md) |
| 7 | An executor outcome with a non-terminal status and no decision (for example `running`) makes `RunOnce` return an error and leaves the attempt `running`. With cost control on, `Settle` for an episode that never reserved errors after the attempt ran. | [episodes.md](modules/episodes.md) |
| 7 | Production files under `internal/episodes/internal/**` still carry ticket comments (`ISSUE-061`, `P8`) against the no-comments rule. | [episodes.md](modules/episodes.md) |
| 6 | Confirmed: one feature can satisfy several transitions; `Version` is bumped per transition but only the last is published, so the first published version can be 2. `situations/UBIQUITOUS_LANGUAGE.md` says every change publishes a new version. Decide which is right. Pinned by `TestChainedTransitionsOfOneFeaturePublishOnlyTheFinalVersion`. | [situations.md](modules/situations.md) |
| 6 | `RecordCostRejectionReason` and `RecordSchedulerExpiryReason` append a reason but leave the outcome `admitted`. The cognition language says reopening an occurrence needs a cooldown, but the engine never reopens one. | [cognition.md](modules/cognition.md) |
| 9 | Security: `wire.SignedPayload` decodes the evidence token signature with non-strict base64, so three other final characters verify as the same token (the MAC still holds; authority does not widen). Fix: `base64.RawURLEncoding.Strict()`. Tracked in [#51](https://github.com/ghassan-ai-projects/agentic-stream/issues/51). | [evidence.md](modules/evidence.md) |
| 9 | A provider result after the call deadline returns `DeadlineExceeded` but leaves the reservation `running` until lease reclaim. Compensating intents bypass `AllowedIntentTypes` (only the reconsider flag and catalog membership gate them). Confirm both. | [evidence.md](modules/evidence.md), [decisions.md](modules/decisions.md) |
| 8 | Invariant 10 gap: `approvalledger.ExpireIntent` writes no `decided_at` or `reason`, so an approval expired this way is not explainable from its row. `Resolve` accepts `pending` and stamps `decided_at` and the approver. Tracked in [#53](https://github.com/ghassan-ai-projects/agentic-stream/issues/53). | [approvalledger.md](modules/approvalledger.md) |
| 8 | Episode lifecycle writers (`Conclude`, `Abandon`, `RetainForRetry`, ...) have no lifecycle guard in their SQL: `Abandon` after `Conclude` overwrites the outcome, `RetainForRetry` revives a concluded episode. They rely on callers. Tracked in [#54](https://github.com/ghassan-ai-projects/agentic-stream/issues/54). | [episodeledger.md](modules/episodeledger.md) |
| 10 | The fixture executor panics on a nil request and ignores its context, while native and remote return an error. The native retry backoff is a fixed 10 ms real timer. | [executor-fixture.md](modules/executor-fixture.md), [executor-native.md](modules/executor-native.md) |
| 13 | Production error text in `device` (`gateway_effector.go`, `session*.go`) still says "serial", a word the device language retires. The optional cross-repository catalog check skips when `REAL_WORLD_SENSOR_ROOT` is unset (it used to pass silently). | [device.md](modules/device.md) |
| 12 | Possible bugs in `actions`: a command refused before the effector is stored `failed` with its verification left `awaiting` forever; `MarkCommandDispatching` also moves `failed` commands to `dispatching`; the lease-expiry metric counts one abandoned lease twice. Not pinned by tests. Tracked in [#58](https://github.com/ghassan-ai-projects/agentic-stream/issues/58). | [actions.md](modules/actions.md) |
| 12 | `watch/internal/app/install.go` `assertGuards` ignores its `tenantID` and `target` parameters. The watch busy-retry test releases the SQLite lock from a 50 ms real timer (the retry backoff is a real timer inside storage). | [watch.md](modules/watch.md) |
| 11 | A stale approval ends `denied` plus `withdrawn_at` because the ledger has no `withdrawn` status (pinned only). `authority/internal/store/reader.go` has its own `nullable[T]` where AGENTS.md names `storage.NullIfEmpty`. `TestACommittedApprovalDispatchesWithoutNewSensorInput` is bound to a fixed 1 s outbox ticker in `Pipeline.Start` (round 15). | [policy.md](modules/policy.md), [authority.md](modules/authority.md) |
| 14 | A thermal-trace replay costs 1.2 s under `-race` (70 ms without), from goroutine wake-ups in `database/sql` and the SQLite driver inside `internal/engine` and `internal/storage`. `internal/replay/internal/transport` failed once in about 12 early timing runs (not captured, not reproduced in 50+ later runs); if it returns, look at `TestShadowWorkerNamesItselfInTheErrorOfAFailedExecution` first. | [replay.md](modules/replay.md) |
| 16 | `run-live` validates `--trace-format` only after opening the database (a behavior change to move it earlier). The experiment end-to-end tests now run the spec with 2 s debounce and cooldown (test copy only; `examples/` untouched) and wait on durable rows instead of sleeping. | [cmd-agentic-stream.md](modules/cmd-agentic-stream.md) |
| 15 | Bug: `notify` reads the tenant bounds (`TenantBounds`) and the page rows (`ReadPage`) in separate autocommit queries. A subscriber reading an empty tenant stream while its first event is appended gets a spurious `cursor_expired`, and a lagging subscriber can miss the lag (a flake in the SSE test under load). Read both in one snapshot, with a regression test in `notify`. Tracked in [#56](https://github.com/ghassan-ai-projects/agentic-stream/issues/56). | [api.md](modules/api.md) |
| 15 | `RunLiveSocket` returns an error when the context is cancelled during a commit: `database/sql` reports `ErrTxDone`, which `domain.NormalLiveSocketShutdown` does not treat as cancellation, so a clean shutdown looks like a failure (`AdvanceEvery` and `RunEpisodesEvery` already ignore errors after cancel). Two fix options in the report. Tracked in [#57](https://github.com/ghassan-ai-projects/agentic-stream/issues/57). | [runtime.md](modules/runtime.md) |
| 15 | Quarantined and expired scheduler items flood test output with slog lines; `AdvanceEvery` at 1 ms starves ingestion under `-race`; no test uses a `GatewayEffector`. | [runtime.md](modules/runtime.md) |

