# Phase 02 — Supervisory shadow (WP3 → Gate G3)

Implementation status: the paired replay shadow path and deterministic
baseline are implemented. The executable work breakdown and evidence bar are
in [PHASE-02-EXECUTION.md](PHASE-02-EXECUTION.md); G3 is not called green until
the repository gates and any required cross-repo evidence pass.

**Goal:** the deterministic baseline **and** Tamoz both produce recommendations
over the **same immutable Situations**, and **no live effector credential
exists**. This is the "shadow tournament" scaffolding on the Agentic Stream side.

**Program proof:** Experiment 4 (baseline vs Tamoz). **Allowed claim after G3:**
*Tamoz shadow decisions compared with a baseline*.

Agentic Stream owns the **shadow dispatch guarantee** and the **immutable
snapshot** both policies read. Tamoz (separate repo, already implemented) owns
the reasoning; the human-authored oracle and the comparison scorer live in the
Agent Research Lab. This phase makes sure the platform *cannot* leak a shadow
recommendation into a physical effect.

---

## Task 2.1 — Confirm and pin shadow semantics

`internal/spec/schema.json` already defines `executor.dispatchPolicy:
{active, shadow}` and `executor.riskCeiling`. Before writing new code, establish
by test what `shadow` currently guarantees:

1. Grep the consumers: `dispatchPolicy`, `shadow`, `riskCeiling` across
   `internal/cognition`, `internal/episodes`, `internal/decisions`,
   `internal/policy`, `internal/actions`.
2. Write a characterization test: deploy `zone-thermal` with
   `executor.dispatchPolicy: shadow`, drive a Situation that would otherwise emit
   a `select_thermal_mode` intent, and assert that **no command row and no outbox
   row** are created — the decision and intent are recorded, but nothing reaches
   the action plane.
3. If shadow does not already fully suppress command creation, fix it at the
   policy boundary (`internal/policy/policy.go`) so a shadow-scoped intent is
   recorded `shadowed` and never `approved`. This must be enforced independently
   of the effector, not by relying on the effector being a no-op.

**Exit:** a shadow-mode episode can propose intents that are durably recorded and
explainable, but provably never create a command/outbox row.

---

## Task 2.2 — Deterministic baseline as a shadow policy

Experiment 4 compares three policies over identical Situations: a deterministic
threshold/hysteresis baseline, Tamoz, and a human oracle. Agentic Stream should
provide the **deterministic baseline** as a first-class, non-model decision
source so the comparison is apples-to-apples (same Situation snapshot, same
decision schema).

Prefer to express the baseline with existing deterministic operators
(`internal/operators`: hysteresis, debounce, cooldown) feeding a rule that emits
the same typed decision shape Tamoz emits — not a second bespoke engine. Options,
in order of preference:

1. A spec-declared deterministic trigger/rule that yields a `select_thermal_mode`
   recommendation directly from features (no episode) — if the grammar supports
   it. Check whether cognition can route a trigger to a deterministic decision.
2. Failing that, a small in-repo "baseline executor" that consumes the same
   Situation snapshot the worker sees and writes a decision with a
   `source: deterministic_baseline` marker. Keep it in `internal/cognition` or a
   new narrow file; do not add an abstraction layer.

Both baseline and Tamoz decisions must reference the **same** `situation_id` +
`situation_version` so the scorer can align them 1:1.

**Exit:** for each trial Situation, both a deterministic decision and a Tamoz
decision exist against the identical immutable snapshot.

---

## Task 2.3 — Immutable-snapshot and abstention guarantees

- Assert (test) that the snapshot handed to the worker is byte-identical across
  the baseline read and the Tamoz read for the same version — no re-derivation,
  no clock drift. This is the fairness precondition of the tournament.
- Confirm `need_more_evidence` / abstention is a **first-class successful
  decision** in the decision schema (`internal/decisions`), not an error. If the
  schema cannot represent abstention, that is a decision-schema change to make
  here (reviewed), because Experiment 4 rewards correct abstention.
- Adversarial evidence: add a fixture where device data/metadata contains
  injected instruction-like text. Assert Agentic Stream treats it as evidence
  only and that a decision citing out-of-catalog operations is rejected by the
  intent-catalog validator (`internal/episodes/intent_catalog.go` +
  `internal/decisions/validator.go`). (Invariant 1.)

**Exit:** abstention is representable and rewarded; adversarial evidence cannot
become an instruction or an out-of-catalog intent.

---

## Gate G3 exit checklist

- [ ] Shadow mode records decisions/intents but provably creates no command or
      outbox row (test-proven, independent of the effector).
- [ ] Deterministic baseline and Tamoz decisions exist over identical snapshots
      with aligned `situation_id`/`version`.
- [ ] Snapshot is byte-identical across both reads (test-proven).
- [ ] Abstention is a first-class decision; correct abstention is scoreable.
- [ ] Adversarial evidence stays evidence; out-of-catalog intents fail closed.
- [ ] Still no live effector credential anywhere.
- [ ] `make ci-check` green.
