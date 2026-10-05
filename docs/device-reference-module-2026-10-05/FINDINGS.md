# Findings

Baseline: `aa90233` on `general-improvements-1`. Paths are relative to
`internal/device/`.

## F1. Protocol rules, orchestration and I/O share functions

| Rule | Where it lives today |
| --- | --- |
| A receipt must name the command and the current boot | `serial_session_exchange.go` `receiptMatchesCommand`, called between transport reads |
| A result must agree with its receipt (executed / safe_state / rejected with the same code) | `resultMatchesCommand`, `resultMatchesReceipt`, inside the receive path |
| A device state must carry the allow-listed capability and firmware digests and a full identity | `serial_session_helpers.go` `validateDeviceState`, which also decodes the frame |
| A new boot discards cached receipts and requires reconciliation | `serial_session_state.go` `applyRefreshedState`, mixed with telemetry and authority calls |
| Observed output verifies a command | `serial_effector.go` `verifyObservedOutput`, inside the effector |
| A safe stop's identity is its digest without the command ID | `serial_materialize.go` `sealSafeStopDocument` and `serial_session_helpers.go` `semanticCommandDigest` (two copies of one rule) |

None of these rules can be tested without building a session and a fake
transport.

## F2. The public surface is much wider than its use

- 64 exported identifiers; production callers (`cmd`, `runtime`) use about 15:
  effect profiles, `ValidateEffectProfile`, `LoadCapabilityCatalog`,
  `CapabilityCatalog`, `DialUDSTransport`, `UDSTransport`, `NewGatewayEffector`,
  `GatewayEffectorConfig`, `SerialEffector` (`VerifyDeviceCommand`),
  `NewSimulatedEffector`, `NewFailClosedEffector`.
- `DeviceSession`, `OpenDeviceSession`, `EncodeDeviceRecord`,
  `DecodeDeviceRecord`, `NewEmulatorEffector`, `NewUDSTransport` and the
  session accessors are exported only for tests.
- `CapabilityCatalog` exposes mutable `Routes` and `SafeStops` maps after its
  digest has been accepted; the serial effector re-digests before every
  dispatch to compensate.

## F3. Safety calls are skipped when a dependency is nil

`OpenDeviceSession` requires an authority, but the session still guards it six
times:

- `claimCommand` returns an **unclaimed** target when the authority is nil, and
  the command is then delivered;
- `completeCommandOutcome` and `cacheCommandOutcome` skip `AssertClaim`;
- `releaseClaims`, `recordSafeStop` and `openReconciliation` skip or fail.

Only a struct literal (used by tests) reaches these branches, but the code
treats "no authority" as a supported mode.

## F4. Device records are untyped

97 `map[string]any` uses in production code. State, command, receipt and
result fields are read by string key (`"reject_code"`, `"error_code"`,
`"current_output"`, `"expected_boot_id"`); a misspelled key compiles and
silently reads a zero value. `documentInt64` casts JSON numbers by hand.

## F5. Vocabulary drift

| Same concept | Names in use |
| --- | --- |
| Commands are blocked until reconciled | barrier, `reconciliationRequired`, `bindHandshakeBarrier`, `persistHandshakeBarrier` |
| A safe stop was recorded for this boot | `safeStopRequested` (session) vs `SafeStopLatched` (authority) |
| Reading a fresh state | `RefreshState`, `QueryState`, `queryCurrentState`, `QueryStateEvidence` |
| The gateway effector | `SerialEffector`, `NewGatewayEffector`, `NewEmulatorEffector`, "serial command" |
| One command's receipt and result | `DeviceExchange`, `Exchange`, `ExchangeWithResult` |

`device.DeviceSession`, `device.DeviceTransport`, `device.DeviceExchange` and
`device.EncodeDeviceRecord` stutter.

## F6. Duplicate and redundant checks

- `AssertRuntime` runs before `RecordDeviceState` and `ResolveReconciliation`,
  which already run ordinary admission in their own transaction.
- 18 `if s.telemetry != nil` guards, although every telemetry method is
  nil-safe.
- The `DispatchAuthorized` preamble is copied into three effectors, each with a
  `fmt.Errorf("%w", err)` no-op wrap.
- `stateDigest` re-implements `canonicaljson.ContentDigest`.

## F7. A latent nondeterminism

`expectedOutput` returns the first numeric parameter it finds while ranging
over a map. With the current catalog each route has exactly one numeric output
besides `lease_ms`, so it is deterministic today; a route with two numeric
outputs would verify against a random one.

## F8. Mixed cohesion

The package holds four unrelated things: the effect-profile policy (process
composition), the simulated and fail-closed effectors, the gateway transport,
and the device session. They share no state.
