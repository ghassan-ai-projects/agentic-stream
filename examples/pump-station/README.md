# Pump station example

The first slice of [rule promotion](../../documentation/design/rule-promotion.md):
eight pumps of the streams-simulator pump station, reported over plant wire, and
the spec the campaign runs against.

| File | Holds |
| --- | --- |
| [`pump-station.situation.yaml`](pump-station.situation.yaml) | One `pump_condition` situation per pump, opened on per-pump relative features (a 15-minute mean against the pump's own 24-hour mean, for radial vibration, axial vibration and current); the diagnosis catalog; the `promotion` block (settle windows, the current as a context feature, the envelope margin). |
| [`testdata/plant-wire/pump-station.site.json`](testdata/plant-wire/pump-station.site.json) | `site-a:pump-1` to `site-a:pump-8`, each reporting for the pump entity of the same id; readings to `pump.*.observed` events. |
| [`testdata/plant-wire/pump-station.catalog.json`](testdata/plant-wire/pump-station.catalog.json) | Fleet routes: each intent route commands the pump bound to the situation's entity and fills `pump_id` with it; `schedule_maintenance` has one preset per work order and expires after 3 minutes, a booking rather than a real-time actuation, so it outlives a barrier-paced world's 60-second steps (serve it with `--dispatch-lease 5m`). |
| [`testdata/plant-wire/kinds/`](testdata/plant-wire/kinds/) | The `pump` kind of the `pump-station` world. |
| [`testdata/site-a.playbook.yaml`](testdata/site-a.playbook.yaml) | A test playbook: an active `bearing-wear` rule and a shadow `misalignment` rule, used by the playbook tests. A real playbook lives in the customer workspace, never in a repository. |
| [`testdata/plant-wire/kinds-probe-b/`](testdata/plant-wire/kinds-probe-b/) | The `pump` kind of the `pump-station-probe-b` world: the same kind with another digest (scenario S7). |
| [`testdata/campaign/`](testdata/campaign/) | The campaign driver's data: the approval principals (relay and approver, the all-zero-seed test key, R1 and R2 on every pump), the empty playbook a learning arm starts from and the hand-written static playbook of arm 0. |

The intents: `schedule_maintenance` (R1, automatic, one preset per work order:
`bearing_wear`, `misalignment`, `sensor_drift`, `sensor_failure`,
`lubrication_due`, the simulator's fault ids), `reduce_load` (R2, approval) and
`stop_motor` (R3, `emergency`, never a rule target), plus the watch fallback.

```bash
agentic-stream validate examples/pump-station/pump-station.situation.yaml
```

The kind files are copies of the simulator's `examples/pump-station/kinds/`;
both define the kind `pump`, so a host commissions one directory at a time.
