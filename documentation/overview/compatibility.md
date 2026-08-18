# Compatibility

Audience: evaluators and integrators. Scope: the tested language, platform,
storage, protocol, ingress, and API baseline—not a promise of broad platform
support.

## Supported baseline

| Surface | Current support |
| --- | --- |
| Language | Go `1.26.5` or a compatible Go 1.26 toolchain; see `go.mod` |
| Runtime shape | One Go binary with optional Go worker processes |
| Operating system | Unix-like systems with Unix domain socket support for worker mode |
| Persistence | SQLite WAL through `modernc.org/sqlite` |
| Authoring | YAML `SituationSpec`, validated by the committed JSON Schema |
| Rule expressions | Restricted deterministic CEL through `cel-go` |
| Local API | Go `net/http`, JSON health/readiness, metrics, Server-Sent Events |
| Worker protocol | Current-v1 Protobuf/gRPC, Unix domain socket by default |
| Telemetry | OpenTelemetry traces and runtime metrics; OTLP/HTTP when configured |
| Ingress | Normalized JSONL and simulator JSONL adapter |

The module path is
`github.com/ghassan-ai-projects/agentic-stream`. The exact dependency set is
in [`go.mod`](../../go.mod); it is the compatibility authority.

## Not supported by the current version

- Python worker implementations.
- A stable external SDK or public Go package contract beyond the documented
  worker and data boundaries.
- Kafka, NATS, MQTT, or other broker-backed ingress in the runtime binary.
- A remote listener directly exposed by `serve`; non-loopback deployment needs
  an authenticated proxy.
- Exactly-once guarantees for arbitrary external providers. Unknown outcomes
  are durable and require reconciliation.

## Versioning

The CLI reports runtime version, contract version, and protocol version through
`agentic-stream version`. Build metadata defaults to `dev`/`none` and can be
injected by the Makefile's linker flags. SituationSpec and wire contracts carry
their own versions and digests; do not infer compatibility from a Git commit
alone.

## Next reads

- [Install](../getting-started/install.md)
- [Worker protocol](../contracts/worker-protocol.md)
- [Limitations](limitations.md)
