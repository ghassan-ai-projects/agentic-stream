# Ingress ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Connector | One named reader of an external source, with a durable position. | connector ID | `connector_checkpoints.connector_id` |
| Checkpoint | The last trace line a connector has fully read. | `domain.Checkpoint` | `checkpoint_blob` |
| Line verdict | The admission outcome of one JSONL line: admitted, or a quarantine reason. | `domain.LineVerdict` | none |
| Quarantine reason | Why a line was refused: `line_too_large`, `malformed_json`, `envelope_invalid`, `schema_invalid`. | `domain.Reason*` | quarantine record reason |
| Trace grammar | The record order a simulator trace must follow: `runtime_config`, events and activations in strictly increasing recorded time, then `trace_end`. | `domain.SimulatorTrace` | none |
| Live line | One framed line from a socket client with its connection and line number. | `domain.LiveLine` | none |
| Sink | The consumer of each admitted live envelope. | `ingress.EnvelopeSink` | none |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Source (for the module type) | Service operation | One service offers three source operations. |
| Replay (for the live socket) | Serve | A live socket has no end or checkpoint. |
