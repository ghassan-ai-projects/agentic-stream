# Engine ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Partition checkpoint | The last applied log position and watermark of the engine for one partition. | `domain.Checkpoint` | `partition_checkpoints` |
| Inbox | The record that an event was already applied, making replay idempotent. | `store.Tx.EventApplied` / `MarkEventApplied` | `event_inbox` |
| Watermark | Event time minus the maximum out-of-orderness, never moving backwards within a partition. | `domain.WatermarkFor` | `watermark` |
| Operator state | The per-entity state of each operator, replaced as a whole on save. | `operators.OperatorStateBlob` | `operator_state` |
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
