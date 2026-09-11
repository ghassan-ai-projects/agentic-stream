# A-022 · `internal/telemetry/runtime.go`

LOC: 338 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Metric memory is bounded for a long-lived process, and a metrics scrape costs bounded work regardless of uptime.
- Every declared field is read.

## Findings
- **[MED] F1. Duration histogram grows without bound and rescrapes cost O(n log n)** — `internal/telemetry/runtime.go:58-59,248-272`. `ObserveDuration` appends one sample per episode decision (`internal/episodes/executor.go:335`) and nothing ever trims `durations`; `Percentile` copies and sorts the entire slice on every call, and `LatencySnapshot` calls it three times per scrape (318-324). In a long-running serve process this is unbounded memory growth plus quadratically-degrading scrape cost under a metrics poller — an infrastructure-plane defect, not a hypothetical. Fix: replace with a fixed-size ring/reservoir or pre-bucketed histogram (fixed bucket edges keep output deterministic too).
- **[MED] F2. `Runtime.started` is written and never read** — `internal/telemetry/runtime.go:22,76`. Set by both constructors, consumed by nothing (grep across repo: declaration and assignment only). It also forces the awkward `NewRuntime(now time.Time)` signature whose argument is otherwise ignored. Fix: delete the field and the parameter, or expose process uptime in `Snapshot`.
- **[LOW] F3. `Percentile` panics on out-of-range input** — `internal/telemetry/runtime.go:270-271`. For `p > 100` (or negative `p`) with ≥3 samples the computed index escapes the slice bounds. Exported method; clamp or reject.
- **[LOW] F4. `sort.Slice` instead of `slices.SortFunc`** — `internal/telemetry/runtime.go:269`. P5 names the stdlib `slices` package.

## Checked, not an issue
- P1: counters are `atomic.Uint64` — no races; nil-receiver guards on every observer; `latencyNanos` clamps negative durations (309-314).
- P2: no tenant, entity, or event-id labels anywhere in the metric set (21, 275-305) — the counter surface cannot leak evidence; the `/metrics` handler writes only names and numbers (328-338).
- P3: the 20+ `ObserveX` methods are all called from runtime/pipeline/action code (verified by grep for the freshest additions: `staleRebinds`, `rebindFailures`, `safeStop*`, `liveLines*`); no speculative abstraction beyond F2.
- P4: provider-neutral by design (documented at 1-3); tracer injection keeps stream logic independent of global OTEL setup (69-77); low-cardinality names follow one convention.
- P5: exported symbols documented; counter set stable in `Snapshot`.
- P6: `runtime_test.go` and `p8_latency_test.go` cover counters and latency percentiles.
- P7: counters monotonic; percentiles computed from recorded samples with no wall-clock dependence beyond the recorded durations themselves.
