# Clock ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Clock | The runtime's only source of time: `Now` and `NewTimer`. Rules receive time as a parameter. | `Clock` | — |
| Physical clock | Wall time in UTC, for live processing. | `Physical` | quality `physical` |
| Virtual clock | Deterministic time that moves only when advanced, for replay and tests. | `Virtual`, `NewVirtual` | quality `virtual` |
| Advance | Moves a virtual clock forward and fires every due timer in due-time order, ties broken by scheduling order. | `Virtual.Advance` | — |
| Timer | The minimal timer surface the runtime uses. | `Timer` | — |
| Clock quality | Whether time is virtual or physical, recorded with replay evidence. | `Quality` | `physical`, `virtual` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| `time.Now` in rules | A `Clock` or a time parameter | Wall-clock reads break deterministic replay; the architecture gate forbids them in domain layers. |
