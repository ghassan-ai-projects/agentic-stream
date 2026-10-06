# Operators ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Operator | A deterministic, keyed stream computation declared in the spec. Kinds: `aggregate`, `slope`, `missing_heartbeat`. | `OperatorRuntime` | `operators` in the spec |
| Operator runtime | Applies the compiled operators to normalized events and to due timers. | `NewOperatorRuntime`, `ApplyEventAt`, `ApplyTimer` | — |
| Window | A tumbling or sliding span of samples, with an emit mode (`on_update`, `on_close`, `early_and_close`). | `WindowState` | `OperatorStateBlob.Window` |
| Sample | One timestamped scalar input to a windowed operator. | `Sample` | `samples` |
| Feature | An emitted operator result with its window, watermark, input event ids and completeness. | `Feature` | event type for situation input |
| Completeness | How final a feature is: `provisional`, `on_time`, `corrected`, `final_by_policy`, `uncertain`. | `Completeness` | `completeness` |
| Heartbeat | The last seen event for a keyed entity; its absence past a deadline is the `missing_heartbeat` feature. | `HeartbeatState` | `OperatorStateBlob.Heartbeat` |
| Boot admission | A device boot is admitted once; events from an earlier boot are stale evidence and never change active state. | `RuntimeState` | `OperatorStateBlob.Runtime`, operator id `__agentic_stream_runtime__` |
| State key | The exact durable key of one operator instance (operator and entity). | `Feature.StateKey`, `State` | `operator_state` (owned by `engine`) |
| Partition state | The in-memory operator state of one partition: operator id to state key to blob. | `PartitionState` | — |
| Timer identity | The tenant and partition whose timer is firing. Required when features are persisted. | `TimerIdentity` | `timers` (owned by `engine`) |

Processing time comes from the runtime clock. Producer timestamps are evidence,
never scheduling authority.

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Metric | Feature | A feature carries provenance and completeness; a metric does not. |
| Runtime (for the operator set) | Operator runtime | `runtime` is the live pipeline package. |
