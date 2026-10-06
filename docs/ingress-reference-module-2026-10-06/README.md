# Ingress reference-module migration

`internal/ingress` reads normalized events from external sources and appends them to the event log: a JSON Lines trace file, a streams-simulator trace, and a live Unix-domain socket. It owns one table, `connector_checkpoints`. Three source types (`JSONLReplay`, `SimulatorJSONLReplay`, `LiveUDSSource`) mix line framing, socket plumbing, record grammar, quarantine decisions, checkpoint SQL and orchestration. This migration gives it the adapter-with-a-table layers while keeping admission order, quarantine identities and checkpoint semantics.

```text
internal/ingress (configured facade, layer 6)
  └── internal/app (replay and live-serve use cases, layer 5)
        ├── internal/domain (envelope admission, simulator grammar and conversion, identities, checkpoint codec, layer 2)
        ├── internal/store (connector checkpoint SQL, layer 3)
        └── internal/transport (file and Unix-socket framing, listener and client plumbing, layer 3)
```

Merge decision: ingress stays one module. The three sources share line admission, batching and the checkpoint table, and `runtime` and `replay` import it as a unit. No `wire` package: line framing is the transport's job and the record codec is a few JSON checks in domain.
