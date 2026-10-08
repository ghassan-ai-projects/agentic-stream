# U10 — Define the primary-hypothesis delta key (keep it)

Status: done · Decision: **keep the key, define it, remove the implementation from the plan** (revised for the experiment) · Priority: P2 · Size: S

## Finding

Trigger expressions can read `delta.primary_hypothesis_changed`
(`spec.DeltaKeys.PrimaryHypothesisChanged`). The cognition delta sets it to
`true` on a Situation's first reasoned version and to a constant `false`
afterwards (`internal/cognition/internal/domain/delta.go:49`: "not implemented
in this slice"). `documentation/overview/limitations.md` records the gap. The
design example `docs/design/examples/predictive-maintenance.situation.yaml:208`
(and the rotating-machinery example) use it in `materialDelta`.

A trigger that relies on the key never fires after the first version, and
nothing tells the author.

## Decision and reasoning

**Revised 2026-10-08 for the experiment constraint.** Keep the key and its
current behavior; remove only the *implementation* from the plan.

The first version of this task removed the key and rejected specs that use it.
That breaks the real-world-sensor experiment: `zone-thermal.situation.yaml`
uses `delta.primary_hypothesis_changed` in `materialDelta`, and editing the
example changes the compiled spec digest that the Arduino gateway allow-lists
([EXPERIMENT_COMPATIBILITY.md](../EXPERIMENT_COMPATIBILITY.md) E4, E5).

Implementing hypothesis tracking is still the wrong move:

- A Situation has no primary hypothesis. Hypotheses are Decision output
  (`decision-v1.json` `primary_hypothesis`), produced by the reasoner (Tamoz).
  Tracking "hypothesis changed" in the deterministic delta means feeding model
  output back into Situation state. Deterministic replay does not re-run the
  model, so Situation history would depend on recorded cognition and break the
  replay contract.
- Reconsideration after new evidence already exists in cognition
  (`correction.go`).

So the key stays as a documented constant: `true` on a Situation's first
reasoned version, `false` afterwards. In zone-thermal that is harmless: on the
first version `phase_changed` is also `true`, and afterwards the term is always
`false`.

## Steps

1. Replace the "not implemented in this slice" comment in
   `internal/cognition/internal/domain/delta.go` with the definition above.
2. Document the key's meaning in `documentation/contracts/situation-spec.md`
   and the cognition design page; remove the sentence from the limitations page
   that calls it unimplemented.
3. Add a cognition test pinning the definition (true first, false after).
4. Remove "primary-hypothesis change tracking" from any plan that lists it as
   future work, with a pointer here. If hypothesis-driven triggers are wanted
   later, that is a new design (Decision → Situation feedback with replay rules).

## Done when

- No code path changes; X01 passes unchanged; the zone-thermal digest is
  unchanged.
