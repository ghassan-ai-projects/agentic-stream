# telemetry

Status: done
Round: 3

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/telemetry` | 66.7% | 100.0% | 1.3 s | 1.2 s | 3 / 0 | 2 / 0 |
| `internal/telemetry/internal/domain` | 93.1% | 100.0% | 1.2 s | 1.1 s | 4 / 0 | 8 / 27 |
| `internal/telemetry/internal/transport` | 71.2% | 94.9% | 1.3 s | 1.2 s | 3 / 0 | 9 / 11 |

Tests are top-level / passing subtests. Hygiene lint 0 (was 7 findings), repository lint 0 (also at `dupl` 60),
`-race -shuffle=on -count=3` passes. No test makes a network call beyond one loopback `httptest.Server`.

## Findings and changes

### Removed
- `TestRuntimeCountersAndMetricsAreLowCardinality` (facade): re-tested the domain counters and the transport handler; replaced by the domain table, `transport/metrics_test.go` and a facade wiring test.
- `TestDurableW3CContextBecomesOpenTelemetryLink` and `TestOTLPHTTPProviderExportsSpans` (facade): moved to `transport/tracing_test.go`, where `AddLinkFromW3C` and the exporter live (T1, T2).
- `TestEveryObservationIncrementsItsCounter` and `TestLatencyPercentilesAndStaleRejectionsAreMeasured`: replaced by stricter tests below. The `P8` and `ISSUE-061` comments went with them (T3, no comments inside modules).

### Renamed or moved
- `transport/otel_test.go` → `transport/tracing_test.go`.

### Improved
- T4: `TestEachObservationIncrementsOnlyItsOwnCounter` asserts, for each of the 18 observers, that exactly its own counter moved (the old test compared a sum). `TestPipelineReportAddsEachFieldToItsOwnCounter` covers negative and zero counts. The metrics handler test requires the served text to equal the runtime's views, with content type and no labels.
- T6: all tests parallel except the two that replace the process tracer provider (`//nolint:paralleltest` with that reason; each restores the previous provider and propagator). The OTLP test uses `NewTracerProvider`, not `Configure`, so it is parallel.
- Percentile cases are one table, including the empty and unordered inputs.

### Added
- Domain: `TestSnapshotNamesEveryCounterExactlyOnce`, `TestNilRuntimeIsInert`, `TestRuntimeSpansAreRecordedOnlyWhenATracerIsConfigured`, `TestSnapshotsAreIndependentCopies`, `TestLatencySnapshotReportsNanosecondsAndClampsNegativeDurations`.
- Transport: service name and its default, unparsable endpoint, W3C link validity (tracestate kept, all-zero trace id, garbage, nil span), `RecordError` on nil error and nil span, and the OTLP/HTTP export (path `/v1/traces`, `application/x-protobuf`, payload carries service and span names) against a loopback `httptest.Server`.
- Facade: `TestMetricsHandlerServesTheRuntimeCountersWithoutLabels`, `TestSpansStartedThroughTheFacadeReachTheConfiguredProvider` (`Configure`, `StartSpan`, `AddLinkFromW3C`, `RecordError`, `NewRuntime(...).StartSpan` parent link; in-memory exporter).

### Speed
- Nothing was slow (all packages about 1.1-1.3 s, which is race-binary start-up).

## Production code touched
- none.

## Invariants proven here
- Telemetry takes no part in verdicts and carries no tenant or event labels: `TestMetricsHandlerServesEveryCounterAndLatencyAsUnlabeledPrometheusText`.

## Open items
- `RecordError` records the error text in an `exception` event although its comment promises no payload leakage; pinned by `TestRecordErrorMarksTheSpanFailedWithAFixedDescription`, product decision needed.
- The OTLP export test uses loopback HTTP; an in-memory exporter cannot exercise `otlpBatcher`.
