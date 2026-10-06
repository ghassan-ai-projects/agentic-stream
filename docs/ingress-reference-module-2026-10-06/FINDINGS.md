# Ingress migration findings

## Mixed responsibilities

- [`jsonl.go`](../../internal/ingress/jsonl.go) mixes file reading, bounded line framing, quarantine decisions, checkpoint SQL and the replay loop.
- [`simulator.go`](../../internal/ingress/simulator.go), [`simulator_control.go`](../../internal/ingress/simulator_control.go) and [`simulator_event.go`](../../internal/ingress/simulator_event.go) mix the trace grammar with file reading, batching and checkpoint SQL; the grammar and conversion are pure.
- The four `live_socket*.go` files mix socket listener safety, client accounting, line framing, line admission, quarantine writes, telemetry and shutdown ordering on one `LiveUDSSource` type with `With*` setters.

## Inconsistencies between sources

- The simulator checkpoint reader treats any query error as "start at line 0" (`loadLineCheckpoint` returns `0, nil`), while the JSONL reader returns the error. Failing open on a storage error restarts a trace from line 0.
- The two checkpoint writers use different SQL (the JSONL upsert refreshes the version, the simulator one does not) and different JSON key order.
- The simulator always uses the physical clock; JSONL takes an injected clock; the live source sets its own.
- Tenant defaults differ: live and simulator default to `default`; JSONL uses the tenant as given.

## Optional dependencies

`NewLiveUDSSource` accepts a nil event log until `Run`; telemetry and logger are set after construction by `WithTelemetry`/`WithLogger`.

## Public surface

| Symbol | Production use | Decision |
| --- | --- | --- |
| `NewJSONLReplay`, `NewJSONLReplayWithClock`, `JSONLReplay.Run` | runtime and replay transports | `Service.ReplayJSONL` |
| `NewSimulatorJSONLReplay`, `SimulatorOptions`, `Run` | runtime transport | `Service.ReplaySimulator` |
| `NewLiveUDSSource`, `WithLogger`, `WithTelemetry`, `Run`, `EnvelopeSink` | runtime transport | `Service.ServeLive`; logger and telemetry in `Config` |

## Cross-module access

Ingress owns `connector_checkpoints` only. It appends and quarantines through the `eventlog` facade and validates schemas through it.
