# A-072 · `internal/operators/operators_test.go`

LOC: 677 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every aggregate kind implemented in operators.go has a test asserting its computed value.
- No test asserts only that a feature was emitted without checking the value.
- Envelope construction goes through the in-file helper everywhere.
- Assertions state exact expected completeness values, not just "not X".

## Findings
- **[HIGH] F1. TestSlope never checks the slope value** — `internal/operators/operators_test.go:59-92`. Inputs are 0, 1, 2 at one-hour steps, so the slope feature must be 1.0 celsius/hour, but the test only asserts `len(fs) != 0` at `i == 2` (`:88-90`). A slope implementation returning the mean, zero, or an inverted sign passes. Assert `fs[0].Value == 1.0` (and ideally the output name and unit).
- **[MED] F2. Half the aggregate surface and window/emit policies untested** — `internal/operators/operators_test.go` (whole file) vs `internal/operators/operators.go:600-632` (`rms`, `count`, `sum`, `min` never exercised; only mean/max/latest are), `operators.go:81` (`tumbling` windows untested; only `sliding`), `operators.go:283-286` (`early_and_close`/`on_close` emit policies untested; only `on_update`). This is the package's only test file, so these paths have zero coverage.
- **[MED] F3. Envelope literals copy-pasted despite the in-file helper** — `internal/operators/operators_test.go:27-39,70-82,109-121` hand-roll the same 13-line `contractsv1.Envelope` while `testOperatorEnvelope` exists at `:621-635`. Route the three tests through the helper.
- **[LOW] F4. TestAggregateMean asserts nothing before the window emits** — `:26-56`. For `i < 2` the emitted features are ignored; there is no negative assertion that no feature is emitted before the first slide boundary, and the value check is gated on the magic `i == 2` index instead of a final post-loop assertion.
- **[LOW] F5. "Not corrected" is weaker than the documented invariant** — `:156-158`. The in-order feature is asserted as merely `!= CompletenessCorrected`; assert the exact expected completeness so a wrongly-labeled "uncertain"/empty value cannot pass.
- **[LOW] F6. Inconsistent `t.Parallel()`** — applied at `:438,474,505,540` only; the other ten tests are equally pure and in-memory. Either apply uniformly or drop.

## Checked, not an issue
- T1: quality-gate, event-time-ordering, and boot-boundary tables assert exact values, input-ID lists, state keys, and window-start times; no tautologies.
- T2: fully deterministic — fixed base dates, no sleeps, no network, no shared state between tests; `t.Parallel()` usage is safe.
- T3: the three table-driven tests use `t.Run()` correctly with independent subtests; `testOperatorEnvelope` and spec-builder helpers are reused where present.
- T4: late-event correction, missing-heartbeat detection-time semantics, invalid-heartbeat liveness, boot-scoped identity, and stale-boot isolation are behaviorally covered with strong assertions.
