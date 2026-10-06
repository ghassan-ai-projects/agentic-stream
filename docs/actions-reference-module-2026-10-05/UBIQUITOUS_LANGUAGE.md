# Actions ubiquitous language

The canonical package glossary is [`internal/actions/UBIQUITOUS_LANGUAGE.md`](../../internal/actions/UBIQUITOUS_LANGUAGE.md). This dated copy intentionally records the starting vocabulary used to plan the refactor; update the canonical file first when terms change, then keep this link current.

| Term | Meaning | Code name | Durable name |
| --- | --- | --- | --- |
| Approved command | Policy-authorized request for one effect route and target. | `actionport.Command` | `commands` |
| Command outbox | Durable handoff awaiting delivery. | dispatch candidate | `outbox` |
| Dispatch lease | Time-bounded claim preventing concurrent delivery. | leased command | `lease_owner`, `lease_until`, `attempt_count` |
| Runtime ownership fence | Runtime owner/epoch required for ledger mutations. | owner assertion | `runtime_owner` |
| Interlock | Readiness guard before dispatch and at effector acceptance. | `interlock.Reader`, `actionport.Authorization` | `runtime_interlock` |
| Effector | Boundary that accepts an approved command. | `actionport.Effector` | none |
| Provider result | Transport receipt; not proof of physical success. | `Effect.ProviderResult` | `provider_result_json` |
| Observed effect | Independently observed state. | `Effect.ObservedEffect` | `observed_effect_json` |
| Unknown outcome | Effect may have happened; blind retry is unsafe. | `UnknownOutcomeError` | command `reconciling`, outcome `unknown` |
| Pending verification | Transport accepted but physical state is unconfirmed. | `Effect.VerificationPending` | command `manual_review`, verification `awaiting` |
| Reconciliation evidence | Independent evidence that resolves an uncertain result. | evidence value | `outcomes` result and evidence fields |
| Current authorization | Rechecked command, intent, decision, episode, Situation, approval, policy, lease, interlock, and runtime ownership. | authorization revalidation | read-only joins across owned ledgers |

## Retired words

| Avoid | Replacement | Reason |
| --- | --- | --- |
| Provider success | Provider result or observed effect | Receipt does not prove physical success. |
| Retry unknown | Reconcile unknown outcome | A possibly applied effect must not be repeated blindly. |
| Lease expiry means no effect | Expired in-flight lease is an unknown outcome | The previous owner may already have reached the provider. |
