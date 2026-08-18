# Configuration reference

The current runtime is configured through CLI flags and environment variables.
There is no committed config-file format, and `config effective` is a
placeholder.

## Environment variables

| Variable | Used by | Notes |
| --- | --- | --- |
| `AGENTIC_STREAM_MODEL_API_KEY` | native model provider | secret; sent as a Bearer header when configured |
| `AGENTIC_STREAM_SUBSCRIBER_TOKEN` | `serve`/SSE | required by `serve`; clients use `Bearer` form |
| `AGENTIC_STREAM_CONTROL_TOKEN` | drain/kill controls | full `Authorization` header value is compared |
| `AGENTIC_STREAM_OTLP_ENDPOINT` | telemetry | highest-priority OTLP/HTTP endpoint |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | telemetry | trace-specific fallback |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | telemetry | general OTLP fallback |

## Worker configuration

Worker transport is flag-based. `--worker-socket` selects the separate worker
route and prevents construction of the native executor. mTLS requires
`--worker-ca`, `--worker-cert`, `--worker-key`, and `--worker-server-name`.
EvidenceTools additionally requires `--evidence-socket` and a
32-byte-or-longer hex `--evidence-key`.

## Data and persistence paths

Trace and spec paths are supplied through `--trace` and `--spec` on `run`,
`run-live`, and `serve`; only `validate` accepts `<spec.yaml>` positionally.
The database path is explicit for live/serve workflows. Deterministic `run`
defaults to `<trace>.replay.db` and requires a fresh path.

## Secret rules

Do not put API keys, tokens, private keys, or HMAC secrets in specs, traces,
database fixtures, command examples, logs, or worker requests. Use the
deployment secret manager and redact environment dumps.

## Next reads

- [CLI](cli.md)
- [Security hardening](../operations/security-hardening.md)
- [Live runtime](../getting-started/live-runtime.md)
