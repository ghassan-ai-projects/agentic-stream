# Operators ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Operator | A deterministic, keyed stream computation declared in the spec. Kinds: `aggregate`, `slope`, `extrapolate`, `missing_heartbeat`. | `OperatorRuntime` | `operators` in the spec |
| Extrapolation | The seconds until a least-squares fit (linear or quadratic) of a window's samples reaches a declared limit; 0 when there is no valid prediction (too few samples, not rising, no crossing ahead or beyond the horizon). | `secondsToLimit`, `spec.Extrapolation` | the operator's `configuration` |
| Operator runtime | Applies the compiled operators to normalized events and to due timers. | `NewOperatorRuntime`, `ApplyEventAt`, `ApplyTimer` | — |
| Window | A tumbling or sliding span of samples, with an emit mode (`on_update`, `on_close`, `early_and_close`). | `WindowState` | `OperatorStateBlob.Window` |
| Sample | One timestamped scalar input to a windowed operator. | `Sample` | `operator_window_samples` (owned by `engine`) |
| Feature | An emitted operator result with its window, watermark, input event ids and completeness. | `Feature` | event type for situation input |
| Completeness | How final a feature is: `provisional`, `on_time`, `corrected`, `final_by_policy`, `uncertain`. | `Completeness` | `completeness` |
| Heartbeat | The last seen event for a keyed entity; its absence past a deadline is the `missing_heartbeat` feature. | `HeartbeatState` | `OperatorStateBlob.Heartbeat` |
| Boot admission | A device boot is admitted once per entity and input event type; events from an earlier boot of that event type are stale evidence and never change active state. A new boot discards the state of every operator the event type feeds (an operator fed by several event types loses its state when any of them admits a new boot) and leaves the other event types' state alone, so several devices can feed one entity. | `RuntimeState`, `BootFence` | `OperatorStateBlob.Runtime`, operator id `__agentic_stream_runtime__` |
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
