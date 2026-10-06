# Validation and review

Coverage (short): facade 81.8%, app 81.2%, domain 84.0%, store 86.7%, transport
81.9%. The original suites moved to their owning layers with the same assertions:
file replay and resume, tenant fill, quarantine of malformed, schema-invalid,
connector-scoped and oversized lines, the simulator conversion, framing and
rejection cases, the bounded-line boundaries, live-socket quarantine and
telemetry, reconnecting clients, and a sink deadline propagated with an active
parent. New tests cover envelope admission order, identities, the checkpoint
codec, the socket path rule, every simulator grammar refusal, listener safety
(owner-only mode, symlink, regular file, active socket, removal on shutdown),
the client limit, handler-error shutdown, store round trips and storage
failure, and constructor refusal.

Fault injection: a failed checkpoint write leaves the replay resumable and the
event log deduplicates; a storage failure reading a checkpoint stops the replay.

Gates proven by injection: facade logic, an exported field on the store, a raw
`Exec` and `database/sql` in app, `os` in domain, SQL outside the store, and a
checkpoint write from app.

| Dimension | Rating / 10 | Remaining limitation |
| --- | --- | --- |
| Layering | 9 | None known |
| Domain rules | 9 | Simulator records stay `map[string]any` |
| Fail-closed safety | 9 | Batch and checkpoint are separate writes by design |
| Ubiquitous language | 9 | Connector kinds are literals |
| Tests | 9 | Socket tests need a writable `/tmp` |
| Encapsulation | 9 | Opaque store and transport |
| Type safety | 7 | Untyped simulator record maps |
| Simplicity | 8 | Two line framings with different resync contracts |
