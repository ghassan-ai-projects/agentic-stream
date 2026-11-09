# Belt conveyor

Test inputs for the belt conveyor's first closed loop (streams-simulator
`docs/belt-conveyor-drives/M2-DESIGN.md`, story S2-B on plant
`belt-conveyor-drives-b`): three drives `D-1`..`D-3` of one kind share the belt
`B-1` of `conveyor-1`, fed by `F-1`. When one drive's current drops to about
0 A while the feed is still high, the spec opens `line_overload` and the
decision `reduce_feed` (R1, automatic) sets the feeder to the limp feed
(`limp_b`, 18 %) the two survivors can carry; the feeder's setpoint read-back
verifies the command.

Everything lives under `testdata/`, outside the `examples/*/*.situation.yaml`
gates, because the spec needs its own event schemas:

| File | What |
| --- | --- |
| `testdata/line-overload.situation.yaml` | The spec; compile it with `--event-schemas testdata/event-schemas` |
| `testdata/event-schemas/conveyor.json` | One event type per drive's current, the feed and the feed setpoint |
| `testdata/plant-wire/kinds/` | Byte copies of streams-simulator's `examples/belt-conveyor/kinds/*.kind.json` (guarded by `TestTheConveyorKindFilesAreByteCopiesOfStreamsims`) |
| `testdata/plant-wire/belt-conveyor.site.json` | Every device on `conveyor-1`; per-device current readings for the drives |
| `testdata/plant-wire/belt-conveyor.catalog.json` | `reduce_feed` to `set_feed` on `F-1`, presets `limp_b` (18 %) and `restore` (100 %) |

The tests are in `cmd/agentic-stream/experiment_belt_conveyor*_test.go`: one
against a fake plant wire node, and one against the paced streams-simulator
twin (skipped without `STREAMSIM_BIN`), graded by `streamsim outcome` and
`streamsim score`.
