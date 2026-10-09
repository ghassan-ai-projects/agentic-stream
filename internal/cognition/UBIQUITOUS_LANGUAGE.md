# Cognition language

This file is the canonical vocabulary for the cognitive scheduler. The dated
migration record (`docs/cognition-reference-module-2026-10-05/UBIQUITOUS_LANGUAGE.md`)
keeps the code-backed planning snapshot.

| Term | Meaning | Code or storage name |
| --- | --- | --- |
| Cognition service | Deterministically evaluates new Situation versions and records why work was or was not admitted | `Service` |
| Trigger | A compiled rule evaluated against Situation features, state, and the delta from the last reasoned version | `spec.Trigger`, `cognition.triggers` |
| Evaluation | The result of evaluating one trigger, including its score, first failed gate, evidence delta, and admission outcome | `Evaluation`, `trigger_evaluations` |
| Last reasoned version | The Situation version used as the baseline for the next cognitive delta | `situations.last_reasoned_version` |
| Scheduler item | Durable unit of queued episode work, owned by the scheduler ledger | `episodeledger.SchedulerItem`, `scheduler_items` |
| Lane | Configured class of episode work, such as fast or deep | Trigger lane, `scheduler_items.lane` |
| Material delta | Configured test for whether a change since the last reasoned version warrants new work | `spec.Trigger.MaterialDelta` |
| Debounce | Minimum delay after an evaluation before its scheduler item may run | `spec.Trigger.Debounce`, `scheduler_items.not_before` |
| Cooldown | Minimum interval after the previous admitted evaluation before the same trigger may run again | `spec.Trigger.Cooldown`, `trigger_evaluations.evaluated_at` |
| Capacity deferral | A valid admission delayed because the tenant queue reached its fixed capacity | Evaluation outcome `deferred` |
| Supersession | Coalescing older pending work when a newer evaluation is admitted for the same Situation and trigger | `episodeledger.CoalesceSchedulerItems`, outcome `coalesced` |
| Correction | A corrected Situation version that invalidates the earlier basis of an accepted, completed action | `situations.Version.Completeness`, `reconsiderations` |
| Reconsideration | A deterministic deep-lane episode request explaining an invalidated accepted action against a corrected Situation | `reconsiderations`, scheduler item kind `reconsider` |
| Cost refusal | Episode admission refusal recorded on the trigger evaluation while queue lifecycle remains owned by the scheduler ledger | `RecordCostRejectionReason` |

| Retired word | Replacement | Reason |
| --- | --- | --- |
| Engine, for the package's public component | Cognition service | One public module entrypoint owns trigger evaluation and scheduling use cases. |
| NewEngine with a database argument | New with explicit configuration | The database argument was unused; operations join the caller's transaction. |
| Scheduler as a public package type | Internal application component | Production callers use the cognition service, not the queue implementation. |

Keep established identities, outcome strings, lane names, notification names,
and storage fields stable. New code names the business action, not the migration
round or its implementation mechanism.
