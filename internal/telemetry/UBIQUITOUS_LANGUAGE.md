# Telemetry ubiquitous language

| Term | Meaning | Code name | Exposed as |
| --- | --- | --- | --- |
| Runtime counters | Process-local, monotonic, low-cardinality counters for the live pipeline. They carry no tenant, entity or event labels. | `Runtime`, `NewRuntime` | Prometheus text |
| Pipeline report | The small counter projection accepted from the runtime: events ingested and processed, episodes admitted and executed, intents evaluated, commands dispatched. | `PipelineReport` | — |
| Metrics handler | Serves the counters without leaking tenant or event data; mounted by the runtime handler. | `Runtime.Handler` | HTTP `/metrics` |
| Observation | One increment of a named counter for a pipeline, failure, stale-rejection, device or action event. | `Runtime.Observe*` | counter |
| Latency | Observed durations with percentile and snapshot views. | `ObserveDuration`, `Percentile`, `LatencySnapshot` | — |
| Span | An OpenTelemetry span started from the process tracer provider. | `StartSpan` | OTLP/HTTP when an endpoint is set |
| Span link | A causal link from a durable W3C trace context, used for asynchronous work instead of a false parent. | `AddLinkFromW3C` | `traceparent`, `tracestate` |
| Tracer provider | The process tracer, configured once; the caller owns shutdown. | `Configure`, `NewTracerProvider` | — |

Telemetry never decides anything and is excluded from verdicts: it resets on
restart and cannot prove a physical event. Spans never carry request or
evidence payloads (`RecordError`).
