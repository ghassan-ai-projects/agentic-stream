# Ingress module design

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade (`internal/ingress`) | `New(Config)`, `ReplayJSONL`, `ReplaySimulator`, `ServeLive`, the option and sink types | Logic, SQL, sockets |
| App (`internal/app`) | The replay loops, event-log batching, the schema step of admission, quarantine writes, live line handling (telemetry, sink) | SQL, `os`, `net`, `database/sql` |
| Domain (`internal/domain`) | Envelope decode and contract admission, quarantine identities, checkpoint codec, simulator trace grammar and event conversion (with the embedded channel data), connector-ID and option defaults, socket-path rule | I/O, clock reads, `os`, `net`, `database/sql` |
| Store (`internal/store`) | `connector_checkpoints` SQL | Deciding where to resume |
| Transport (`internal/transport`) | Trace file opening, bounded line framing for files and sockets, Unix-socket listener safety, accept loop, client registry and limit, queue and shutdown ordering | Admission or quarantine decisions |

`Config` requires `DB` and `Log`; `Clock` defaults to the physical clock, `TenantID` to `default`; `Logger` and `Telemetry` are optional. Admission order is preserved: decode, envelope contract, then event schema; an oversized line is quarantined before decoding. Live lines are admitted in arrival order, and a client disconnect is never a runtime failure.

Enforcement: `TestIngressFacadeOnlyDelegates`, `TestIngressStoreKeepsTransactionsOpaque`, `TestIngressApplicationUsesTransactionalPorts`, the existing domain/app/SQL gates, and `durableOwners` pointing `connector_checkpoints` at the store. No schema change.
