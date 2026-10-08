# X08 — The live pipeline advances on a clock

Status: done · Decision: **fix (found by X01)** · Priority: P0 (experiment core) · Size: S

## Finding

With `serve --live-socket`, the pipeline advanced only inside
`ingestLiveEvent`, once per received event. `--poll-interval` drove only the
file-polling source. So on a quiet live socket:

- debounced or cooled-down cognition never started. The X01 test showed a
  scheduler item due at 10:23:35 still pending at 10:24:15. RUNBOOK-G1's "if
  the feed pauses through cooldown, send one additional heartbeat" was a
  workaround for this;
- an approval resolved over HTTP did not dispatch until the next event;
- worst, `missing_heartbeat` could not detect a dead link: a dead link sends no
  events, so the silence timer never ran.

## Decision and reasoning

`advanceBatch` already does all time-driven work (`engine.RunGlobal` fires due
timers, then admission, episodes, policy and dispatch). Run it on the existing
`--poll-interval` while the live socket is open:

- `runtime.Pipeline.Advance` runs the post-ingest stages without new evidence;
  `AdvanceEvery` runs it on a ticker.
- One batch lock serializes ingestion and scheduled advances, so deterministic
  state still changes one batch at a time (invariant 4).
- `serve` starts the clock next to the live socket source; a clock failure
  stops `serve`, like a source failure.

Rejected: a timer per scheduled item. Every kind of time-driven work (timers,
queue `not_before`, approvals, watch expiry) would need its own wake-up, while
the batch already handles all of them in order.

Replay is unaffected: it uses a virtual clock and its own run loop.

## Evidence

- `internal/runtime/internal/app/pipeline_clock_test.go`: a debounced item is
  not admitted before it is due, and is admitted and dispatched by `Advance`
  once due, with no new evidence.
- X01's end-to-end test closes the loop through `serve` without any extra
  event after the feed stops.
