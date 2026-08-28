# Phase 04 — Recovery, authority, and soak (WP5 → Gates G4c, G5)

**Goal:** the bounded loop stays safe and explainable when authority is lost,
when deliveries race a superseding Situation, and over an 8-hour run.

**Program proofs:** Experiment 7 (authority loss), Experiment 8 (stale race &
reconciliation), Experiment 9 (human override & local interlock), Experiment 10
(HIL chaos soak).
**Allowed claims:** after G4c — *one governed low-voltage physical loop verified,
including authority loss, reconciliation, and e-stop (maturity M2)*; after G5 —
*the reference rig survives the declared faults over an 8-hour soak (maturity
M3)*. **Do not** claim a verified loop before G4c, or fault-qualification before
G5.

Much of this reuses primitives that already exist — the work is wiring them to
the serial boundary and proving the compositions under fault.

---

## Task 4.1 — Authority epoch + boot-id fencing at the serial boundary

Two independent fences must both hold before any energizing command is sent:

- **Runtime authority epoch** — already implemented as
  `storage.EpochControl` (`Kill`/`Drain`/`State`/`AssertAdmission`/
  `AssertDecision`) and `storage.RuntimeOwner`. The dispatcher already asserts the
  runtime owner (`dispatcher.assertRuntimeOwner`). "authority_epoch changes when
  the live controller changes or is killed" **is** this runtime epoch — reuse it,
  do not add a parallel concept.
- **Device boot id** — device-side, delivered by the handshake (Phase 03 Task
  3.4). Each command carries `expected_boot_id`; the effector rejects a command
  whose `expected_boot_id` ≠ the live session boot id.

Wire both into the serial effector's pre-send check (Phase 03 Task 3.3 step 3):

1. On `EpochControl.Kill`/`Drain` of the owning epoch, in-flight and queued
   serial sends must stop; only the safe-stop path (Task 4.3) may proceed.
2. On a device reboot (`boot_id` change observed at handshake), enter the
   reconciliation barrier (Task 4.2) before any energizing command resumes.
3. Split-brain: only one authority epoch may command a target. A second gateway
   / second controller epoch attempting to command the same target is rejected
   (Experiment 7 pass condition). The runtime owner lease already enforces
   single-writer; add the target-scoped assertion so a rejected second epoch is
   observable, not merely lost.

**Exit:** Experiment 7 trials pass — killing Tamoz / Agentic Stream / gateway, or
unplugging USB, causes a bounded safe transition (the MCU lease expires to safe
state; Agentic Stream never re-energizes across the epoch/boot change without
reconciliation); a second epoch cannot also command the target.

---

## Task 4.2 — Reboot reconciliation barrier

If the device dedup ledger does **not** survive MCU reset (decision-log item 5),
a reboot must create an explicit **reconciliation barrier** before energizing
commands resume (`ARCHITECTURE_AND_PROTOCOLS.md` → *Device command state
machine*).

Implement as a serial-boundary gate:

- On a new `boot_id`, mark the target `reconciling`. While reconciling, the
  effector refuses energizing commands and instead issues a **state query**
  (`device.state/1`) to learn the device's actual current output.
- Any `outcome_unknown` command for that target is resolved via
  `Dispatcher.ReconcileUnknown` using the queried device state + independent
  feedback as evidence — `succeeded`, `failed`, or `manual_review`. Only after
  the barrier clears may new energizing commands flow.
- Safe-stop commands are **never** blocked behind the barrier (Task 4.3).

**Exit:** Experiment 8 trials pass — supersession before/after dispatch, duplicate
delivery before/after reboot, lost acks, and delayed old frames after reconnect
produce **zero stale energizing effects** and **zero duplicate net energizing
effects**; every ambiguous branch terminates in verified / failed / safe-expired
/ manual-review with evidence.

---

## Task 4.3 — Safe-stop priority path

A safe-state command (e-stop-driven, watchdog, lease loss) uses a **dedicated
path that must not queue behind ordinary work** (`ARCHITECTURE_AND_PROTOCOLS.md`).

