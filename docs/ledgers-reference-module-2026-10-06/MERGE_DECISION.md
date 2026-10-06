# Merge decision

Rule: merge when a layer or package would be tiny, unless a dependency rule or a contract forbids it.

| Pair | Verdict | Evidence |
| --- | --- | --- |
| `scheduleledger` → `episodeledger` | **Merge** | Same transactions (`MarkAdmitted` with `Admit`; `Coalesce` with `SupersedeCoalesced`) and the same callers (`admission`, `cognition` store, `episodes` store). `SupersedeCoalesced` already joins `scheduler_items`. The queue item is the pre-admission state of an episode. 220 lines and one table cannot carry four layers. Neither imports anything internal, so there is no cycle. Cognition's pure layers import only the `Item` struct, which the facade re-exports as `SchedulerItem`. |
| `approvalledger` → `policy` | Rejected | `cognition` withdraws approvals on supersession; merging would make `cognition` import `policy`, which reasoning layers are forbidden to reach. |
| `approvalledger` → `episodeledger` | Rejected | Unrelated lifecycle and tables; `approvalledger` imports `notify` (layer 4) while `control`'s store (layer 3) imports `episodeledger`, so the merge would lift the whole `control` stack and every consumer above it. |
| `approvalledger` stays alone | **Keep, drop the `notify` import** | `WithdrawSuperseded` takes a publisher function supplied by its caller (`cognition` store, which already imports `notify`). The publish still happens in the same transaction, after each mutation, so the existing ordering and rollback test holds. |

Cost of layering the merged ledger: its facade is the vocabulary source for `episodes/internal/domain`
and the executors, so those sit above it. The layer table is re-derived by the longest-path check, as
for notify and control.
