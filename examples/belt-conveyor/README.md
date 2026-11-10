# Belt conveyor

Test inputs for the belt conveyor's closed loops (streams-simulator
`docs/belt-conveyor-drives/`): three drives `D-1`..`D-3` of one kind share the
belt `B-1` of `conveyor-1`, fed by `F-1`. Three specs, one per `serve`:

- `line_overload` (M2, story S2-B on plant `belt-conveyor-drives-b`): when one
  drive's current drops to about 0 A while the feed is still high, the decision
  `reduce_feed` (R1, automatic) sets the feeder to the limp feed (preset `limp`, 18 % on plant B)
  the two survivors can carry; the feeder's setpoint read-back verifies the
  command.
- `drive_degradation` (M3, stories S1, S3 and S4): a drive whose winding
  temperature or vibration leaves its peers' band at equal current walks the
  line through `normal`, `watch`, `degrading`, `shedding`, `belt_empty`, a
  cause phase and `isolated` or `belt_stopped`, one trigger per step. The
  steps that do not depend on the cause come first, whatever the diagnosis
  (O34): at `degrading` shed the feed to the plant's limp preset (R1), at
  `shedding` stop the feed so the belt runs empty (R2). On the empty belt the
  line waits until the evidence names the cause: `cause_bearing` when the
  drive's vibration residual reaches 0.3 mm/s (stop the drive by plan,
  `isolate_drive`, R2), `cause_gearbox` when its winding slope residual
  reaches 2 K/h while its vibration residual stays below 0.05 mm/s (stop the
  belt, `stop_belt`, R2), or `cause_undecided` after 5 h with neither (stop
  the belt, the safe default). Once the drive is isolated the limp feed is
  restored (R1). The watch preset (`install_watch_condition`) holds when the
  bearing or the gearbox rule holds; a reasoner that only installs it at
  `degrading` or `shedding` is asked again when it fires (the triggers name
  `delta.watch_fired`). A feed surge moves every drive together and no residual.
  Every window emits on close, because an R2 intent is denied on provisional
  evidence, and counts only with 90 % of its readings. The thresholds come
  from streams-simulator's calibration (`M3-DESIGN.md` section 7) and are
  named in the one transition that uses each. The wait: an empty belt is the
  safe state (the drive cannot reach its overtemperature trip there and a
  seizing gearbox cannot strand it), but a dry bearing wears with its load
  cubed, about 4.5 times slower than at full feed, so the bearing rule, 0.6 to
  1.0 h after the degrading rule at full feed, takes up to about 4.2 h on an
  empty belt; the twin measured 1.9 h (S1-A) and 3.6 h (S1-B) on row 1.

- `drive_degradation_planned_stop` (M4, story S1-B on plant `belt-conveyor-drives-b`,
  whose survivors have no margin and whose limp feed, 18 %, earns too little to
  pay for an early isolation): the same readings, rules and thresholds, but the
  line keeps its full feed after the drive starts degrading (`degrading`) and
  after its bearing is named (`bearing_loaded`, no step), and takes the planned
  isolation (`shedding`: stop the feed, R2; on the empty belt `cause_bearing`:
  stop the drive, R2; `isolated`: the limp feed, R1) when D-3's winding is
  predicted to reach its 150 C trip within 1.5 h plus the 3 min the belt takes
  to empty. The prediction is the operator kind `extrapolate`: a quadratic fit
  of D-3's winding over 60 min (`documentation/design/extrapolate-operator.md`,
  calibrated on S1-B's ten do-nothing rows). A gearbox named at full feed
  empties the belt at once; a cause still untold when the trip is predicted
  empties the belt and waits there for it (O34). Each plant's deployment runs
  its own spec, as it runs its own device catalog: nothing in a snapshot tells
  the plants apart, and a hold command at full feed would be an effector call
  that ends the prognosis window.

Everything lives under `testdata/`, outside the `examples/*/*.situation.yaml`
gates, because the specs need their own event schemas:

