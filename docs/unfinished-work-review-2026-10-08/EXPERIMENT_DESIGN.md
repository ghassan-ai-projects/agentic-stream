# Design: the real-world-sensor closed loop

Status: D1 accepted as ADR-018 and implemented (X03, X08, X09) on 2026-10-08; D2–D6 open (X04–X07).
Scope: what Agentic Stream, Tamoz, Streams Simulator and the bench gateway must
each do so that the experiment's closed loop works live, not only in a paused
test feed. The experiment is the product's core proof, so this design comes
before every other task in this review.

## The loop

```text
DHT11 ─▶ Mega firmware ─USB─▶ gateway (serial owner) ─UDS─▶ live socket
  ─▶ event log ─▶ engine ─▶ Situation zone_over_temp (versions)
  ─▶ cognition trigger (materialDelta) ─▶ episode ─gRPC─▶ Tamoz worker
  ─▶ Decision + Intent select_thermal_mode{mode: bounded_cooling} (R2)
  ─▶ policy ─▶ approval request ─SSE─▶ Tamoz approval relay ─▶ human signs
  ─▶ /v1/approvals ─▶ policy approves ─▶ command (catalog preset 450‰ / 5 s)
  ─UDS device-wire v1─▶ gateway (policy + capability digest allow-list)
  ─▶ board: receipt ▸ result ▸ query_state ─▶ outcome + verification
  ─▶ (independent feedback as evidence) ─▶ export-run ▸ verify-run
```

## What is proven and what is not

| Hop | Evidence today | Gap |
| --- | --- | --- |
| Sensor → live socket → event log | Physical: 8 + 8 DHT11 events, zero quarantine | — |
| Event log → Situation | Simulator: 22 versions, 1 Situation | **Bench cannot open the Situation** (G1 below) |
| Situation → episode → Tamoz | Simulator pass2: one Tamoz decision | — |
| Decision → intent → dispatch, **R1 LED** | Simulator pass2: `set_indicator` executed, verified | Passed only because the feed **paused** for 20 s (G2) |
| Decision → intent, **R2 fan** | Never | Vocabulary (G3), risk ceiling (G4), approval (G5), freshness (G2) |
| Command → board → state | Physical: direct serial, operator saw rotation | Not through Agentic Stream on the bench (G1, G6) |
| Effect → independent feedback | Operator observation only | No instrument (D6) |

## Gaps found

**G1. The bench cannot open a Situation.** zone-thermal opens only when
`features.ambient_count_5m >= 2` and gates every transition on
`ambient_slope_5m`. The bench has no ambient sensor (temperature and humidity
only), so the Situation never opens and nothing downstream can happen. The spec
was written for the simulator's four-sensor world.

