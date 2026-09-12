# A-005 · `internal/operators/operators.go`

LOC: 862 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Every window emit mode declared in `internal/spec/schema.json` (`on_update`, `on_close`, `early_and_close`) produces features; the schema default `on_close` is implemented.
- Window `slide` (and `count`, `halfLife`) configuration is either honored by the window logic or rejected at compile time; no silently ignored knobs.
- No event-type-family or unit-string branches in Go code; domain-specific behavior is data-driven per AGENTS.md.
- Only one operator entry point exists in production; any secondary entry point is removed or tested.
- Tenant attribution for timer-emitted features comes from the same source as event-emitted features.
- Timer features carry the partition the timer belongs to.
- No dead branches, no redundant state writes, unused exported symbols removed.
- Package tests cover all reachable emit modes.

## Findings
- **HIGH F1. `on_close` windows never emit any feature** — `internal/operators/operators.go:286-287`. `emit: on_close` sets `emit = false` and the only other emission path, `ApplyTimer` (line 697), skips every kind except `missing_heartbeat`. There is no window-close code path anywhere (`grep -rn on_close internal/` hits only this switch). Consequence: any aggregate/slope on an `on_close` window silently produces nothing — this is also the runtime default (`newWindowConfig`, lines 76-78) and the schema default. The flagship example routes `vibration_slope` through the `on_close` window `trend_6h` (`docs/design/examples/predictive-maintenance.situation.yaml:67-70,92-98`), so its 6-hour trend output is never materialized; `internal/replay/replay_test.go:513` works around the bug by rewriting the spec to `emit: on_update`. Fix: implement watermark-driven window close (emit on eviction boundary or via `ApplyTimer`), or reject `on_close` at compile time until implemented.
- **MED F2. `slide` is parsed and validated but never used** — `internal/operators/operators.go:92-97`. `windowConfig.slide` is written in `newWindowConfig` and never read anywhere; sliding windows emit on every event regardless of the declared slide interval. A spec author setting `slide: 15m` gets per-event emission with no error. Same treatment needed as F1: honor it or reject it.
- **MED F3. Per-domain branches in Go code** — `internal/operators/operators.go:468-477,661-664`. `numericObservationQualityValid` branches on `strings.HasPrefix(env.Type, "zone.")` — the "thermal wire family" is hard-coded, exactly the per-domain Go branch AGENTS.md forbids ("domains are data"); the quality contract should ride the event schema registry (`internal/eventschema/registry_data.json`). Likewise `linearSlope` infers rate semantics from `strings.Contains(unit, "per_second") || strings.Contains(unit, "_per_s")` — unit-to-scale mapping encoded as string matching instead of spec/schema data.
- **MED F4. Dead production entry point and dead interface** — `internal/operators/operators.go:122-125`, `internal/operators/types.go:39-44`. `ApplyEvent` is called only by `operators_test.go` (engine calls `ApplyEventAt` directly, `internal/engine/engine_apply.go:41`), and the `operators.Runtime` interface has zero references repo-wide. `ApplyEvent` also bakes in `env.IngestedAt` as processing time — a second, divergent processing-time semantic. Delete the interface and `ApplyEvent`, and use `ApplyEventAt` in tests.
- **MED F5. Timer features hard-code tenant and partition** — `internal/operators/operators.go:744,749`. `applyHeartbeatTimer` sets `TenantID: contractsv1.TenantID` (the constant `"default"`, `internal/contractsv1/ids.go:31`) and `PartitionID: 0`, while the event path uses `env.TenantID` and `env.PartitionID(0)` (lines 298, 381-386). Timer-emitted features misattribute tenant and partition for any non-default tenant or non-zero partition. Pass the runtime partition into `ApplyTimer` and derive tenant from state.
- **LOW F6. Redundant `setBlob` and dead nil-check** — `internal/operators/operators.go:191,212-222,369`. `getBlob` already inserts a missing blob into the map (lines 204-208), so the following `setBlob` with the same pointer is a no-op duplicated map walk; delete `setBlob`. At line 369, `hs.LastEventTime != nil` is always true — it was assigned from `env.EventTime` three lines above (line 354).
- **LOW F7. `qualityAdmitsBoot` resolves policy by first-operator-kind** — `internal/operators/operators.go:479-489`. The event-level gate applies whichever quality contract belongs to the first operator instance in declaration order; an input feeding both an aggregate and a heartbeat gets the aggregate's stricter gate for both. Deterministic today, but fragile and mostly redundant with the per-operator checks in `extractValue`/`applyHeartbeatOperator`; make the gate explicit per operator kind.
- **LOW F8. stdlib `sort` instead of `slices`, discarded ctx** — `internal/operators/operators.go:261,688,720,677`. Repo style prefers `slices.SortStableFunc`/`slices.Sort`; `ApplyTimer` takes `context.Context` and immediately discards it (`_ = ctx`, line 678) — drop the parameter if it is never honored.
- **P6.** No test exercises `on_close`, `slide`, or window close behavior — the exact gap hiding F1/F2.

## Checked, not an issue
- P1: errors wrapped with `%w` throughout; none swallowed; maps accessed nil-safely; state serial per partition, no races.
- P2: watermark-based eviction/window bounds are explicit (`cutoff := watermark.Add(-size)`, line 251); producer timestamps never drive scheduling (`ApplyEventAt` doc, line 128); boot fencing fails closed at the bounded-history limit (lines 547-551).
- P7: `ApplyTimer` sorts instance names and state keys before iteration (lines 688, 720); samples sorted by (event time, event ID) for deterministic `latest` (lines 261-266, 632-636); aggregates are order-independent or order-pinned.
- P5: exported symbols documented; `log/slog` not needed at this layer.

## Resolution (isolated implementation, 2026-09-12)

The scoped implementation closes the operator-runtime findings without adding
domain branches or unbounded state:

- **F1/F2/P6:** watermark advancement now closes `on_close` windows at the
  configured tumbling size or sliding hop. `early_and_close` continues to emit
  deterministic provisional updates while advancing the same close horizon.
  The runtime validates emit modes, positive sizes, and sliding hops; invalid
  `count`, `decay`, zero/oversized hops, and tumbling `slide` configurations
  fail closed before the runtime is constructed. Tests cover the explicit
  `on_close` path, hop boundaries, and all reachable emit modes.
- **F3:** provenance admission is driven by the registered input schema's
  fields rather than an event-type prefix. Slope calculation has one documented
  per-hour runtime contract and no unit-name scaling branch.
- **F4:** the unused `ApplyEvent` production wrapper and the unreferenced
  `operators.Runtime` interface were removed; tests use `ApplyEventAt` so
  processing time remains explicit and there is one production entry point.
- **F5:** timer output no longer invents the default tenant or partition. Direct
  timer callers provide `TimerIdentity`; the existing engine path supplies its
  authoritative tenant and partition during persistence enrichment. A test
  proves non-default tenant and non-zero partition attribution.
- **F6/F8:** the redundant state write was removed, deterministic ordering uses
  `slices`, and `ApplyTimer` now honors cancellation before and during timer
  evaluation.
- **F7:** quality and boot admission is evaluated per operator, so one
  operator's provenance contract cannot suppress a different operator sharing
  its input.

Focused evidence:

```text
go test -race -count=1 ./internal/operators   PASS
go vet ./internal/operators                   PASS
go test ./...                                  PASS
git diff --check                               PASS
```
