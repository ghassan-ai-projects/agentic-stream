# Plan: Real-World Sensor HIL-0 (Agentic Stream slice)

Status: active implementation; partial through Phase 01 and Phase 03 Task 3.2;
HIL-0 gates are not yet met
Date: 2026-08-28
Source program: `agent-research-lab/real-world-sensor/round-2/`
Scope of THIS plan: **only the Agentic Stream changes** required to reach the
program's first release target, **HIL-0: one bounded verified effect**.

The executable quality bar and evidence ledger is
[QUALITY_BAR.md](QUALITY_BAR.md). This plan is the work breakdown; a checked
task or passing unit test is not, by itself, evidence of a physical HIL claim.

## 0. Read this first

This plan turns the Round 2 program (`round-2/README.md`,
`ARCHITECTURE_AND_PROTOCOLS.md`, `EXPERIMENTS.md`, `EXECUTION_PLAN.md`) into a
concrete, phased build for **this repository**. The program spans four repos;
this plan implements Agentic Stream's slice and defines the contracts it must
honor at the boundaries with the other three.

Ownership boundary (from `ARCHITECTURE_AND_PROTOCOLS.md` → *Required project
changes*):

| Concern | Repo | In this plan? |
|---|---|---|
| Governed serial-device effector, adapter config, thermal spec, reconciliation, action tests | **Agentic Stream** | **Yes — the whole plan** |
| Arduino firmware, safe state, watchdog, hard limits | firmware (out of repo) | No — contract only (§ boundary) |
| Edge gateway process, raw serial framing, device identity, clock mapping, delivery ledger | edge gateway (out of repo) | No — contract only (§ boundary) |
| Thermal supervisory objective, semantic decision schema, adversarial evidence | **Tamoz** (separate repo) | No — interface only |
| Serial-device emulator + protocol fault layer, effector oracle | **Streams Simulator** (separate repo) | No — interface only |

> The Tamoz worker side is already implemented per the user; this plan does not
> touch it. Agentic Stream sees Tamoz only through the existing worker protocol
> (`proto/agenticstream/runtime/v1/`) — no change is required there for HIL-0.

The ownership split is deliberate: Agentic Stream owns the governed
`actions.Effector` and a typed device-session boundary, while the edge gateway
owns raw serial bytes, framing, reconnect, device identity, and its delivery
ledger. Agentic Stream must not grow a second serial stack. In the emulator and
physical profiles, `DeviceTransport` is therefore the gateway link carrying the
versioned device records, not a direct Arduino/USB serial implementation.

## 1. What "done" means for the Agentic Stream slice

HIL-0 pass (`round-2/README.md` → *First release target*) requires, from
Agentic Stream specifically:

1. A real sensor observation opens a real Situation → **WP2 telemetry vertical**.
2. A deterministic baseline and Tamoz both produce **shadow** recommendations
   over the same immutable Situations → **WP3 shadow** (Agentic Stream provides
   the shadow dispatch path + snapshots; Tamoz provides the reasoning).
3. After promotion, exactly one governed command reaches one LED or fan → **WP4
   serial effector**.
4. The device rejects expired / malformed / out-of-range / wrong-boot / replayed
   commands → enforced by firmware+gateway; Agentic Stream must **materialize
   only bounded commands** and **treat receipt ≠ effect**.
5. Independent feedback verifies or refutes the effect → Agentic Stream ingests
   feedback as observations and drives verification/reconciliation.
6. A restart never causes a blind resend → existing outbox/lease + unknown-outcome
   semantics, extended with **authority epoch + reboot reconciliation barrier**.
7. E-stop / lease loss forces safe state → firmware owns it; Agentic Stream must
   never re-energize across an authority-epoch or boot-id change without
   reconciliation → **WP5**.
8. The whole run is explainable from durable evidence → **run manifest export**.

## 2. Phase map (what to build, in order)

