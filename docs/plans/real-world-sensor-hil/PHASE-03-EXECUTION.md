# Phase 03 execution plan — governed gateway-link effector

Status: repository implementation complete; phase gate remains open pending
full repository and cross-repository/emulator evidence. The gateway transport
adapter and physical HIL remain external to this repository.

This plan implements the Agentic Stream side of G4a/G4b. The edge gateway
continues to own raw serial bytes, framing, reconnect, device identity, and
its delivery ledger. Agentic Stream receives and sends typed versioned device
records through `DeviceTransport`; it does not add a second serial stack.

## Bar

Phase 03 is complete only when the following are proven by named tests:

1. Strict NDJSON codec helpers accept exactly one supported device record,
   reject malformed/trailing/oversized/unknown/wrong-version records, and
   validate the corresponding embedded schema before returning a document.
2. `DeviceSession` performs a state handshake before sending any command,
   requires protocol and capability-digest agreement with the configured
   catalog, exposes the current boot, and rejects stale boot/session state.
3. `SerialEffector` implements `AuthorizedEffector`, checks the final
   interlock, materializes the closed route exactly once, sends only encoded
   bounded commands, and correlates receipt/result to the command and boot.
   A receipt returns `VerificationPending`; it never becomes physical success.
4. Pre-send failures produce an ordinary dispatch failure and zero sends;
   ambiguous transport after possible send produces `UnknownOutcomeError`;
   accepted receipt/result remains awaiting independent feedback; duplicate
   idempotency returns the stable prior receipt without a second send.
5. Composite routing explicitly sends `set_indicator` and
   `select_thermal_mode` to the serial effector, keeps watch conditions on the
   watch effector, and rejects unknown routes when no configured fallback is
   allowed. Replay/shadow profiles cannot reach this effector.
6. The session/effector tests prove wrong boot, unknown capability, malformed
   receipt, expired/not-before, out-of-range, and interlock cases fail closed.
   No test treats a commanded value as an observation or verification.

Physical HIL evidence, firmware safe-state/watchdog behavior, and the gateway's
raw serial implementation remain external evidence. This phase must not claim
that an emulator test proves a physical LED or fan effect.

## Implementation result

The repository now has strict device-record codec helpers, a capability-digest
bound DeviceSession, a governed SerialEffector, explicit thermal routing,
startup profile isolation, and action-boundary telemetry counters. The
dispatcher still supplies lease, policy revalidation, runtime ownership,
interlock, and unknown-outcome ledger semantics. The gateway transport adapter,
firmware behavior, independent feedback verifier, and physical HIL evidence
remain external or Phase 04 work.

## Implementation sequence

### 3.1 Strict device-record codec

- Add a narrow codec in `internal/actions` that maps `message_type` to the
  registered device schema, decodes one JSON object with a fixed maximum frame
  size, rejects trailing bytes/records, and validates before returning.
- Encode only the four supported device records and always emit one NDJSON
  line. Do not accept arbitrary message types or silently drop fields.
- Add table-driven malformed/wrong-version/unknown-field/trailing/oversized
  tests and a fuzz target that asserts no invalid input yields a valid command.

### 3.2 Capability digest and session handshake

- Give `CapabilityCatalog` a canonical digest over its validated document.
- Add `DeviceTransport` and `DeviceSession` around the typed gateway link.
- Open by receiving and validating `device.state`; require protocol version,
  device/boot identity, capability digest equality with the catalog, and an
  explicit configured allow-list. Do not send a command before a successful
  open.
- Track session boot and semantic idempotency receipts under a mutex. A boot
  change invalidates the session's receipt cache and causes the effector to
  reject commands bound to the old boot.

### 3.3 Serial effector dispatch

- Implement `SerialEffector` as an `AuthorizedEffector` using the existing
  `Authorization.Check` immediately before materialization/send.
- Materialize from the policy command and current session boot; encode the
  result; send; decode a receipt and, when available, a result. Correlate
  command ID and boot ID exactly.
- Return a provider result containing typed receipt/result documents and set
  `VerificationPending` for accepted transport receipts. Return
  `UnknownOutcomeError` when bytes may have left the host but no trustworthy
  receipt was obtained. Never retry inside the effector.
- Return ordinary errors before send for stale boot, expired/not-before,
  interlock, schema, target, route, and bound failures.

### 3.4 Routing, profile isolation, and metrics

- Extend `CompositeEffector` with explicit thermal-route dispatch to the
  serial effector. Keep a fallback only for configured non-physical routes;
  unknown routes must fail when no fallback is intentionally supplied.
- Add a profile constructor/configuration check for simulated, emulator, and
  physical modes. Reject replay/JSONL plus a physical gateway link at startup;
  replay/shadow code must not accept an `Effector` or credential capability.
- Add minimal counters for frame errors, reconnects, unknown outcomes,
  verification-pending/failures, lease expiry, and safe-state observations via
  the existing telemetry surface where the package boundary supports it.

## Validation and evidence

Before committing, run the changed action/contract/storage/runtime packages,
`go vet ./...`, `go build ./...`, `git diff --check`, and documentation checks.
Run `go test ./...` and `make ci-check`; preserve exact environment blockers.
The phase commit must name codec, handshake, receipt/unknown, routing, and
profile-isolation tests. No G4a/G4b physical claim is allowed without the
external emulator/HIL evidence matrix from Phase 05.

## Explicit non-goals

- No direct USB/Arduino serial library or raw framing implementation.
- No firmware watchdog, hard limit, emergency stop, or device dedup ledger;
  those remain firmware/gateway responsibilities.
- No independent feedback verifier or reboot authority barrier; those are
  Phase 04.
- No promotion of a shadow comparison into an active command.
