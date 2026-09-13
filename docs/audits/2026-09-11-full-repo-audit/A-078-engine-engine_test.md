# A-078 · `internal/engine/engine_test.go`

LOC: 427 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T2: no sleep-based synchronization; every concurrency wait is bounded and proves the waited-for event happened.
- T5: CompiledSpec fixtures in the package share one builder instead of repeating boilerplate literals.

## Findings
- **[HIGH] F1. Fixed 100 ms sleep decides whether the busy-retry path is exercised at all** — `internal/engine/engine_test.go:129-131`. `TestEngineRetriesApplyAfterTransientSQLiteBusy` holds a `BEGIN IMMEDIATE` lock, sleeps 100 ms, then ROLLBACKs. If the engine goroutine does not reach the contended write within 100 ms (slow/loaded CI), the lock is already released and the run proceeds uncontended — every assertion still passes while the retry logic under test was never executed. Nothing observes that contention occurred (no retry counter, no busy-error hook), so the test gives false confidence precisely when CI is slowest. Fix: keep the lock until the retry is observed (e.g., assert an engine retry/backoff metric or hook fires before ROLLBACK, or poll a bounded deadline with failure diagnostics), and assert the retry happened explicitly.
- **[MED] F2. Goroutine leaks on the timeout path** — `internal/engine/engine_test.go:124-127,143-145`. If the 5 s `time.After` fires, `t.Fatal` returns while `eng.RunGlobal` is still running against a DB that the deferred `db.Close()` will close — a potential panic-on-closed-DB flake in the failure path. Use `t.Context()` cancellation for the goroutine and drain the `done` channel before closing.
- **[LOW] F3. Assertions coupled to exact state-JSON encoding** — `internal/engine/engine_test.go:226,323`. `strings.Contains(stateJSON, `"facts.level":20`)` / `"facts.missing":true` break on any change to map marshaling (spacing, key order via different encoder). Assert on an unmarshaled `map[string]any` instead of substring-matching the serialized blob.
- **[LOW] F4. CompiledSpec boilerplate duplicated across the package** — `internal/engine/engine_test.go:377-396` vs `internal/engine/engine_cognition_test.go:26-64`. Three near-identical CompiledSpec literals (same SchemaVersion/Digest/Windows/Operators skeleton) built inline; extract one `specBuilder` helper with per-test overrides.

## Checked, not an issue
- T1: assertions check observable behavior (processed counts, durable situation versions, timer rows, completeness column values) with diagnosing messages; the restart test's codec-version refusal is asserted with a targeted substring ("requires rebuild") on an error that has one stable cause.
- T2: all other tests use the virtual clock (`clock.NewVirtual` + `Advance`) — deterministic; the only wall-clock use is the flagged sleep.
- T3: shared `appendLevel`/`appendHeartbeat(WithBoot)` helpers use `t.Helper()` and `t.Fatalf`; subtests where used are independent.
- T4: covers checkpoint idempotence (replay processes 0), restart state restore, codec-version refusal, durable timer fire-exactly-once, stale-boot timer retirement, and source-health completeness transitions.
- T5: `appendHeartbeat` delegates to `appendHeartbeatWithBoot` rather than duplicating it.