Build by evidence dependency, exactly as `EXECUTION_PLAN.md` prescribes. Each
phase file is a self-contained task list with file paths, interfaces, tests, and
its acceptance gate.

| Phase file | Program WP / gate | Agentic Stream deliverable |
|---|---|---|
| [01-telemetry-vertical.md](01-telemetry-vertical.md) | WP2 / G2 | Thermal event schemas + compiler-valid `zone_thermal` spec + observation ingress with quality/time. No model, no actuator. |
| [02-shadow-path.md](02-shadow-path.md) | WP3 / G3 | Verify/extend `dispatchPolicy: shadow` so baseline + Tamoz run over identical Situations with no live credential present. |
| [03-serial-effector.md](03-serial-effector.md) | WP4 / G4a–G4b | `serial-device` effector behind `actions.Effector`; deterministic intent→command materializer; receipt/result/observation/verification split; unknown-outcome reconciliation; physical deployment profile; protocol/action metrics. |
| [04-authority-and-soak.md](04-authority-and-soak.md) | WP5 / G4c–G5 | Authority epoch + boot-id fencing; reboot reconciliation barrier; safe-stop priority; evidence/run manifest export; soak-supporting counters. |
| [05-tests-and-gates.md](05-tests-and-gates.md) | all (M1–M3) | Validation matrix mapped to concrete Go tests; per-gate exit checklists. |

**HIL-0 (M1–M3) is Phases 01–05 above** — the fully-detailed, start-now work.
Phases 06–09 below are **longer-horizon skeletons** for the later maturity levels
(M4–M6); they fix the shape and invariants but are intentionally lighter, and
each carries a "do not start until…" guard tied to a Round 2 checkpoint. See § 6a.

| Phase file | Program WP / gate | Agentic Stream deliverable |
|---|---|---|
| [06-ecosystem-adapters.md](06-ecosystem-adapters.md) | Ph6 / P6 (M4) | Adapter boundary (ingress + egress); two gateway ecosystems (MQTT/ESPHome, then EdgeX/Modbus/OPC-UA) over the same closed contract. |
| [07-productization.md](07-productization.md) | Ph7 / P7 (M4.5) | Physical-path operator read APIs + explicit approve/reconcile/safe-disable ops; preflight, health, packaging, compatibility matrix. |
| [08-pilots.md](08-pilots.md) | Ph8 / P8 (M5) | Pilot evidence pipeline + longitudinal hardening for two supervised non-critical pilots. |
| [09-production-1.0.md](09-production-1.0.md) | Ph9 (M6) | Fleet identity/authority lifecycle, upgrade/rollback, independent assurance, the 1.0 bar, cross-cutting workstreams. |

Sequencing rule: **do not start a phase until the prior phase's gate is green.**
Passing a lower validation level never substitutes for a higher one. The M4–M6
phases additionally gate on the Round 2 investment checkpoints (A after Phase 02,
B after Phase 04, C after Phase 06, D after Phase 08).

## 3. Non-negotiable design constraints (from AGENTS.md + the program)

These are release-blocking. Every phase reasserts the relevant ones.

- **Agentic Stream is supervisory, never real-time.** No PWM timing, debounce,
  emergency shutdown, or fast loop in this repo. If loss of the L2 connection can
  make the rig unsafe, the design is wrong.
- **The model never names a raw target, pin, opcode, or PWM value.** The model
  proposes a *semantic* intent; deterministic policy materializes the concrete,
  bounded device command via the spec's `presets` + `modelWritableFields`
  (see § 4). This is invariant 1 (untrusted content is evidence, not
  instructions) applied to actuation.
- **Receipt ≠ result ≠ observation ≠ verification.** Never let `ack: true` stand
  for physical success. An acknowledgement is command receipt only; the effect is
  proven by an independent feedback observation evaluated against the command's
  expected effect and deadline.
- **Ambiguity stops.** A timeout after possible delivery is `outcome_unknown`;
  the system reconciles before any retry. This maps directly onto the existing
  `actions.UnknownOutcomeError` / `ReconcileUnknown` path — reuse it, do not
  reinvent it.
