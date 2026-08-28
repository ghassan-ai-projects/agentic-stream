# Phase 01 — Telemetry vertical (WP2 → Gate G2)

**Goal:** a real thermal sensor observation, carrying explicit quality and time,
opens a real Situation. **No model. No actuator. No live credential.**

**Program proofs unlocked:** Experiment 1 (sensor truth & calibration) and
Experiment 3 (time, reboot, offline buffer) — the Agentic Stream side of them.
**Allowed claim after G2:** *real sensor telemetry ingested with explicit
quality/time*.

This phase is deliberately the safest and comes first. It exercises the ingress →
eventlog → engine/operators → situations path with a compiler-valid spec, and
nothing downstream of cognition.

---

## Task 1.1 — Thermal event schemas (data, not Go)

Add the device observation schemas to
`internal/eventschema/registry_data.json`. Model them on the existing
`motor.temperature.observed/1.0` entry. Minimum set for the thermal chamber:

- `zone.temp.observed/1.0` — the primary sensor (quantity `celsius`, UCUM unit).
- `zone.ambient.observed/1.0` — the confounder (ambient temperature).
- `zone.heartbeat.observed/1.0` — device liveness (drives missing-heartbeat).
- `zone.fan_tach.observed/1.0` — **independent feedback** channel (fan tach / Hall
  / current), used later in Phase 03 for verification. Add it now so the schema
  and situation grammar are stable before actuation.

Each observation's `data` must be able to carry the quality/provenance fields the
program requires (`round-2/ARCHITECTURE_AND_PROTOCOLS.md` → *Observation*):
`quality` (enum: `valid|warming|invalid|disconnected|rail_high|rail_low`),
`calibration_id`, `firmware_id`, `schema_version`, `raw_value`, and the
device-time fields `boot_id`, `seq`, `device_mono_us`. Gateway-added fields
(`gateway_recv_time`, normalized `event_time`, `time_uncertainty_ms`) ride on the
envelope, not `data`.

Steps:

1. Edit `internal/eventschema/registry_data.json` — add the four refs with their
   field definitions. Keep `snake_case` field names (device wire convention).
2. Run `go test ./internal/eventschema/...`. `TestAllBuiltinsLoadFromData` will
   fail on the pinned golden digest — this is expected. Update `pinnedDigest` in
   `internal/eventschema/registry_test.go` to the new value the test prints. Treat
   this as a **deliberate, reviewed data change** (AGENTS.md), and say so in the
   commit.
3. Confirm no Go literal was added for a schema (grep the diff).

**Exit:** `go test ./internal/eventschema/...` green with the updated pin;
schemas resolve by ref.

---

## Task 1.2 — Compiler-valid `zone_thermal` SituationSpec

Author `docs/design/examples/zone-thermal.situation.yaml`. This **replaces** the
invalid Round 1 starter — do **not** port its fictional fields.

Use the real grammar (validated against `internal/spec/schema.json`), modeled on
`docs/design/examples/predictive-maintenance.situation.yaml`:

- `inputs` — the four schemas from Task 1.1, `partitionKey: entity.id`,
  `entityType: thermal_zone`.
- `time` — tight windows suitable for a bench rig (e.g. `maxOutOfOrderness: 15s`,
  `idleTimeout: 3m`).
- `windows` / `operators` — a `temp_mean` aggregate, a `temp_slope` slope, an
  `ambient_slope` slope (the confounder cross-check), a `latest` aggregate over
  `zone.fan_tach.observed` for the observed-fan feature, and a
  `missing_heartbeat` operator over `zone.heartbeat.observed`.
- `situation` — `type: zone_over_temp`, phases
  `candidate/watch/cooling/over_ceiling/recovering/resolved` with transitions that
  require **both** an out-of-band mean **and** a positive slope (a single high
  reading is not an over-temp), and discount an ambient-tracking rise.
