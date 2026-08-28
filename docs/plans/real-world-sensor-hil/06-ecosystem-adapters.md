# Phase 06 — Ecosystem compatibility (M4 → roadmap Phase 6)

> **Longer-horizon skeleton.** Do **not** start this until Gate G5 (M3) is green
> and Round 2 **Checkpoint B** is cleared ("Can the reference rig sustain the
> safety invariants under faults?"). Detailed design here would be
> assumption-driven ahead of that evidence. This file fixes the *shape* of the
> work and the invariants it must not break; the concrete tasks get filled in
> when the M3 evidence exists.

**Goal:** prove the supervisory layer is **not coupled to one Arduino path** —
the same Situation/Decision/Command/Outcome lifecycle runs against two different
external gateway ecosystems through the **same closed capability contract**.

**Allowed claim after Gate P6:** *the supervisory layer is portable across the
tested accessible and industrial gateway paths.* Never: "supports protocol X"
generally, or "is a device cloud / SCADA / gateway platform."

The critical-path warning from `MATURITY_ROADMAP.md` governs this phase:
*"Additional protocols and UI do not shorten the critical path."* Adapters are
breadth, not depth — they must reuse the M3 proof, never re-open it.

---

## The one rule that makes or breaks M4

**No device-specific detail may leak into core code.** No MQTT topic, Modbus
register, OPC-UA node id, ESPHome entity, pin, or vendor payload field may appear
in `internal/situations`, `internal/cognition`, `internal/episodes`,
`internal/decisions`, `internal/policy`, or `internal/actions`. Everything
device-specific lives **below the adapter boundary** and is mapped up into the
existing typed contracts (event schemas + intent catalog).

Test this negatively and keep it green forever:

- A CI grep/lint asserting the core packages contain no protocol/register/topic
  literals.
- Removing either adapter leaves the entire M0–M3 proof suite green
  (roadmap Exit gate P6).

---

## Task 6.1 — Extract the adapter boundary (both directions)

Two boundaries already exist in embryonic form; formalize them into stable,
versioned interfaces:

- **Ingress adapter** (telemetry in) — today `internal/ingress` has `JSONLReplay`
  and the simulator. Define an `ingress.Adapter` contract that any ecosystem
  connector implements: it produces the same normalized `contractsv1.Envelope`
  stream, with explicit quality/time/uncertainty, from a vendor source. Vendor
  quality/time limitations must be made **explicit**, never silently upgraded to
  "valid/precise."
- **Egress adapter** (commands out) — today the effect boundary is
  `actions.Effector` + the `DeviceTransport` from Phase 03. A new ecosystem
  effector implements `AuthorizedEffector` and maps a materialized, already-bounded
  `actions.Command` onto the vendor command surface. **Gateway acknowledgement is
  never confused with plant effect** — the receipt/result/observation/verification
  split from Phase 03 Task 3.5 is mandatory for every adapter, and independent
  feedback still comes through ingress, not the egress ack.

Both adapters map onto a **closed capability profile** per device: discovery of an
external sensor/actuator produces a fixed allow-list of typed quantities and
operations. No arbitrary register/tag/topic write ever reaches the model-facing
intent catalog (`internal/episodes/intent_catalog.go`).

**Exit:** a documented `Adapter` / ecosystem-effector interface with conformance
fixtures; the reference serial path from Phase 01/03 is re-expressed as the first
implementer of these interfaces without behavior change.

---

## Task 6.2 — Integration A: accessible device ecosystem

Preferred path (roadmap): **ESPHome or a small MQTT edge gateway** (MQTT 5, per
`ARCHITECTURE_AND_PROTOCOLS.md` protocol table — the default network transport
after USB). Agentic Stream connects **through a gateway**, never to a bare
broker as a control bus.

Prove:

- external sensor/actuator discovery maps to a closed capability profile;
- telemetry quality/time limitations are explicit in the ingested observations;
- device unavailability and reconnects preserve the action invariants (unknown
  outcome → reconcile, no blind resend, safe-state on authority/lease loss);
- Tamoz and the core remain independent of vendor-specific payloads above the
  adapter.

Do **not** assume MQTT QoS 2 == exactly-once actuation; idempotency + verification
still own correctness (protocol table caveat).

**Exit:** the thermal Situation/Decision/verification loop runs end to end over
the MQTT/ESPHome path with the M3 invariants intact.

---

## Task 6.3 — Integration B: industrial gateway ecosystem

Preferred path (roadmap): **EdgeX, ThingsBoard Gateway, or an OPC-UA/Modbus
simulator behind a gateway.**

Prove:

- register/tag metadata maps to typed quantities and capabilities (no raw
  register semantics in core);
- gateway acknowledgement is not confused with plant effect;
- authentication and device identity are reviewed (network deployment needs
  authenticated channels + device credentials — USB's physical trust boundary no
  longer applies);
- no arbitrary register/tag write reaches the model-facing catalog;
- the same semantic Decision and outcome lifecycle work across both integrations.

**Exit:** the same core proof runs over the industrial path; auth/identity review
recorded.

---

## Gate P6 exit checklist

- [ ] The same Situation/Decision concept runs against two different gateway
      ecosystems.
- [ ] No core package contains device-specific registers, topics, node ids, or
      pins (CI-enforced).
- [ ] Adapter conformance fixtures and compatibility versions are published.
- [ ] Removing either adapter leaves the core proof suite green.
- [ ] Protocol integration code is materially smaller than the core
      governance/evidence layer.
- [ ] `make ci-check` green.

**Checkpoint C (after P6):** is the core portable without becoming a protocol
platform? If integration demanded device-specific core code, stop and redesign
the adapter boundary before M5.