- **Deterministic replay is preserved.** Replay never performs external effects
  (`internal/replay` invariant). The serial effector must be inert under replay
  and under the shadow profile.
- **No new top-level dependency without justification.** Prefer stdlib. A serial
  transport library, if truly needed for real hardware, is introduced only in
  Phase 03 and gated behind the physical deployment profile; the emulator path
  needs none.
- **Domain data is JSON, not Go literals.** New event schemas go in
  `internal/eventschema/registry_data.json` (updating the pinned golden digest in
  `internal/eventschema/registry_test.go` as a deliberate reviewed change), never
  as Go literals (AGENTS.md → *Forbidden Changes*).

## 4. The materialization mechanism (why the model can't set PWM)

The Round 1 starter YAML
(`round-1/temp-regulation/starters/temp-regulation.situation.yaml`) invented
fields (`schema:`, `supervisor:`, `expectedFeedback:`, `idempotencyKeyTemplate:`)
that **do not exist** in the compiler and will fail validation. This plan does
**not** copy them. The real grammar already expresses the safety envelope:

- `intents[].parameterSchema` — the full JSON Schema of the intent's parameters
  (`type: object`, `additionalProperties: false` required); carried and digested
  verbatim by `internal/episodes/intent_catalog.go`.
- `intents[].modelWritableFields` — the *only* parameter names the model may
  write. Everything else is fixed.
- `intents[].presets` — named, bounded parameter sets the deterministic policy
  materializes. A fan duty ceiling is a preset value, not a model output.
- `intents[].risk` (`R1` LED / `R2` fan) + `intents[].policy`
  (`automatic`/`approval`) — the policy plane (`internal/policy/policy.go`)
  enforces approval for `R2` and revalidates at dispatch.

So "the model requests a mode, policy clamps the parameters" is already the
system's shape: the model may only touch `modelWritableFields` (e.g. a mode
enum), and the concrete bounded command (`duty_permille`, `lease_ms`) comes from
a `preset`. The serial effector then translates that already-bounded command into
device wire bytes and enforces the bound **again** at the last boundary
(defense in depth). Phase 03 details this.

The thermal profile uses an explicit configuration binding from the semantic
zone (`zone-01`) to each physical target (`led-01` or `fan-01`). The model and
policy may select the semantic entity; only the closed capability catalog may
resolve it to a physical target.

## 5. Decision log to close before coding (owner input required)

`EXECUTION_PLAN.md` requires these choices recorded before implementation. The
ones that shape Agentic Stream code are starred; capture answers in
`docs/plans/real-world-sensor-hil/DECISIONS.md` (create it when answered):