- Add a priority lane in the serial effector / session for safe-stop frames that
  bypasses the ordinary command queue and the reconciliation barrier.
- Authority ordering is deterministic and must be encoded/tested:
  **hard interlock > e-stop > local safety > current approved command >
  supervisory recommendation** (Experiment 9). The upper three are firmware-owned;
  Agentic Stream must never emit anything that can override them, and software
  must never be able to clear a physical e-stop remotely.
- `raise_manual_review` (R2, approval) is the escalation when the deterministic
  bound clamps a proposal the model insists on, or on `over_ceiling`.

**Exit:** Experiment 9 trials pass — exact approved bytes/digest dispatched; stale
or mutated approvals fail; e-stop always wins and must be cleared locally; no
output while interlocked; software cannot clear a physical e-stop.

---

## Task 4.4 — Evidence / run manifest export

Every execution must produce one immutable run artifact
(`EXECUTION_PLAN.md` → *Run artifact*). Agentic Stream owns the durable ledgers;
add an exporter (CLI subcommand, e.g. `agentic-stream export-run`) that writes the
run directory:

- `manifest.json` — git commit + dirty state per repo, board/device/boot/firmware
  identity, schema + capability digest, spec + policy digest, Tamoz
  prompt/decision-schema/provider/model/sampling (from the worker manifest),
  calibration + wiring revision, scenario seed + wall/monotonic clock mapping,
  start/end + operator identity, safety-review reference, pass/fail + known blind
  spots.
- The ledger exports Agentic Stream already stores: `observations.jsonl`,
  `situations.jsonl`, `decisions.jsonl`, `commands.jsonl`,
  `device-results.jsonl`, `feedback.jsonl`, plus `spec.canonical.json`,
  `policy.canonical.json`, `metrics.json`, `verdict.json`, and `checksums.sha256`.
- Everything binds by digest so the artifact verifies independently.

Keep this an **export of existing durable records**, not a new write path in the
hot loop.

**Exit:** a completed experiment produces a verifying run directory; re-running
the checksum verifier passes.

---

## Task 4.5 — Soak-supporting counters (Gate G5)

Expose the zero-tolerance counters (`EXPERIMENTS.md` → Experiment 10) as durable,
queryable metrics so an 8-hour run can be judged:

- unsafe output count, stale energizing effect count, duplicate net energizing
  effect count, unexplained actuator transition count, false verified-success
  count, safe-state deadline misses — **all must be 0**;
- evidence completeness = 100% for every physical transition;
- report-only: p50/p95/p99 stage latency, reconciliation duration, parser errors,
  MCU resets/brownouts, memory high-water marks, decision/model cost,
  baseline-vs-Tamoz disagreement.

The physical soak itself runs in the lab (out of repo); Agentic Stream must make
every counter derivable from its durable ledgers, and add a `verdict`
computation that fails if any zero-tolerance counter is non-zero.

**Exit:** the counters compute from ledgers; a synthetic fault schedule in
emulation drives each counter and the verdict logic distinguishes pass from fail.

---

## Gate G4c exit checklist (maturity M2)

- [ ] Authority-epoch + boot-id fences both enforced before any energizing send.
- [ ] Killing any upstream component causes a bounded safe transition; only one
      epoch commands a target.
- [ ] Reboot reconciliation barrier prevents stale/duplicate energizing effects;
      every ambiguous branch terminates with evidence.
- [ ] Safe-stop bypasses ordinary work and the barrier; authority ordering
      test-proven; e-stop cannot be cleared by software.
- [ ] Run manifest exports and verifies.
- [ ] `make ci-check` green; safety/security review updated.

## Gate G5 exit checklist (maturity M3)

- [ ] Every zero-tolerance counter derivable from ledgers and = 0 under the
      emulated fault schedule.
- [ ] Evidence completeness = 100% for every physical transition.
- [ ] The 8-hour lab soak's run artifact verifies (lab-executed; Agentic Stream
      provides the ledgers, counters, and verdict).
- [ ] No zero-tolerance counter generalized beyond "this rig survived the declared
      faults over 8 hours." Not production, not certification, not exactly-once.