| File | What |
| --- | --- |
| `testdata/line-overload.situation.yaml` | The M2 spec; compile it with `--event-schemas testdata/event-schemas`. It declares every reading the shared site gives as an input, also those no operator reads: a deployment registers only its inputs' event schemas and quarantines the rest |
| `testdata/drive-degradation.situation.yaml` | The M3 spec; compile it the same way |
| `testdata/drive-degradation-planned-stop.situation.yaml` | The M4 planned-stop spec for plant B; compile it the same way |
| `testdata/event-schemas/conveyor.json` | One event type per drive's current, winding and vibration, the feed, its setpoint and the belt's tail speed |
| `testdata/plant-wire/kinds/` | Byte copies of streams-simulator's `examples/belt-conveyor/kinds/*.kind.json` (guarded by `TestTheConveyorKindFilesAreByteCopiesOfStreamsims`) |
| `testdata/plant-wire/belt-conveyor.site.json` | Every device on `conveyor-1`; per-device current, winding and vibration readings for the drives |
| `testdata/plant-wire/belt-conveyor-{a,b}.catalog.json` | One device catalog per plant, as each plant's deployment has its own: `reduce_feed` (presets `limp`, the plant's safe limp feed, 54 % on plant A and 18 % on plant B, and `restore` 100 %; the specs name only `limp`, because nothing in a snapshot tells the plants apart) and `stop_feed` (0 %) to `set_feed` on `F-1`, `isolate_drive` to `stop_drive` on `D-3`, `stop_belt` to `stop_belt` on `B-1`; a command lives 180 s, longer than a barrier-paced world step (60 s), so `serve` takes `--dispatch-lease 5m` |
| `testdata/principals.yaml` | A test approver for R1 and R2 on `conveyor-1` (the all-zero-seed test key) |

The tests are in `cmd/agentic-stream/experiment_belt_conveyor*_test.go` (M2) and
`experiment_belt_degradation*_test.go` (M3): against a fake plant wire node that
replays backdated readings at the sensors' cadences and obeys the host's
commands, and against the streams-simulator twin (skipped without
`STREAMSIM_BIN`; M3's stories S1-A, S3-A and S4-A run barrier-paced, and the
planned-stop spec plays S1-B, S4-B, and S4-A and S0-A on plant A's catalog, where
nothing degrades), graded by `streamsim outcome` and `streamsim score`. On the
planned-stop story the verdict carries D-3's predicted trip
(`predicted_failure_at`) and the run must beat every baseline. Their reasoner is a stand-in that
diagnoses by the catalog's rules (`drive_degrading` while neither the bearing
nor the gearbox rule holds; the cause refines it) and takes the phase's step,
which does not depend on its diagnosis. The verdict maps `drive_degrading` to
D-3 without a label, a symptom that dates the later cause's detection, and
leaves `unknown` out.

`experiment_belt_live_*_test.go` plays S1-A, S2-B and S3-A with the real Tamoz
worker as the reasoner instead (arm B: Tamoz reasons, the specs act). It calls a
real model and runs only when asked: `CONVEYOR_LIVE_MODEL=1`,
`CONVEYOR_TAMOZ_DIR` (a Tamoz checkout, run under `rbenv`), `ZAI_API_KEY` in the
environment, `STREAMSIM_BIN`, and optionally `CONVEYOR_LIVE_ROWS` (seed rows of
streams-simulator's `stories.json`, default `1`) and `CONVEYOR_LIVE_OUT` (where
each run keeps its database, Tamoz's database and log, the worker exchanges, the
twin's run folder, `outcome.json`, `score.txt` and the verified export). Each run
starts its own worker on a fresh Tamoz database, copies the spec with the live
budget (300 s wall time, 24000 input and 20000 output tokens: the model reasons
20 to 80 s per call), counts model calls and refuses further episodes past 60;
R2 steps are approved by the scripted approver through the signed relay.
