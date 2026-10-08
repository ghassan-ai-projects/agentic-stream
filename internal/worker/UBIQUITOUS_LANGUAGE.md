# Worker ubiquitous language

| Term | Meaning | Code name | Wire name |
| --- | --- | --- | --- |
| Episode worker | An out-of-process reasoner that serves the streamed `EpisodeWorker` protocol. It has no effectors, credentials or persistence. | `internal/testsupport/workerfake.Server` (test fake); the runtime is the client | `EpisodeWorker` gRPC service |
| Handshake | The first call: the worker states its name, version and features; the runtime checks them against protocol version and the features it requested. | `ProtocolVersion`, `ContractVersion` (fake: `workerfake.Server.Handshake`) | `HandshakeRequest`, `HandshakeResponse` |
| Feature | A named optional capability negotiated at handshake. | `EvidenceToolsFeature` | `evidence_tools.v1` |
| Episode stream | The sequence of events a worker emits for one request: started, model and tool progress, budget reports, diagnostics, at most one Decision, and exactly one terminal. The runtime may also send a cancelling event. | fake: `workerfake.ExecuteFunc`, `workerfake.Server.Execute` | `EpisodeEvent` |
| Terminal | The final event that classifies how the attempt ended. A Decision is accepted only with a matching terminal. | `Terminal` event | `Terminal` |
| Episode budget | The ceilings the runtime imposes: wall time, model calls, input and output tokens, tool calls, tool result bytes, provider retries and cost in microunits. | `ValidateBudget` | `EpisodeBudget` |
| Stream limits | Bounds on request bytes, event bytes, event count and total stream bytes. | `DefaultMaxEvents`, `DefaultMaxStreamBytes` | — |
| Evidence socket | The private, runtime-owned Unix socket on which workers call evidence tools. It refuses symlinks and live sockets. | `ListenEvidenceSocket`, `DialEvidenceSocket`, `ValidateEvidenceSocketPath` | UDS path |
| Worker socket | The Unix socket on which the runtime reaches a worker, optionally with TLS. | `DialEpisodeWorkerSocket`, `DialEpisodeWorkerSocketTLS` | UDS path |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Agent (for the worker) | Episode worker | An agent suggests it may act; a worker proposes typed Decisions and nothing else. |
| Plugin | Feature | Workers are not loaded into the runtime; features are negotiated over the protocol. |