1. Exact board + logic voltage (drives the observation unit/quality fields). ★
2. Exact first sensor + first actuator (LED then 5 V fan). ★
3. Safe state for each output (the effector's declared safe command). ★
4. Feedback mechanism + its independence (tach / Hall / current). ★
5. Does dedup survive MCU reset? If not, the reboot **reconciliation barrier**
   design in Phase 04 is mandatory. ★
6. NDJSON prototype vs COBS/CBOR HIL frames — the effector serializer targets
   NDJSON first (Phase 03), framed binary is a later swap behind the same
   interface. ★
7. Serial library + port-ownership model (only relevant when real hardware is
   attached; emulator path is a UDS/pipe). ★
8. Command lease + watchdog deadlines (become effector config bounds). ★
9. Location of the shared protocol contract (see § 6). ★
10. Owner authorization for real low-voltage actuation (blocks G4a).

Until answered, Phases 01–02 (no actuator, no model credential) can proceed;
Phase 03 needs at least 2, 3, 6, 8, 9, 10.

## 6. Shared protocol contract (one source of truth)

`EXECUTION_PLAN.md`: *"Keep cross-repo contracts in one versioned source of
truth… Do not hand-edit five divergent JSON schemas."* Agentic Stream already
holds versioned JSON Schemas under `internal/contractsv1/schemas/v1/`. The device
wire records (observation, command, receipt, result, state) get **new** schemas
here, and the gateway/emulator/firmware repos consume copies via generated
conformance fixtures. Field-naming convention is fixed and intentional:

- **device wire records** use `snake_case` (`message_id`, `boot_id`,
  `idempotency_key`, `expires_after_ms`);
- **SituationSpec** uses `camelCase` (`parameterSchema`, `eventType`).

Do not normalize one to the other. Phase 01 adds the observation schema; Phase 03
adds the command/receipt/result/state schemas.

## 6a. Maturity coverage (where this plan stops)

This directory plans the Agentic Stream repo across the **whole** maturity ladder
(**M0.5 → M6**), but at two very different resolutions. **M1–M3 (Phases 01–05) is
the fully-detailed, start-now HIL-0 plan** — the Round 2 target
(`MATURITY_ROADMAP.md`: *"The immediate goal is not production. It is M2…"*).
**M4–M6 (Phases 06–09) are deliberately lighter skeletons**: they fix the shape,
the boundaries, and the invariants each level must not break, but their concrete
tasks are meant to be filled in against real evidence, because planning them in
full now would be assumption-driven ahead of the hardware inventory and the
Checkpoint-A decision on whether Tamoz beats the deterministic baseline. Each
carries an explicit "do not start until…" guard.

| Maturity | Roadmap phase | Agentic Stream priority | This plan | Resolution |
|---|---|---|---|---|
| M1 | Ph2 real telemetry | 1. compiler-valid physical Situation | ✅ [Phase 01](01-telemetry-vertical.md) | detailed |
| M1.3 | Ph3 shadow | (shadow dispatch + baseline) | ✅ [Phase 02](02-shadow-path.md) | detailed |
| M2 | Ph4 HIL-0 | 2. serial effector + materializer | ✅ [Phase 03](03-serial-effector.md) | detailed |
| M2/M3 | Ph4–5 | 3. verification/reconciliation projection | ✅ [Phase 04](04-authority-and-soak.md) | detailed |
| M3 | Ph5 fault-qualified | (soak counters + evidence manifest) | ✅ [Phase 04](04-authority-and-soak.md) | detailed |
| M4 | Ph6 ecosystem | 5. adapter conformance interface | 🟨 [Phase 06](06-ecosystem-adapters.md) | skeleton |
| M4.5 | Ph7 productization | 4. physical-path operator APIs | 🟨 [Phase 07](07-productization.md) | skeleton |
| M5 | Ph8 pilots | — | 🟨 [Phase 08](08-pilots.md) | skeleton |
| M6 | Ph9 1.0 | 6. multi-device isolation + packaging | 🟨 [Phase 09](09-production-1.0.md) | skeleton |

The Agentic Stream slice of the roadmap's **Phase 1** (device protocol + emulator,
M0.7) is not a separate phase file here — its repo-local work (wire schemas, the
emulator-facing `DeviceTransport` adapter skeleton, codec fuzz, safe-stop
priority, "no live port in shadow") is folded into Phases 03–04 where it is first
needed. The standalone protocol emulator itself is a Streams Simulator
deliverable, not Agentic Stream.

**Treat the M4–M6 skeletons as guardrails, not marching orders.** Before starting
any of them, re-open the corresponding roadmap phase with real M3 evidence in hand
and flesh the tasks out — and honour the checkpoint guards (B before Phase 06, C
before Phase 08, A before pilots, D before 1.0).

## 7. Explicit non-goals for HIL-0

Not in scope (and must not be silently added): MQTT/Modbus/CAN/BLE/Sparkplug/OPC
UA adapters (program Round 3+), R3/R4 autonomous action (advisory-only), any
mains/heat/pressure/motion/medical/vehicle control, a web UI, a graph engine, or
per-event LLM calls. Never generalize a passed gate to production, certification,
arbitrary devices, or exactly-once effects.
