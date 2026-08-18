# Operational observability

Audience: operators and incident reviewers. Scope: health, metrics, telemetry,
durable notifications, and the limits of current inspection surfaces.

## Health and metrics

Use liveness and readiness for process/service routing, and `/metrics` for
low-cardinality runtime signals. Metrics include pipeline counters and bounded
latency/stale-rejection observations; they are not a durable replacement for
the SQLite ledger.

```bash
curl http://127.0.0.1:8080/health/live
curl http://127.0.0.1:8080/health/ready
curl http://127.0.0.1:8080/metrics
```

## OpenTelemetry

Set one of these endpoints according to deployment convention:

```bash
export AGENTIC_STREAM_OTLP_ENDPOINT='https://collector.example/v1/traces'
```

The runtime falls back to the standard OTEL trace endpoint variables. Export
only the data policy allows; do not include API keys, capability tokens, raw
restricted payloads, or full model prompts in telemetry.

## Durable SSE

```bash
curl -N \
  -H "Authorization: Bearer $AGENTIC_STREAM_SUBSCRIBER_TOKEN" \
  'http://127.0.0.1:8080/v1/events?tenant=default'
```

Persist the numeric SSE `id` and resume with `Last-Event-ID`. A subscriber is
disconnected when its bounded lag is exceeded. Cursor expiry, poison retries,
and audited skips are explicit failure states.

## Explain a decision

Start from the notification or command ID and follow durable links through:

```text
event -> situation_version -> scheduler_item -> episode/attempt
      -> decision -> intent -> policy_audit -> command/outbox -> outcome
```

The public API does not currently expose a general query endpoint or packaged
inspection CLI for every record. Durable rows can be examined with approved,
read-only deployment tooling; quarantine release, approval resolution, and
unknown-outcome reconciliation remain internal operational capabilities.

## Next reads

- [Observability design](../design/observability.md)
- [Notification contract](../contracts/notifications.md)
- [Recovery](recovery.md)
