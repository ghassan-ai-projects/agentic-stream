# Ingress module

Ingress reads normalized events from external sources and appends them to the
event log: a JSON Lines trace file, a streams-simulator trace, and a live
Unix-domain socket. It owns one table, `connector_checkpoints`. It decides
nothing about reasoning or effects.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `New(Config)`, `ReplayJSONL`, `ReplaySimulator`, `ServeLive`, and the option and sink types |
| App | Replay loops, event-log batching, the event-schema step of admission, quarantine writes, live line handling with telemetry |
| Domain | Envelope admission, quarantine and connector identities, the checkpoint codec, the simulator trace grammar and event conversion with its embedded channel data, the socket path rule |
| Store | Connector checkpoint SQL |
| Transport | Trace file opening, bounded line framing, Unix-socket listener safety, accept loop, client limit and shutdown ordering |

`New` requires the database and event log. The clock defaults to the physical
clock and the tenant to `default`; logger, telemetry and the live queue size are
optional.

A line is admitted in order: decode, envelope contract, then the registered
event schema. An oversized, malformed or invalid line is quarantined and skipped
rather than aborting the replay; quarantine identities are scoped to their
connector (or to the live source's instance, connection and line). File traces
resynchronize after an oversized line; a socket client that sends one is
disconnected. Events are appended in batches before the checkpoint is written,
so a failed checkpoint write re-reads the trace and the event log deduplicates.
A storage error while reading a checkpoint stops the replay; it never means
"start at line 0".

The simulator trace must open with `runtime_config`, carry events and model
activations in strictly increasing recorded time, and close with `trace_end`.
The channel-to-field mapping is data in
`internal/domain/simulator_data.json`, not code.

Migration record (`docs/ingress-reference-module-2026-10-06/README.md`).
Architecture gates enforce a pure domain, an opaque store, exact write ownership
of `connector_checkpoints`, and facade delegation.
