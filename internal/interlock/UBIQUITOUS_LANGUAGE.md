# Interlock ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Interlock | The single durable, global switch that blocks the action plane. It is not an epoch kill: epochs belong to `control`. | `State`, `Assert` | `runtime_interlock` (one row, `singleton_id = 1`) |
| Ready | The interlock allows commands to be created and effects delivered. | status `ready` | `status` |
| Tripped | The interlock blocks all action. The reason is recorded and returned in the error. | status `tripped`, `ErrTripped` | `status`, `reason` |
| Assert | Read the interlock inside the caller's transaction and fail closed when it is absent or not ready. Run before command creation and again before effect delivery. | `Reader.Assert` | — |
| Version | Strictly increasing counter; an update must carry a higher version. | `Set(version)` | `version` |

`Set` changes the state and must be fenced by the caller with the active runtime
owner. No production path calls it yet (see the remaining-migration plan).

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Kill switch (for the interlock) | Interlock, or epoch kill | `control` owns the epoch kill; the interlock is a separate, read-only gate for actions. |
