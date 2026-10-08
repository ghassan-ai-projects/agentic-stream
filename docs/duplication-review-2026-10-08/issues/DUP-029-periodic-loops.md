# DUP-029: Periodic loop and shutdown-predicate copies across runtime, episodes, cmd and api

- Status: open
- Severity: low
- Verdict (finders): REAL
- Themes: mechanisms
- Wave: not scheduled
- Finder sources: M10 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Lowest priority; do last or close as won't-fix if the loops differ in policy more than in shape.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M10: Periodic loop and shutdown predicate copied across runtime, episodes, cmd, api

- Verdict: REAL
- Shared meaning: run a step every interval until the context ends; context cancellation caused by shutdown is not an error.
- Sites (tick loops): runtime/internal/app/pipeline_clock.go:9-26 `AdvanceEvery`; pipeline_episodes.go:19-31 `executeEpisodesOnSchedule` (steps first, then waits); pipeline.go:63-80 `maintainWatches`; service.go:73-89 `heartbeat`; episodes/internal/app/runner_failure.go:24-38 `watchSupersession`; cmd/agentic-stream/live.go:184-199 `pollTrace`; cmd/agentic-stream/serve_source.go:70-95 `waitForLiveSocket`/`awaitSocketPoll`; api/internal/transport/sse.go:143-165.
- Sites (normal-shutdown predicate): runtime/internal/domain/lifecycle.go:19-21 `NormalLiveSocketShutdown(parentErr, err)` = `parentErr != nil && (Canceled || DeadlineExceeded)`; ingress/internal/transport/server.go:107-109 `normalShutdown(ctx, err)` = `ctx.Err() != nil && (Canceled || DeadlineExceeded)`; cmd serve_source.go:65 and live.go:189 test only `Canceled`.
- How they differ: step-first vs wait-first; error policy (return error unless `ctx.Err() != nil`, record and stop, mark not-ready and stop); the shutdown predicate takes a parent error in one place, the context in the other, and cmd ignores `DeadlineExceeded`.
- Risk if left: a shutdown race fix (e.g. ignoring an error that arrives after cancel) has to be applied in 8 loops; behaviour for deadline errors during shutdown differs by command.
- Proposed canonical owner: `internal/sources` (time/clock leaf, already imported by runtime app, episodes app, cmd) for `Every(ctx, interval, step func(context.Context) error) error` with the "ignore error once ctx is done" rule; the shutdown predicate as `sources.Shutdown(ctx, err)`. Edges: `internal/ingress/internal/transport -> internal/sources`, `internal/api/internal/transport -> internal/sources` (new). Lower priority than C1-C8.
- Proposed fix: replace `AdvanceEvery`, `watchSupersession`, `pollTrace`, `heartbeat` loop skeletons with `Every`; unify the predicate and delete the two copies.
- Behaviour to preserve: first-tick timing of each loop, `AdvanceEvery`/`RunEpisodesEvery` positive-interval errors, heartbeat's `setNotReady` on failure, `watchErr` recording.
- Verification: runtime pipeline tests (`AdvanceEvery`, episodes-beside), cmd live_test. New test: `Every` returns nil on cancel and ignores a step error that coincides with cancel.

## Outcome

Not started.