- `cognition` — one deep-lane trigger (needed so Phase 02/03 can attach an
  episode). Objective text is supervisory only ("propose a bounded mode or ask
  for evidence; never own timing").
- `actions.intents` — **declare here now** so the spec is stable, but keep them
  benign for G2:
  - `set_indicator` (LED) — `risk: R1`, `policy: automatic`.
  - `select_thermal_mode` — `risk: R2`, `policy: approval`, with
    `parameterSchema` whose only `modelWritableFields` entry is `mode` (enum
    `hold|bounded_cooling`), and `presets` that fix `duty_permille` / `lease_ms`
    to the bench-safe ceiling. (§4 of the README explains why this keeps PWM out
    of the model's hands.) The effector that consumes these arrives in Phase 03;
    for G2 the spec must merely **compile and deploy**.

Steps:

1. Write the YAML.
2. `go run ./cmd/agentic-stream validate docs/design/examples/zone-thermal.situation.yaml`
   (the `validate` subcommand takes the spec path as a **positional** argument —
   see `cmd/agentic-stream/main.go:397`). Fix until it passes the current
   validator.
3. Add a compile test alongside the existing example tests so regressions are
   caught in CI (mirror how `predictive-maintenance.situation.yaml` is exercised —
   grep `internal/spec` tests for the example load pattern).

**Exit:** the spec compiles and `SaveDeployment` accepts it; the intent catalog
digests cleanly (`internal/episodes/intent_catalog.go`).

---

## Task 1.3 — Observation ingress + quality/time normalization

The gateway (out of repo) emits normalized JSONL envelopes; Agentic Stream
ingests them through the existing `internal/ingress` JSONL replay path. For
HIL-0, **reuse `JSONLReplay`** — do not build a live serial ingress in this repo
(the gateway owns serial; Agentic Stream consumes normalized JSONL).

1. Produce a trace fixture `examples/thermal-chamber/testdata/trace-*.jsonl` with
   envelopes for the four event types, including the Experiment-1/3 edge cases:
   `warming`, `invalid`, `disconnected`, `rail_high/low` quality samples; a
   sequence-counter wrap; two different `boot_id`s (cross-boot ambiguity); and an
   offline-then-flush backlog burst.
2. Confirm the eventlog's quarantine path handles malformed lines (it already
   does — `JSONLReplay.Run` quarantines bad JSON / invalid envelopes). Add a test
   asserting that `invalid`/`disconnected`/`warming` samples **never** surface as
   ordinary valid feature inputs (Experiment 1 pass condition). This likely means
   an operator-level filter keyed on `data.quality`; verify whether existing
   operators can filter on a field or whether a small `quality_gate` operator is
   needed. If a new operator is needed, prefer extending
   `internal/operators` minimally over a new abstraction.
3. Add a test that events are ordered **within a boot** and that cross-boot order
   is left explicit (not silently merged) — Experiment 3 pass condition. The
   gateway supplies `boot_id`; Agentic Stream must not fabricate cross-boot
   precision.

**Boundary note (do not implement here):** the MCU ring buffer, the offline
buffer, and the boot handshake live in firmware/gateway. Agentic Stream only
*consumes* the resulting envelopes and must order them honestly.

**Exit:** with the trace fixtures, a `zone_over_temp` Situation opens on the
sustained-rise trace and does **not** open on the noisy-single-reading or
ambient-tracking traces; quality-gated samples never appear as valid values.

---

## Gate G2 exit checklist

- [ ] `zone.*` schemas load from `registry_data.json`; golden digest re-pinned as
      a reviewed change.
- [ ] `zone-thermal.situation.yaml` passes the live validator and deploys.
- [ ] Trace fixtures cover stable / step / noise / unplug / rail / warm-up /
      repeated-boot / backlog-flush.
- [ ] Invalid/disconnected/warming samples are never consumed as valid features
      (test-proven).
- [ ] Within-boot ordering holds; cross-boot ambiguity stays explicit
      (test-proven).
- [ ] No model call and no effector credential exist anywhere in this path.
- [ ] `make ci-check` green.

Only when every box is checked may Phase 02 begin.
