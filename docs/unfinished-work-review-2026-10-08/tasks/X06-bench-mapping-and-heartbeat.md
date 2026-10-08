# X06 — Bench mapping entity id and gateway heartbeat

Status: todo · Decision: **change the experiment's gateway config, not the catalog** · Priority: P0 (experiment) · Size: S · Depends on: X05

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md) G6, D4.

## Finding

`assessment/arduino-mega-dht11.mapping.json` maps the DHT11 to
`entity_id: zone-1`; the capability catalog binds `zone-01`. The materializer
refuses unbound targets, so a bench command fails closed. The gateway emits no
heartbeat, so a dead USB link is indistinguishable from a stable room.

## Steps (in `agent-research-lab/real-world-sensor`; owner approval needed)

1. Mapping: `entity_id: zone-01` for both sensors.
2. Gateway: emit `zone.heartbeat.observed/1.0` for each board `state` or
   telemetry frame, with the board's boot id and sequence, so `missing_heartbeat`
   works. Add a gateway test.
3. Re-run the sensor-only ingress proof and record that the Situation opens with
   the X05 bench spec when the sensor is heated.

## Done when

On the bench, one heated reading series opens `zone_over_temp` for `zone-01`,
and unplugging USB raises `link_missing_90s` within 90 s.
