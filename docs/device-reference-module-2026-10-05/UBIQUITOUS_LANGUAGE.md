# Ubiquitous language: device boundary

The device boundary turns an approved command into bounded bytes for one
physical device, and turns what the device says back into evidence. It shares
the identity words of the [device-authority language](../authority-reference-module-2026-10-05/UBIQUITOUS_LANGUAGE.md)
(owner, device boot, target, target claim, reconciliation, safe stop) and adds
the words below.

## Effect boundary

| Term | Meaning | Code |
| --- | --- | --- |
| **Effect profile** | Which effect boundary a runtime process may use: `simulated`, `emulator` or `physical`. | `EffectProfile` |
| **Gateway link** | The typed connection to the device gateway. The gateway owns raw serial framing; this side sends and receives whole device records. | `Transport` (port), `UDSTransport` (Unix socket) |
| **Live actuation** | Explicit operator consent that a physical profile may energize real outputs. | `EffectProfileConfig.LiveActuation` |
| **Gateway effector** | The effector that delivers approved commands to a device through a session. | `GatewayEffector` |
| **Simulated effector** | Deterministic stand-in that accepts commands once per idempotency key. | `SimulatedEffector` |
| **Fail-closed effector** | Fallback that refuses routes no live effector owns. | `FailClosedEffector` |

## Capability catalog

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Capability catalog** | The closed set of routes and safe stops a device accepts. Configuration, never model output. Identified by its digest. | `CapabilityCatalog` | catalog JSON |
| **Route** | One approved effector route mapped to one device operation and target. | `Route` | `routes.<name>` |
| **Preset** | A named, complete parameter set a route may send. The only source of device parameters. | `Route.Presets` | `presets` |
| **Selector** | The command payload field that names a preset. The model may only choose among presets. | `Route.SelectorField` | `selector_field` |
| **Hard bound** | An inclusive numeric limit re-checked on the parameters actually sent. | `Bound` | `bounds` |
| **Target binding** | A logical target name that resolves to the route's physical target. | `Route.TargetBindings` | `target_bindings` |
| **Materialize** | Turn an approved command into a device command using only the catalog. | `Materialize` | — |

## Device records

| Term | Meaning | Code | Wire `message_type` |
| --- | --- | --- | --- |
| **Device record** | One newline-delimited JSON message between runtime and device. | `wire.Encode`, `wire.Decode` | — |
| **Device command** | A materialized command bound to the expected boot. | `Command` | `command` |
| **Receipt** | The device's admission answer: accepted, or rejected with a code. Not proof of effect. | `Receipt` | `receipt` |
| **Result** | The device's terminal execution status for a command. Not physical confirmation. | `Result` | `result` |
| **Device state** | The device's report of identity, digests, safe state and current output. | `State` | `state` |
| **Exchange** | One command's send, receipt and result, in order. | `Exchange` | — |
| **Command identity** | A command's digest without its command ID; equal identities are the same command. | `Command.Identity` | — |

## Session

| Term | Meaning | Code |
| --- | --- | --- |
| **Device session** | The only object allowed to use a gateway link. Serializes exchanges and remembers receipts for the current boot. | `Session` |
| **Handshake** | The first device state, which must match the catalog digest, an allowed firmware digest and a complete identity before anything is sent. | `Open` |
| **State query** | Ask the gateway for a fresh device state. | `QueryState` |
| **Receipt cache** | Accepted exchanges remembered by idempotency key for the current boot, so a retry returns the first answer. | — |
| **Unknown outcome** | An exchange whose bytes may have reached the device but whose answer cannot be trusted. Opens a reconciliation and invalidates the link. | `actionport.UnknownOutcomeError` |
| **Safe-stop lane** | The priority path for the catalog's safe-stop command. Never blocked by a reconciliation; once requested, ordinary commands are refused. | `SafeStop` |
| **Output verification** | Comparing the observed output in a fresh state with the command that was sent. | `VerifyCommand` |

## Retired words

| Do not say | Say instead | Why |
| --- | --- | --- |
| serial effector, serial command | gateway effector, device command | Nothing here opens a serial port; the gateway does. |
| barrier (as a session flag) | reconciliation required | One name with the authority. |
| safe stop requested (as a sticky flag) | safe-stop latched | Same as the authority's latch. |
| refresh state | query state | One operation, one name. |
| `DeviceSession`, `DeviceTransport`, `DeviceExchange`, `EncodeDeviceRecord` | `Session`, `Transport`, `Exchange`, `wire.Encode` | The package name already says "device". |
| semantic digest | command identity | Says what it identifies. |
