# Operational observability

Use health checks and telemetry to observe the running service. Use stored
records to explain its decisions, including after a restart, with the
read-only inspection commands.

## Health and metrics

Use liveness to check that the process is running and readiness to decide
whether it can accept work. `/metrics` reports counters and latency measures
with a bounded set of labels. Metrics include pipeline counters and bounded
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
disconnected when its bounded lag is exceeded. Expired cursors, repeated
delivery failures for an invalid notification, and audited skips are recorded
explicitly.

## Explain a decision

Start from the notification or command ID and follow durable links through:

```text
event -> situation_version -> scheduler_item -> episode/attempt
      -> decision -> intent -> policy_audit -> command/outbox -> outcome
```

The chain is inspectable read-only from the CLI: `situation list|show`,
`explain situation|trigger`, `episode show` and `intent show` (through the
command's outcome, verification and any watch it installed). There is no HTTP
query endpoint. Changes go through their own commands: `quarantine release`,
`commands resolve` for uncertain outcomes, and the signed `/v1/approvals/{id}`
flow for approvals. See the [CLI reference](../reference/cli.md).

## Next reads

- [Observability design](../design/observability.md)
- [Notification contract](../contracts/notifications.md)
- [Recovery](recovery.md)
