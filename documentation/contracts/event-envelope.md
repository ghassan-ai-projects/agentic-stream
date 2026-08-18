# Event envelope and ingress

Ingress converts source data into the normalized event envelope before the
event log accepts it. The envelope is evidence, not an executable command.

## Normalized envelope

The normalized JSONL shape contains:

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

HTTP, Kafka, NATS, and MQTT adapters are not part of the current runtime.

## Data quality behavior

Malformed or schema-invalid input is retained in bounded quarantine with a
stable line identity where possible. Released rows are validated again before
redrive. Duplicate IDs are ignored only when their payload digest agrees;
conflicting reuse is rejected. Event gaps preserve discontinuity rather than
inventing evidence.

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

- Envelope type and validation: [`internal/contractsv1/envelope.go`](../../internal/contractsv1/envelope.go)
- JSONL adapter: [`internal/ingress/jsonl.go`](../../internal/ingress/jsonl.go)
- Simulator adapter: [`internal/ingress/simulator.go`](../../internal/ingress/simulator.go)
- Schema registry data: [`internal/eventschema/registry_data.json`](../../internal/eventschema/registry_data.json)

## Next reads

- [Stream-processing design](../design/stream-processing.md)
- [Event catalog](../reference/event-catalog.md)
- [Recovery](../operations/recovery.md)