**G2. Window completeness flips make pending intents stale.** *(Corrected
2026-10-08 from X01's continuous-feed test.)* Policy evaluation
(`policy/internal/domain/routing.go:33`), approval resolution (`routing.go:62`)
and dispatch authorization (`actions/internal/domain/authorization.go:107`)
require `current_version == intent.situation_version`. The engine publishes a
version only on a lifecycle change (open, close, phase transition) or a
completeness change (`situations/internal/domain/evaluate.go`), not for every
reading. But with all sources streaming, every window slide flips completeness
`provisional → on_time` and publishes **two versions** with the same phase and
severity: pass 2's versions 8–22 are exactly these pairs, one pair per 30 s
slide. An intent therefore goes stale if a slide happens before it dispatches:

- for R2, a human approval almost always takes longer than the time to the next
  slide, so the fan path resolves `stale`;
- for R1 it is hidden today only because the episode blocks ingestion (G9);
  once that is fixed, any slide inside an episode stales its intent.

TECHNICAL_DESIGN §11.5 already anticipates this: "Version 1 defaults to
rejection unless compatibility is explicitly declared." D1 is that declaration.

**G3. Intent vocabulary seam.** Tamoz's thermal domain proposes
`request_bounded_cooling` (and `downgrade_cooling`, `withdraw_cooling`). The spec
and the capability catalog define `select_thermal_mode {mode: hold |
bounded_cooling}`. Tamoz's `DecisionBuilder#admissible_entry` drops a proposal
that is not in the request's catalog and demotes it to `install_watch_condition`.
The fan intent can therefore never be proposed; it silently becomes a watch.

**G4. Risk ceiling.** The executor `riskCeiling` defaults to `R1`
(`spec/internal/domain/normalize.go:44`), and the runbook's spec copy sets only
`name: tamoz` and `dispatchPolicy: active`. `select_thermal_mode` is R2, so it
is above the episode's ceiling: Tamoz demotes it, and the Decision validator
would reject it anyway.

**G5. R2 approval cannot complete.** R2 always needs human approval (the
calibration route has no writer; U24). Approval needs provisioned principals,
which no command can create (U15), and with G2 the approval resolves `stale`
anyway.

**G6. Entity binding mismatch on the bench.** The DHT11 mapping sets
`entity_id: zone-1`; the capability catalog binds `zone-01 → fan-01` /
`led-01`. The materializer refuses unbound targets (`device/internal/domain/materialize.go:68`),
so a bench command fails closed. The simulator path uses `zone-01`.

**G7. Hand-edited spec copies.** Each run copies the example and edits it by
hand. That caused B1 (saved copies no longer validate) and G4 (missing ceiling),
and the gateway's policy-digest allow-list must follow whatever was edited.

**G8. The live pipeline only advanced when an event arrived.** Found by
X01's end-to-end test. Due cognition, approved commands and silence timers
waited for the next event, so a quiet feed stalled the loop and a dead link
was never detected. Fixed in [X08](tasks/X08-live-pipeline-clock.md).

**G9. Episodes run inside the ingest batch.** `runner.RunOnce` calls the
worker synchronously from `advanceBatch`, which runs for each ingested event.
While Tamoz reasons (up to the spec's 30 s wall time), ingestion, the stream
engine, silence timers, phase transitions and dispatch all stop, and readings
back up in the bounded socket queue until the gateway blocks. This contradicts
TECHNICAL_DESIGN §11.4 ("preserve ingress, evidence log, deterministic state")
and makes §11.5 impossible live: a running episode cannot be cancelled by a
material supersession, because no new version is computed while it runs. That
MVP item ("cancel a stale episode after material supersession") holds only in
replay. On the bench it means an over-temperature escalation or a dead link is
not processed while the model thinks. Fix: [X09](tasks/X09-episodes-beside-ingestion.md).

**G10. Every engine run re-read the whole event log.** Found while deleting
U04: the cost of each live advance grew with the log, which a long bench soak
would hit. Fixed in [X10](tasks/X10-engine-resumes-from-applied-position.md).

## Decisions

### D1 — Material freshness: declared compatibility (Agentic Stream core; new ADR)

**Decision.** An intent is fresh while no **material** Situation change has
happened since the version it was reasoned on. "Material" is the rule cognition
already uses: an admitted evaluation of the producing trigger. When cognition
admits a new version, `supersedePending` additionally marks the older versions'
open intents `superseded`, reusing the approval-withdrawal path it already has.
Policy evaluation, approval resolution and dispatch authorization replace
`current_version == intent.version` with:

1. the intent is not superseded, withdrawn or expired;
2. the Situation occurrence is still open (not resolved or closed);
3. source health and completeness still pass (unchanged check);
4. plus every existing check: approval signature and expiry, rate limit,
   interlock, owner epoch, device authority and boot.

**Why this is right.**

- Invariant 7 requires revalidation "against current state", not version
  equality. The current state now includes cognition's durable judgment of
  materiality, which is deterministic, replayable and recorded in
  `trigger_evaluations` (invariant 10).
- The spec author already defines materiality per trigger (`phase_changed ||
  severity_change >= 10` in zone-thermal). A temperature reading that moves the
  mean by 0.1 °C should not cancel a cooling request; a phase change to
  `recovering` must, and does.
- One rule replaces two contradictory ones. Without it, Situation churn is a
  denial of service on every governed action.
- Physical safety does not rest on this check alone: the firmware enforces
  bounds and the lease, the gateway allow-lists digests, and the catalog caps
  duty at 600‰ and lease at 10 s.

**Rejected alternatives.**

- *Keep version equality and require quiet feeds*: impossible with a live
  sensor; it is the current failure.
- *Publish versions only on material change*: snapshots would carry stale
  facts, and provenance per reading is lost.
- *A wall-clock grace period*: not deterministic in replay, and an arbitrary
  number on a safety path.

**Experiment impact.** The zone-thermal digest is unchanged (no spec change).
Keep the policy document unchanged as well: freshness is enforced in code and
recorded per evaluation, so adding a field to the document would change the
policy digest every gateway allow-lists for no safety gain.
X01 must gain a test that feeds continuously while an approval is pending.

### D2 — The spec's intent catalog is the only intent vocabulary

**Decision.** Tamoz adopts `select_thermal_mode {mode: hold | bounded_cooling}`.
`request_bounded_cooling` becomes `mode: bounded_cooling`; `downgrade_cooling`
and `withdraw_cooling` become `mode: hold`. No conversion table anywhere.

**Why.** The catalog is digest-bound across the worker protocol and is the
authority the Decision validator enforces (invariant 6). A translation layer
would be an unreviewed authority mapping, which is exactly the "conversion
boundary" CURRENT-STATUS flags. Renaming on the Agentic side instead would
change the capability catalog. The catalog's digest (`0d6122…`) is embedded in
the firmware the bench runs, so that path needs a firmware rebuild and
re-qualification. The Tamoz change touches only reasoning prompts, fixtures and
evals.

**Guard.** A thermal intent-catalog parity pin on both sides, like the
aquaculture `TestAquacultureIntentCatalogDigestParity`.

### D3 — The experiment's specs are checked in and pinned

**Decision.** Add `examples/real-world-sensor/` to Agentic Stream with the two
specs the experiment runs, so nobody edits copies by hand:

- `zone-thermal-sim.situation.yaml`: the simulator run (four sensors), Tamoz,
  `dispatchPolicy: active`, `riskCeiling: R2`.
- `zone-thermal-bench.situation.yaml`: the physical bench. Inputs temperature,
  humidity and heartbeat. No ambient guard, because there is no ambient sensor
  and the ambient discount is a simulator-world feature; this is stated in the
  file. Same intents, presets and risks as the sim spec. Thresholds documented
  with how to drive them on the bench (heat source on the sensor).

The design example `zone-thermal.situation.yaml` stays the safe default
(`native`, shadow). X01 validates all three, pins both experiment digests, and
`validate` prints the policy digest (the value the gateway allow-lists, see
E5) so the runbook never computes or hand-types it.

**Why.** It fixes G1, G4 and G7 in one place and puts the experiment's real
inputs under this repository's CI.

### D4 — Bench mapping uses the catalog's entity ids, and the gateway emits a heartbeat

**Decision (experiment repo).** `arduino-mega-dht11.mapping.json` maps both
sensors to `entity_id: zone-01`. The gateway emits `zone.heartbeat.observed`
for every board state or telemetry frame, so `missing_heartbeat` detects a dead
link on the bench.

**Why.** Changing the catalog's binding would change the capability digest the
firmware embeds. The mapping file is gateway configuration and changes freely.
Without a heartbeat, a dead USB link looks like a quiet room.

### D5 — R2 stays human-approved on the bench

**Decision.** Keep human approval for the fan: provision principals with
`principals apply` (U15) before `serve` starts, run Tamoz's approval relay
against `/v1/approvals`, and set an approval expiry that covers real human
latency. With D1 an approval survives non-material versions. Calibrated
automation is removed (U24), not resurrected for the bench.

**Why.** Physical actuation needs explicit owner authorization (HIL DECISIONS
item 10). The approval relay is already proven in Tamoz; only provisioning and
freshness were missing.

### D6 — Verification levels and independent feedback

**Decision.** Keep the two levels separate, as the experiment requires:

1. **Board-verified**: `query_state` matches the command (existing
   `OutputVerified`). This is the G4a level.
2. **Independently observed**: an instrument outside the board. Add a tach or a
   current sensor to the bench and send it as `zone.fan_tach.observed`, which
   the spec already models (`fan_rpm_latest`). It is evidence, not a command,
   and enters the Situation and the export like any reading.

No new runtime concept: operator observation stays in the run report until the
instrument exists.

Check: the bench rotated at 600‰ after the wiring fix; 450‰ (the
`bounded_cooling` preset) was only tested before it. Verify that 450‰ turns
the motor. If it does not, changing the preset changes the catalog digest and
needs a coordinated firmware, catalog and pin update.

## Tasks (in order)

| ID | Task | Repo | Gap | Depends on |
| --- | --- | --- | --- | --- |
| X01 | [Experiment compatibility guard](tasks/X01-experiment-compatibility-guard.md) | agentic-stream | all | — |
| X02 | [Repair experiment references](tasks/X02-repair-experiment-references.md) | research | B1, B2 | — |
| X03 | [Material freshness (ADR-018)](tasks/X03-material-freshness.md) | agentic-stream | G2 | X01 |
| X04 | [One intent vocabulary for thermal](tasks/X04-thermal-intent-vocabulary.md) | tamoz + pin here | G3 | — |
| X05 | [Checked-in experiment specs](tasks/X05-checked-in-experiment-specs.md) | agentic-stream | G1, G4, G7 | X01 |
| X06 | [Bench mapping and heartbeat](tasks/X06-bench-mapping-and-heartbeat.md) | research | G6, D4 | X05 |
| U15 | [Approval principal provisioning](tasks/U15-approval-principal-provisioning.md) | agentic-stream | G5 | U13 |
| X08 | [The live pipeline advances on a clock](tasks/X08-live-pipeline-clock.md) | agentic-stream | G8 | done |
| X09 | [Episodes run beside ingestion](tasks/X09-episodes-beside-ingestion.md) | agentic-stream | G9 | X03 |
| X07 | [Joined rehearsal: simulator, then bench](tasks/X07-joined-rehearsal.md) | all | proof | X03–X06, X09, U15 |

After X07, the experiment-relevant tasks from the main review follow:
U21 (shadow, G3 gate), U14 (software interlock for G4c), U01 (safety). The
cleanup tasks are independent and run in parallel as long as X01 stays green.
