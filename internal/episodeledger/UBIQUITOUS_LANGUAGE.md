# Ubiquitous language — episode ledger

The words of the episode pipeline's durable lifecycle. Code, storage and audit use the same
names. The ledger runs every operation on the caller's transaction.

| Term | Meaning | Code name | Storage / wire name |
| --- | --- | --- | --- |
| Scheduler item | One queued opportunity to reason about a Situation; the pre-admission state of an episode | `SchedulerItem` | `scheduler_items` |
| Pending / admitted / coalesced | Queue states: waiting, taken by an episode, replaced or skipped | `Status` | `status` |
| Coalesce | Remove a pending or admitted item because newer work replaced it, or it was skipped or cost-rejected | `CoalesceSchedulerItems`, `CoalesceSkippedItem`, `CoalesceCostRejectedItem` | `status = 'coalesced'` |
| Episode | One bounded reasoning run admitted from a scheduler item | `Admission` | `episodes` |
| Lifecycle | The episode's coordination state; not a Decision, intent or command | `LifecycleStatus` | `lifecycle_status` |
| Admission | Persisting the episode and its identity once | `Admit` | `episodes` insert |
| Dispatch policy | Whether the episode's intents enter governance; empty means shadow | `DispatchPolicy`, `DispatchShadow` | `dispatch_policy` |
| Live episode conflict | A reconsideration colliding with the one live episode a Situation may have | `ErrLiveEpisodeConflict` | unique `situation_id` |
| Attempt | One worker dispatch of an episode | `AttemptStatus` | `episode_attempts` |
| Fence | Monotonic number of an episode's attempts; an older fence is stale | `Identity.Fence` | `current_fence`, `fence` |
| Identity | Episode, attempt, fence and owner epoch carried by every worker object | `Identity` | — |
| Owner epoch | The runtime tenure an attempt belongs to; must still hold the lease | `Identity.OwnerEpoch` | `owner_epoch` |
| Transition | A valid move between attempt states | `CanTransitionAttempt` | `status` |
| Terminal attempt | Produced, declined, cancelled, failed, timed out or abandoned | `IsTerminalAttempt` | — |
| Rejection | Durable refusal of worker input, with a registered reason | `RejectionReason`, `RecordRejection` | `episode_rejections` |
| Supersession | Ending live episodes because their epoch was killed or their item coalesced | `SupersedeEpoch`, `SupersedeCoalesced` | `lifecycle_status = 'superseded'` |
| Recovery | Abandoning attempts of a previous epoch at restart; canceling ones abandon their episode | `RecoverUnfinishedAttempts` | `terminal_json.reason = runtime_restart` |
| Cost settler | The port that releases an abandoned episode's cost reservation | `CostSettler` | — |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `scheduleledger` (package) | `episodeledger` | The queue item is the episode's pre-admission state, written in the same transactions |
| `Item`, `Upsert`, `MarkAdmitted`, `Coalesce`, `NextPending` | `SchedulerItem`, `UpsertSchedulerItem`, `MarkSchedulerItemAdmitted`, `CoalesceSchedulerItems`, `NextPendingSchedulerItem` | Meaningful without the old package prefix |
| `RecoverUnfinishedAttemptsWithCost` and the cost-free wrapper | `RecoverUnfinishedAttempts(…, costs)` | One entry point; a nil settler skips cost release |
| `Abandon`/`Conclude` (episode) vs attempt `Abandoned` | keep, but read the receiver: episode writers take an episode id, attempt states are `AttemptStatus` | Two lifecycles share a word on purpose |
