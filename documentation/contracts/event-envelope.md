# Event envelope and ingress

Ingress converts source data into a common event format, the **envelope**,
before the event log accepts it. The envelope carries an observation and its
identity, source, and times. It does not carry permission to execute a command.

## Normalized envelope

Each normalized JSON Lines (JSONL) record contains:

| Field | Role |
| --- | --- |
| `id` | Stable event identity for deduplication |
| `type`, `schema_version` | Event contract identity |
| `tenant_id`, `source` | Ownership and provenance |
| `partition_key`, `entity` | Virtual partition and domain identity |
| `event_time`, `observed_at`, `ingested_at` | Time semantics |
| `correlation_id`, `causation_id`, trace fields | Causality and tracing |
| `classification`, `quality` | Data handling and quality annotations |
| `data` | Typed domain payload |

Required identity/time fields are checked before append. The selected runtime
tenant must match the envelope tenant. A schema registry check validates the
payload type and declared fields.

## Supported input adapters

- **Normalized JSONL:** one envelope per line, used by `run`, `run-live`, and
  `serve` by default.
- **Simulator JSONL:** the streams-simulator `trace-record-v0.1` adapter,
  selected with `--trace-format simulator` on live workflows.

- **Live normalized JSONL socket:** `serve --live-socket` accepts normalized
  envelopes over a Unix domain socket. It is mutually exclusive with `--trace`
  and requires `--trace-format normalized`.

HTTP, Kafka, NATS, and MQTT adapters are not part of the current runtime.

## Data quality behavior

Malformed or schema-invalid input is retained in bounded quarantine with a
stable line identity where possible. Released rows are validated again before
**redrive**, an attempt to process them again. Duplicate IDs are ignored only
when their payload digest agrees; conflicting reuse is rejected. Gap records
identify missing input; they do not invent replacement evidence.

## Example envelope

```json
{
  "id": "motor-17-vibration-001",
  "type": "motor.vibration.observed",
  "schema_version": "1.0",
  "tenant_id": "default",
  "source": "sensor-gateway",
  "partition_key": "motor-17",
  "entity": {"type": "motor", "id": "motor-17"},
  "event_time": "2026-01-01T00:00:00Z",
  "ingested_at": "2026-01-01T00:00:01Z",
  "classification": "internal",
  "quality": [],
  "data": {"rms_mm_s": 5.0}
}
```

## Source evidence

- Envelope type and validation: [`internal/contractsv1/internal/domain/envelope.go`](../../internal/contractsv1/internal/domain/envelope.go)
- JSONL adapter: [`internal/ingress/internal/app/jsonl.go`](../../internal/ingress/internal/app/jsonl.go)
- Simulator adapter: [`internal/ingress/internal/app/simulator.go`](../../internal/ingress/internal/app/simulator.go)
- Live socket adapter: [`internal/ingress/internal/app/live.go`](../../internal/ingress/internal/app/live.go)
- Schema registry data: [`internal/spec/event_schema_data.json`](../../internal/spec/event_schema_data.json)

## Next reads

- [Stream-processing design](../design/stream-processing.md)
- [Event catalog](../reference/event-catalog.md)
- [Recovery](../operations/recovery.md)
