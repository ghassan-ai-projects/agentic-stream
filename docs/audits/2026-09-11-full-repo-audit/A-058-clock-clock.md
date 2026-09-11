# A-058 · `internal/clock/clock.go`

LOC: 164 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every timer due at the advance target fires during that `Advance` call, in due-time order (ties by scheduling order).
- `sortTimers` establishes a total order, or the scheduling loop re-establishes order after every pop.
- `Timer.Reset` is either implemented with standard-library semantics (re-inserting a fired timer) or absent from the interface.
- Tests cover multiple timers with interleaved due times and partial advances.

## Findings
- **HIGH F1. `Virtual.Advance` skips due timers behind a not-yet-due head** — `internal/clock/clock.go:107-121,124-135`. `sortTimers` is not a sort: its swap pass only guarantees the minimum lands at index 0, and `v.modified` makes every subsequent call a no-op. After the first pop, `timers[0]` can be an undued timer that trips the `next.due.After(v.now)` break (line 114) while a due timer sits behind it. Reproduced empirically (test since removed): timers scheduled at +10m, +5m, +7m, then `Advance(7m)` — the 5m timer fires, the 7m timer does not fire during the advance in which it is due; the 10m timer blocks it at the head. The engine's timer-driven detection (`internal/engine/engine_timers.go:143` → heartbeat `ApplyTimer`) inherits delayed or misordered timer delivery, breaking the virtual clock's determinism contract. Fix: keep `timers` sorted with `slices.SortStableFunc` on (due, creation sequence) after every mutation, or drop the `modified` short-circuit and sort each loop iteration.
- **MED F2. `Timer.Reset` is dead in production and broken for fired virtual timers** — `internal/clock/clock.go:36-37,156-164`. No caller in the repo invokes `Reset` (physical or virtual). Worse, `virtualTimer.Reset` on an already-fired timer sets `active = true` and `modified = true`, but the timer was removed from `v.timers` when it fired (line 116), so it can never fire again while `Stop()` reports it active. Implement re-insertion on `Reset`, or remove `Reset` from the `Timer` interface until needed.
- **LOW F3. Doc contradicts implementation on concurrency** — `internal/clock/clock.go:9-11`. The `Clock` doc says the virtual clock "is not safe for concurrent use without external synchronization," yet every method locks `v.mu` and is safe. Fix the doc (or the redundant-looking API contract) so consumers do not serialize needlessly.
- **P6.** `clock_test.go` covers only single-timer advance, stop, and basic `Now` — precisely the scenarios where F1 is invisible. Add the 3-timer partial-advance case as a regression test.

## Resolution (2026-09-11) — FIXED
- **F1** fixed: `Advance` now sorts `v.timers` by `(due, seq)` with `slices.SortStableFunc` once per call (firing only pops the front and adds nothing, so the remainder stays ordered). The broken `modified`/partial-selection `sortTimers` is deleted. A monotonic `seq` gives a total order with stable scheduling-order tie-break.
- **F2** fixed by removal: `Reset` had zero callers repo-wide, so it is removed from the `Timer` interface and both implementations (P3). Full build passes without it.
- **F3** fixed: `Clock` doc no longer claims the virtual clock is concurrency-unsafe; it serializes under `v.mu`.
- **P6** fixed: added `TestVirtualAdvanceFiresDueTimersBehindLaterHead` (the +10m/+5m/+7m, `Advance(7m)` regression case plus a later advance for the +10m timer).

## Checked, not an issue
- P1: no errors to swallow; mutex discipline correct (`Stop`/`Reset` lock the owning clock; buffered timer channel prevents Advance deadlock).
- P4: clean abstraction, dependency-free (stdlib only); `Quality` (line 22) is consumed by engine telemetry and its `*Virtual` type-assertion is safe for the two shipped implementations.
- P7: timer delivery records the due time, not wall time (`next.c <- next.due`, line 119); physical clock returns UTC (line 47), matching replay semantics.
- P5: exported symbols documented.
