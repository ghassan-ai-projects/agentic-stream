# Cognition ubiquitous language

This is the code-backed planning snapshot. The canonical glossary is
[`internal/cognition/UBIQUITOUS_LANGUAGE.md`](../../internal/cognition/UBIQUITOUS_LANGUAGE.md).

| Term | Meaning | Code or storage name |
| --- | --- | --- |
| Cognition service | Deterministically evaluates new Situation versions and records why work was or was not admitted | `Service` |
| Trigger | A compiled rule over Situation features, state, and the last-reasoned delta | `spec.Trigger`, `cognition.triggers` |
| Evaluation | One trigger result with score, first failed gate, evidence delta, and outcome | `Evaluation`, `trigger_evaluations` |
| Last reasoned version | Baseline used for the next cognitive delta | `situations.last_reasoned_version` |
| Scheduler item | Durable queued episode work, owned by the scheduler ledger | `scheduleledger.Item`, `scheduler_items` |
| Reconsideration | Deep-lane work that re-evaluates an accepted action after a correction | `reconsiderations`, kind `reconsider` |
| Cost refusal | Admission refusal appended to evaluation reasons without changing queue state | `RecordCostRejectionReason` |

| Retired word | Replacement | Reason |
| --- | --- | --- |
| Public Engine | Cognition service | One facade owns the public module operations. |
| NewEngine with database parameter | New with explicit configuration | Construction does not own a database; operations join the caller's transaction. |
| Public Scheduler | Private application component | Queue mechanics are not an external contract. |
