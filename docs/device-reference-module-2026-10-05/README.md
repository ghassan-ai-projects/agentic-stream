# Device as a reference module — October 2026

Working record for rebuilding `internal/device` to the reference-module
standard set by [`internal/authority`](../authority-reference-module-2026-10-05/README.md),
following the [reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md).
Backward compatibility is not a goal.

- [Findings](FINDINGS.md)
- [Ubiquitous language](UBIQUITOUS_LANGUAGE.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

`device` is the effect boundary to physical devices. Unlike `authority` it owns
no tables: it talks to a device gateway over a socket, encodes and decodes
device records, materializes approved commands into bounded device commands,
and keeps a session's protocol state. Its durable facts are recorded through
`authority`.

Today one package mixes the device protocol rules, session orchestration,
the socket transport, the record codec, three effectors and the effect-profile
policy. Device records are `map[string]any` everywhere, 64 symbols are
exported although production callers use about 15, and several safety calls
are skipped when the authority is nil.

Target shape, the adapter-module form of the pattern:

```
internal/device/                      facade: effect profiles, effector constructors, catalog loading, gateway dial
internal/device/internal/app/         use cases: device session (handshake, refresh, exchange, safe stop,
                                      reconciliation, verification) and the effectors
internal/device/internal/domain/      rules: capability catalog and materialization, effect-profile policy,
                                      device records and their matching rules, output verification
internal/device/internal/wire/        adapter: device record codec (NDJSON + schema)
internal/device/internal/transport/   adapter: Unix-socket gateway link
```
