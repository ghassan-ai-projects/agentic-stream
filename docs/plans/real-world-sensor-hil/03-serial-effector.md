# Phase 03 — Physical effector (WP4 → Gates G4a LED, G4b fan)

**Goal:** one accepted semantic intent becomes **one** policy-bounded device
command and **one** independently verified physical effect — first an LED, then a
5 V fan. This is the core Agentic Stream deliverable.

**Program proofs:** Experiment 5 (one bounded effect) and Experiment 6 (false
success). **Allowed claims:** after G4a — *one bounded LED effect through the
governed action plane*; after G4b — *one independently verified low-voltage
mechanical effect*.

The whole phase sits behind the existing `actions.Effector` /
`AuthorizedEffector` interface (`internal/actions/dispatcher.go`). **Reuse** the
dispatcher's lease, revalidation, interlock, unknown-outcome, and reconciliation
machinery — it already implements most of the safety envelope. New code is the
effector, its device session, the materializer, and the wire schemas.

---

## Task 3.1 — Device wire schemas (shared contract)

Add versioned JSON Schemas under `internal/contractsv1/schemas/v1/` for the four
records the boundary needs (`ARCHITECTURE_AND_PROTOCOLS.md` → *Canonical
records*). Keep `snake_case`.

- `device.command/1` — `message_type`, `protocol_version`, `command_id`,
  `idempotency_key`, `target`, `operation`, `parameters`, `expected_boot_id`,
  `not_before_mono_us`, `expires_after_ms`, `policy_digest`. Relative expiry
  (`expires_after_ms`) after a boot-bound handshake — **not** a server UTC
  deadline (a raw device cannot reliably interpret one).
- `device.receipt/1` — bytes parsed + command accepted for execution (receipt ≠
  result).
- `device.result/1` — device-reported execution outcome.
- `device.state/1` — the handshake/reconciliation state query response
  (`device_id`, `boot_id`, `firmware_digest`, `protocol_version`,
  `capability_digest`, `safe_state`, current output state, dedup-ledger summary).

Register them in `internal/contractsv1` next to the existing `SchemaCommand` /
`SchemaOutcome` constants so `contractsv1.Validate` can check them. These schemas
are the **single source of truth**; the gateway/emulator/firmware repos consume
generated copies (README § 6). Do not hand-fork them.

**Exit:** the four schemas validate golden frames; a fuzz/property test rejects
malformed, oversized, wrong-version, and unknown-type frames (Experiment 2's
Agentic Stream half — the codec must fail closed and never yield a valid
unintended command).

---

## Task 3.2 — Deterministic intent→command materializer

