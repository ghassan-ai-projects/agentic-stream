# Code and boundary audit

| Candidate | Production evidence | Decision |
| --- | --- | --- |
| `NewJSONLReplay`, `NewJSONLReplayWithClock`, `NewSimulatorJSONLReplay`, `NewLiveUDSSource`, `WithLogger`, `WithTelemetry` | Runtime and replay transports constructed one per call; tests constructed them directly | One `New(Config)` and three service operations |
| Simulator checkpoint reader returning `0, nil` on any query error | Fail-open on a storage error | Shared strict reader: only a missing row means line 0 |
| Two checkpoint writers with different SQL and key order | Same table, same meaning | One upsert and one codec |
| `readBoundedLine` and `readLiveLine` | Two framings with different resync rules | Both kept, in transport, with their distinct contracts documented |
| `LiveUDSSource` nil-source and nil-log guards | Unreachable once `New` guarantees both | Removed |
| `quarantineID` receiver method | Never used its receiver | `domain.QuarantineID` |
| Embedded `simulator_data.json` beside the package root | Domain data loaded by machinery | Moved with domain (`go:embed` needs it in the package); AGENTS.md and guides updated |

Ingress owns `connector_checkpoints`, now pinned to `internal/ingress/internal/store`.
It reads no foreign table and writes the event log only through the `eventlog`
facade. Events are appended before the checkpoint, so recovery is at-least-once
with deduplication by the event log; making the batch and its checkpoint one
transaction would change that and is deferred.
