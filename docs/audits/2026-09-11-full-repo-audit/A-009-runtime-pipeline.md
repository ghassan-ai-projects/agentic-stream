# A-009 · `internal/runtime/pipeline.go`

LOC: 646 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Every event appended in a batch is watch-fired or the batch errors; no silent truncation cap.
- Event-log reads go through the eventlog package that owns the table.
- Constructor collaborators are reviewable without parsing 200-character chained one-liners.

## Findings
- **MED F1. Watch firing silently truncated at 100,000 events per batch** — `internal/runtime/pipeline.go:427-438`. `fireRecentWatches` reads events after `before` with `Limit: 100000` and never checks whether the read was truncated; a simulator/normalized batch appending more than the cap leaves the excess events never passed to `watch.FireEvent`, and the batch still reports success — a silent completeness gap in the trigger path. Fix: paginate until the read returns fewer than the limit, or return an error when the cap is hit.
- **LOW F2. Raw SQL reaches into the event_log table behind eventlog's back** — `internal/runtime/pipeline.go:419-425`. `currentEventPosition` queries `SELECT COALESCE(MAX(position),0) FROM event_log ...` directly on `storage.DB` even though the pipeline already holds `p.log *eventlog.EventLog`, which owns that table's schema and invariants. Fix: expose a position/append-count reader on EventLog and call it.
- **LOW F3. NewPipeline builds collaborators as unreadable fluent one-liners** — `internal/runtime/pipeline.go:118,123,141-143`. Four-plus-option chains (`NewRunnerWithEpoch(...).WithAssembler(...).WithCostControl(...).WithEpochControl(...)...`) hide which options are set and make omissions (e.g. the run-live route's missing `EpochControl`) hard to spot in review. Fix: assign each collaborator to a named local with one option per line.

## Resolution

`fireRecentWatches` now reads the event log in ordered pages of 1,000 records,
advancing from the last delivered position until a short page is returned. It
rejects a non-advancing page, so an event beyond the former single-read cap
cannot be silently skipped. `TestFireRecentWatchesPaginatesPastFullPage` places
the only matching event after a full page and verifies that it fires.

`eventlog.EventLog.CurrentPosition` now owns the tenant-scoped position query;
the runtime calls that API instead of reaching into `event_log` directly.
`TestCurrentPositionIsTenantScoped` covers empty tenants, interleaved tenants,
and position advancement.

`NewPipeline` now names the composite effector, assembler, runner, policy
gateway, and dispatcher, applying each option as a separate statement before
the pipeline is assembled. This preserves the existing collaborator wiring
while making every lifecycle, epoch, cost, shadow, telemetry, and interlock
option reviewable.

Focused proof: the A-009 runtime and eventlog tests pass, including the race
test run; `go vet ./...` and `git diff --check` pass.

## Checked, not an issue
- P1: all errors wrapped `%w`; watch failure is recorded under mutex and surfaced at the next batch (365-367); `Close` cancels and joins the maintenance goroutine (192-206); no data races (watchMu guards all watch fields).
- P2: fixture executors rejected and quarantined on production routes with the item coalesced, never left blocking the queue (486-532); policy epoch stamped exactly once (493); ownership re-asserted inside every mutating transaction.
- P3: `assemblePending`/`evaluatePendingIntents` loops provably make progress (skip paths mark the item coalesced before `continue`); no unused exported symbols (`ErrFixtureRejected`, `PipelineReport`, `Start`, `Close`, `RunJSONL`, `RunLiveSocket`, `RunSimulatorJSONL` all referenced).
- P4: cmd-agnostic; effectors, policy, dispatcher composed from their own packages; no domain branches.
- P6: pipeline_test.go, pipeline_e2e_test.go, pipeline_live_socket_test.go, p8_mode_control_test.go, reconsideration_test.go, costcontrol_test.go, recovery_test.go cover admission, drain, fixture rejection, and live-socket sinks.
- P7: `RunJSONL`, `RunSimulatorJSONL`, and `RunLiveSocket` all converge on `runAfterIngest`; the replay command reuses the same `ingress` connectors and `engine` (replay.go:489-498) — no divergent second pipeline.
