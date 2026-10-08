# X04 — One intent vocabulary for the thermal domain

Status: Agentic Stream part done (parity pin); Tamoz change pending · Decision: **Tamoz adopts the spec catalog; pin parity here** · Priority: P0 (experiment) · Size: S here, M in Tamoz

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md) G3, D2.

## Finding

Tamoz's `test/fixtures/domains/thermal-lab.json` declares
`request_bounded_cooling` (R2), `downgrade_cooling` and `withdraw_cooling` (R1).
Agentic Stream's zone-thermal catalog and the device capability catalog declare
`select_thermal_mode {mode: hold | bounded_cooling}` (R2). Tamoz drops proposals
that are not in the request catalog and demotes them to a watch, so the fan path
cannot fire.

## Steps

In **Tamoz** (owner approval needed; separate repository):

1. The thermal domain proposes `select_thermal_mode` with model-writable
   `mode`; `bounded_cooling` replaces `request_bounded_cooling`, and `hold`
   replaces downgrade and withdraw.
2. Update prompts, fixtures, the adversarial, tournament and evidence-gate
   evals, and the BAR/PLAN docs.
3. Note: `DecisionBuilder#preset_for` allows only a `default` preset; the
   zone-thermal intent sets `mode` as a model-writable field, which Tamoz
   supports. Keep it that way: the device catalog picks duty and lease from
   `mode`.

In **Agentic Stream**:

4. Add `TestThermalIntentCatalogDigestParity` next to the aquaculture parity
   test: the catalog compiled from the X05 specs must equal a pinned digest that
   Tamoz's suite also pins.

## Done when

A Tamoz thermal episode over a sustained rise proposes
`select_thermal_mode {mode: bounded_cooling}`, and the Agentic Decision
validator accepts it in X07's simulator rehearsal.