Between the policy-approved `actions.Command` (whose `Payload` already contains
only bounded parameters — see README § 4) and the device wire command, add a
**deterministic materializer**. It is the guarantee that no arbitrary target,
pin, or opcode ever comes from model output (`ARCHITECTURE_AND_PROTOCOLS.md` →
*Required project changes* #2).

Place it in a new file `internal/actions/serial_materialize.go`:

- Input: `Command{EffectorRoute, NormalizedTarget, IdempotencyKey, Payload}`.
- A **closed capability catalog** loaded from effector config maps
  `(EffectorRoute, NormalizedTarget)` → allowed `operation` + a bound table
  (`duty_permille` max, `lease_ms` max, allowed enum values). The catalog is
  configuration, not model output.
- Re-clamp / reject: even though policy already materialized bounded params via
  `presets`, the materializer **re-enforces** the hard bound here (defense in
  depth). A param above bound is rejected, not silently clamped, and the
  rejection is a durable `dispatch_failed` (not `outcome_unknown` — nothing left
  the host).
- Output: a `device.command/1` document with `idempotency_key` carried through
  unchanged (`idempotency_key` identifies one *semantic effect*; `command_id`
  identifies one *dispatch*), `expected_boot_id` from the live session (Task
  3.4), and `expires_after_ms` from config.

**Rule:** the materializer never reads any free-form string from the model into a
target/operation. Only `modelWritableFields` values already validated against the
intent's `parameterSchema` may appear, and only where the catalog allows them.

**Exit:** table-driven tests prove valid intents map to exactly one bounded
command; out-of-range / wrong-target / unknown-operation inputs fail closed with
zero commands emitted.

---

## Task 3.3 — The `serial-device` effector

New file `internal/actions/serial_effector.go`, implementing
`AuthorizedEffector`:

```go
type SerialEffector struct {
    session   *DeviceSession   // Task 3.4 — handshake + transport
    catalog   CapabilityCatalog
    clk       clock.Clock
    metrics   SerialMetrics    // Task 3.7
    interlock interlock.Reader
}

func (e *SerialEffector) DispatchAuthorized(ctx context.Context, cmd Command, auth Authorization) (Effect, error)
```

Dispatch algorithm (all of it inside the dispatcher's lease/timeout window):

1. `auth.Check(ctx)` first — the final interlock/authorization at the concrete
   boundary (mirror `WatchEffector.DispatchAuthorized`).
2. Materialize the wire command (Task 3.2). A materialization rejection →
   ordinary error (`dispatch_failed`).
3. Verify the live session's `boot_id` still equals the command's
   `expected_boot_id` and the authority epoch is current (Task 3.4 / Phase 04). A
   mismatch → **reject** (`dispatch_failed`, "stale boot/epoch") — never send.
4. Send the frame; await **receipt** within the send deadline.
   - Receipt received → the command is *accepted for execution*, not done. Return
     `Effect{ProviderResult: <receipt+result-so-far>}` with the **verification
     still pending** (Task 3.5). The command's success is decided by
     verification, not the receipt.
   - No receipt / ambiguous (send succeeded but reply lost, or deadline exceeded
     after bytes went out) → return `&UnknownOutcomeError{...}`. The existing
     dispatcher path records `outcome_unknown` + `reconciling` and never blindly
     retries (`dispatcher.go` finalizeTx). **Do not** invent a new unknown path.
   - Send failed before any byte left the host → ordinary error
     (`dispatch_failed`); safe to let policy re-drive later.
5. Idempotency: the device dedups on `idempotency_key`; the effector also
   remembers `idempotency_key → receipt` for the session so a duplicate dispatch
   of the same semantic effect returns the stable prior receipt (mirror
   `SimulatedEffector`).

Transport is abstracted so the **emulator** (Streams Simulator, over a UDS or
pipe) and the physical edge gateway share one effector. The edge gateway owns
the raw serial framing and device identity; Agentic Stream consumes the typed
gateway link. Define:

```go
type DeviceTransport interface {
    Send(ctx context.Context, frame []byte) error
    Receive(ctx context.Context) ([]byte, error) // receipt/result/state frames
    Close() error
}
```

- NDJSON transport first (bring-up), COBS/CBOR framed transport later behind the
  same interface (decision-log item 6). The emulator and gateway adapter
  implement `DeviceTransport`; no serial library is needed in Agentic Stream,
  including the physical profile.

**Exit:** Experiment 5 trials pass against the emulator transport — only the valid
command acts; out-of-range / expired / wrong-target / wrong-boot / repeated-key /
command-during-fault all fail closed; the lease returns the output to safe state.

---

## Task 3.4 — Device session + handshake

New `DeviceSession` (same file or `internal/actions/serial_session.go`):

- On open, perform the handshake: read `device.state/1` and record `device_id`,
  `boot_id`, `firmware_digest`, `protocol_version`, `capability_digest`,
  `safe_state`. Reject if `protocol_version` is unsupported or `capability_digest`
  is not in the configured allow-list (`ARCHITECTURE_AND_PROTOCOLS.md` →
  *Required project changes* #3).
- Expose the current `boot_id` and `authority_epoch` to the effector for the
  freshness check (Task 3.3 step 3, and Phase 04).
- The session is the only thing that touches the transport; the model never does
  (Tamoz "does not get the serial port").

**Exit:** a handshake mismatch (wrong firmware digest, unknown capability,
unsupported protocol) refuses to open the session; no command can be sent without
a valid session.

---

## Task 3.5 — Receipt / result / observation / verification split

The four records are distinct and must never collapse into `ack: true`.

- **Receipt** and **result** come back over the transport → stored on the
  `Effect.ProviderResult` and the outcomes ledger (existing `outcomes` table via
  `dispatcher.finalizeTx`).
- **Observation** — the independent feedback (`zone.fan_tach.observed` from Phase
  01) arrives through **ingress as an ordinary event**, not through the effector.
  This preserves independence: physical truth comes from the feedback sensor, not
  the command record.
- **Verification** — a runtime step evaluates the observation against the
  command's expected effect and deadline and drives the existing `verifications`
  table. Implement verification as a small reconciler that:
  - marks `verified` when feedback within the deadline matches the expected
    effect (e.g. fan tach > 0 after a `bounded_cooling` command);
  - marks `verification_failed` when the command was received but the plant did
    not change (Experiment 6: acknowledged / no motion);
  - leaves `outcome_unknown` → `reconciling` when motion occurred but no receipt,
    and closes it via `Dispatcher.ReconcileUnknown` using the feedback as
    evidence — **without a blind resend**.

Do **not** copy commanded state into observed state, and never use the command
record as verification evidence (Experiment 6 stop condition).

**Exit:** Experiment 6 trials pass — ack/no-effect → `verification_failed`;
effect/no-ack → `unknown` then reconciled from independent feedback; a stuck
sensor at the expected value does not count as verification.

---

## Task 3.6 — Physical deployment profile (isolation)

Add a deployment profile that **cannot** load a replay/shadow configuration and a
live serial credential at the same time (`ARCHITECTURE_AND_PROTOCOLS.md` →
*Required project changes* #9; security checklist "replay/shadow environments
cannot reach a live port").

- Extend the CLI wiring (`cmd/agentic-stream/main.go`) so the effector selection
  is profile-driven: `simulated` (default, current), `emulator` (serial effector
  over emulator transport — safe, no physical port), and `physical` (serial
  effector over a real port, requires an explicit live-actuation flag + the owner
  authorization from decision-log item 10).
- Enforce mutual exclusion at startup: a config that sets both a replay/JSONL
  source **and** a physical serial port is a hard startup error. Replay
  (`internal/replay`) must remain inert — it never performs external effects, and
  the physical effector must be unreachable from a replay run.
- The physical profile must also fail closed if the durable interlock is not
  `ready` (reuse `interlock.DurableReader`).

**Exit:** a test asserts that (a) `physical` + replay source refuses to start, and
(b) replay of a trace that contains a `select_thermal_mode` intent produces no
serial send.

---

## Task 3.7 — Protocol + action metrics

Add the metrics the program lists (`ARCHITECTURE_AND_PROTOCOLS.md` → #8) via the
existing `internal/telemetry`: frame errors, reconnects, unknown outcomes,
verification failures, lease expiries, and safe-state entries. Also stage-latency
(dispatch/receipt/execution/verification) for the soak report in Phase 04.

**Exit:** metrics increment under the fault fixtures and are exported through the
existing telemetry path.

---

## Composite routing

`internal/actions/composite_effector.go` currently routes
`install_watch_condition` to the watch effector and everything else to a single
fallback. Extend the composition so the thermal routes (`set_indicator`,
`select_thermal_mode`) route to the `SerialEffector`, watch installs stay on the
watch effector, and any other route still hits the configured fallback. Keep the
routing table explicit and closed — an unknown route is an error, never a
default-send.

---

## Gate G4a (LED) exit checklist

- [ ] Device wire schemas validate golden frames; codec fuzz fails closed.
- [ ] Materializer emits exactly one bounded command for a valid `set_indicator`
      intent; every out-of-bound / wrong-target case yields zero commands.
- [ ] The LED lights **only** on the valid governed command; expired / malformed /
      out-of-range / wrong-boot / replayed commands are rejected.
- [ ] A restart never causes a blind resend (reuse outbox/lease + unknown path).
- [ ] Physical profile refuses to co-load replay/shadow with a live port.
- [ ] The complete LED run is explainable from durable evidence.

## Gate G4b (fan) exit checklist — additionally

- [ ] `select_thermal_mode` (R2) requires approval and dispatches only the exact
      approved, materialized bytes (policy plane revalidates at dispatch).
- [ ] Independent feedback (tach/Hall/current via ingress) verifies or refutes the
      effect; ack alone never means success.
- [ ] Acknowledged/no-motion → `verification_failed`; motion/no-ack → `unknown`
      then reconciled from feedback with no blind resend.
- [ ] Fan duty/lease bounds enforced independently of Tamoz at the effector
      boundary; the lease returns the fan to safe state.
- [ ] `make ci-check` green; a security/safety review (WP4 deliverable) is
      recorded.
