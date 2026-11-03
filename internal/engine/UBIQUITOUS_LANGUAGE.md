# Engine ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Partition checkpoint | The last applied log position and watermark of the engine for one partition. | `domain.Checkpoint` | `partition_checkpoints` |
| Inbox | The record that an event was already applied, making replay idempotent. | `store.Tx.EventApplied` / `MarkEventApplied` | `event_inbox` |
| Watermark | Event time minus the maximum out-of-orderness, never moving backwards within a partition. | `domain.WatermarkFor` | `watermark` |
| Operator state | The per-entity state of each operator, replaced as a whole on save; a window's samples are not in it. | `operators.OperatorStateBlob` | `operator_state` |
| Window sample row | One sample of a windowed operator, inserted when the window admits it and deleted when the window expires it, so a save writes what changed and not the window. | `operators.Sample`, `store.Tx.SaveOperatorState` | `operator_window_samples` |
| Operator state cache | The engine's in-memory copy of each entity's operator state as last committed, dropped whenever a record rolls back. | `operatorStateCache` | none |
| Situation runtime state | The digest-bound in-memory state of a Situation persisted for restart. | `domain.SituationState` | `situations.state_json`, `state_sha256` |
| Situation version | One immutable published version of a Situation with its lineage. | `situations.Version` | `situation_versions` |
| Lineage | The ordered evidence set a version rests on, identified by a collision-free length-prefixed hash. | `domain.LineageID` | `lineage_sets` |
| Processing-time timer | A durable timer that fires a missing-heartbeat feature if no newer event arrived. | `domain.DueTimer`, `domain.HeartbeatTimer` | `timers` |
| Boot fencing | A timer whose operator state is no longer active is acknowledged without firing. | `domain.InactiveTimerIDs` | none |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Stream engine vs engine | Engine with cognition disabled | One engine; cognition is a configuration choice. |
| Run (partition) | Drain partition | Only `RunGlobal` is production API. |
