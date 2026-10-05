# Target design

## Layers

`device` is an adapter module: it owns no tables, talks to an external system
(the device gateway), and records its durable facts through `authority`.

```
          callers (cmd, runtime)
                  │  public API only
                  ▼
 internal/device                     FACADE: effect profiles, effector constructors,
                                     catalog loading, gateway dial
                  │
                  ▼
 internal/device/internal/app        LOGIC: session use cases and effectors
     │            │            │
     ▼            ▼            ▼
  domain        wire       transport
  RULES         ADAPTER    ADAPTER
  pure          record     Unix-socket
                codec      gateway link
```

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade (`internal/device`) | Public types and constructors; configuration checks; delegation | hold protocol rules or I/O |
| Logic (`internal/app`) | Session use cases: handshake, state query, exchange, safe-stop lane, reconciliation, output verification; the three effectors; calls to `authority` | encode bytes or touch sockets directly; decide protocol rules itself |
| Rules (`internal/domain`) | Capability catalog and materialization, effect-profile policy, device record types and matching rules, state identity checks, command identity, output verification | perform I/O or read a clock |
| Codec (`internal/wire`) | Encode and decode device records: size limit, single NDJSON line, schema by `message_type` | decide anything beyond well-formedness |
| Transport (`internal/transport`) | The Unix-socket gateway link: framing, deadlines, cancellation, the "may have been sent" signal | interpret records |

`app` sees the gateway only through a `Transport` port (Send, Receive,
QueryState, Close); `transport.UDS` implements it.

## Public API (facade)

| Group | API |
| --- | --- |
| Effect profile | `EffectProfile` and its three values, `EffectProfileConfig`, `ValidateEffectProfile` |
| Catalog | `CapabilityCatalog`, `LoadCapabilityCatalog` |
| Gateway | `Transport` (port), `UDSTransport`, `DialUDSTransport(ctx, socketPath)` |
| Effectors | `NewGatewayEffector(ctx, GatewayEffectorConfig)`, `GatewayEffector` (`Dispatch`, `DispatchAuthorized`, `SafeStop`, `VerifyDeviceCommand` — the last name is fixed by `actionport.DeviceStateVerifier`), `NewSimulatedEffector` and `NewFailClosedEffector` (both return `actionport.AuthorizedEffector`) |

Everything else — sessions, record codec, materialization helpers — stays
internal and is tested in its own layer.

## Rules the session follows

1. A session exists only after a valid handshake; `Open` requires an
   authority, a catalog, allow-lists and a transport. No operation checks for
   a nil authority.
2. Ordinary exchange order: session open → no safe-stop latch → command bound
   to the current boot → no reconciliation required → claim target → bind
   command → assert claim → send → receipt → assert claim → result → assert
   claim → cache.
3. Any untrustworthy answer after bytes may have left opens a reconciliation
   (falling back to the priority path on authority loss) and invalidates the
   link.
4. The safe-stop lane bypasses the reconciliation check and ordinary
   admission; it latches the boot first, and every stage is recorded.
5. Telemetry calls are unconditional; the telemetry runtime is nil-safe.

## Enforcement

| Rule | Check |
| --- | --- |
| Domain is pure | `TestDomainPackagesArePure` (already generic) |
| App touches neither the database nor the network | `TestApplicationLayersDoNotTouchInfrastructure` (generic; now also forbids `net` and `net/http`) |
| Reasoning and replay cannot reach the device implementation | `TestReasoningAndReplayCannotReachEffectImplementations` now also targets `internal/device/internal/app` and `internal/device/internal/transport` |
| Layer order | `packageLayers`, `allowedImports` |

## Deferred: production entry points

Barrier clear (`Session.ResolveReconciliation`) and the safe-stop lane have no
production caller; see [why they exist](PLAN.md#why-safe-stop-and-barrier-clear-stay).
They move into the new layers unchanged and get entry points in a separate,
designed change.
