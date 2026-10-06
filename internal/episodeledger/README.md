# Episode ledger module

The episode ledger owns the durable lifecycle of the episode pipeline: scheduler queue items,
the episodes they admit, their fenced attempts, the rejection audit, supersession and restart
recovery. `scheduleledger` was merged into it: the queue item is an episode's pre-admission
state and is written in the same transactions by the same callers.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | Aliases of the domain vocabulary; one-line operations that join the caller's `*sql.Tx`; the wall-clock default for the owner-lease check |
| App | Use cases in order: read state, decide (domain), write; worker identity validation (run by every transition), attempt start and transition under fencing, recovery loop, queue operations |
| Domain | Statuses, `Identity`, fence and attempt checks over values the store read, the transition table, rejection reasons and ids, admission defaults, the recovery terminal |
| Store | The only SQL for `scheduler_items`, `episodes`, `episode_attempts`, `episode_rejections`; reads the runtime owner lease; classifies unique violations |

Every operation takes the caller's transaction; the ledger never begins or commits. Fencing keeps
its order: episode fence, owner lease, attempt state; a closed episode may still acknowledge the
cancellation of its current attempt, never a produced Decision. Recovery releases cost
reservations only for canceling attempts, through the `CostSettler` port, because `control`
imports this ledger and cannot be imported back.

The store reads `runtime_owner` (control's table) and the coalescing statement reads
`trigger_evaluations` (cognition's table); both are recorded as follow-ups.

[Migration record](../../docs/ledgers-reference-module-2026-10-06/README.md).
