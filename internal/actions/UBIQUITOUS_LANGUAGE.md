# Actions ubiquitous language

This glossary is the canonical vocabulary for the governed action plane. The
dated migration record links here so code, durable records, and audit events use
the same terms.

| Term | Meaning | Code name | Durable or wire name |
| --- | --- | --- | --- |
| Approved command | A policy-authorized request that names one effect route, target, and idempotency identity. | `actionport.Command` | `commands`; `command_id`, `intent_id`, `effector_route`, `normalized_target`, `idempotency_key` |
| Command document | The schema-validated, digest-bound command record that is rechecked before dispatch. | `domain.Document`, `domain.CommandRow` | `command_json`, `command_sha256` |
| Command outbox | The durable handoff that makes an approved command eligible for delivery. | `domain.Candidate` | `outbox` row with `kind='command'` |
| Dispatch lease | A time-bounded claim that prevents another dispatcher from sending the same outbox row. | `domain.LeasedCommand`, `domain.OutboxLease` | `lease_owner`, `lease_until`, `attempt_count` |
| Runtime ownership fence | The current runtime owner and epoch required to mutate the dispatch ledger. | `store.Tx.AssertOwner` | `runtime_owner`, `Config.Epoch` |
| Interlock | A final readiness gate checked before the call and again at effector acceptance. | `store.Tx.AssertInterlock`, `store.Store.DispatchAuthorization` | `runtime_interlock` |
| Effector | The narrow port that accepts an approved command and returns a provider result. | `actionport.Effector` | no independent row |
| Provider result | The transport/provider receipt for a dispatch attempt. It does not by itself prove a physical state change. | `actionport.Effect.ProviderResult` | `outcomes.provider_result_json` |
| Observed effect | Independently observed state associated with an effect attempt. | `actionport.Effect.ObservedEffect` | `outcomes.observed_effect_json` |
| Outcome | The digest-bound record of what the dispatcher knows about one effect attempt. | `domain.OutcomeRecord`, `domain.DispatchResult` | `outcomes`; `status`, `reconciliation_status`, `outcome_sha256` |
| Verification | The current verification state for an intent and its latest outcome. | `domain.DispatchClosure`, `domain.ReconciliationClosure` | `verifications`; `status`, `outcome_id`, `reconciled_at` |
| Unknown outcome | The provider may have applied an effect, so automatic retry is unsafe until independent evidence resolves it. | `actionport.UnknownOutcomeError` | command `reconciling`; outcome `unknown`; reconciliation `required`; outbox `failed` |
| Pending verification | The provider accepted transport delivery, but physical success still needs an independent observation. | `Effect.VerificationPending` | command `manual_review`; outcome `reconcile_required`; verification `awaiting` |
| Reconciliation evidence | Independent provider or device evidence used to resolve an unknown result. | `domain.ValidateReconciliation` input | outcome result `evidence`; device fields include `device_id`, `boot_id`, `state`, and a SHA-256 digest |
| Policy freshness | Confirmation that the command's policy digest remains the latest approved evaluation. | `domain.CheckPolicyDigest` | `policy_digest`, `policy_evaluations` |
| Current authorization | The live proof that command, intent, decision, episode, Situation version, approval, policy, lease, interlock, and runtime ownership still agree before dispatch. | `domain.AuthorizationRecords` | joined command, intent, decision, episode, Situation, approval, and policy records |

## Status vocabulary

| State | Meaning | Code or stored value |
| --- | --- | --- |
| Pending | Approved command is waiting for a dispatcher. | command `pending`, outbox `pending` |
| Dispatching | The command is leased and is at the effect boundary. | command `dispatching`, outbox `leased` |
| Succeeded | The effect result is known and successful. | command/outcome `succeeded`, verification `observed` |
| Failed | The effect was rejected or known to have failed. | command/outcome `failed`, outbox `failed` |
| Reconciling | The effect may have happened; the command cannot be retried blindly. | command `reconciling`, outcome `unknown` |
| Manual review | Transport accepted the command, but independent verification remains outstanding. | command `manual_review`, outcome `reconcile_required`, verification `awaiting` |
| Reconciled | Independent evidence resolved the outcome. | outcome `reconciled`; verification `reconciled`, `refuted`, or `inconclusive` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Provider success | Provider result or observed effect | A transport acknowledgement alone does not establish physical success. |
| Retry unknown | Reconcile unknown outcome | Repeating a possibly applied effect can duplicate a consequential action. |
| Lease expiry means not sent | Unknown outcome after an expired in-flight lease | The former owner may have crossed the effect boundary before losing its lease. |
| Command payload as authority | Digest-bound command document plus live authorization records | A payload is data; permission comes from current policy and lifecycle records. |
