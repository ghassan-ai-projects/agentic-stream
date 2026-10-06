# Episodes language

Names mean the same thing in conversation, code, storage and audit trails.

## Terms

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Episode | One bounded reasoning unit over an immutable situation version | `Request` / `episodes` row | `episodes` |
| Episode request | The durable canonical executor input document | `Request.RequestJSON` | `episodes.request_json` |
| Admission | Persisting an assembled episode and marking its scheduler item | `Service.Persist` | `episodeledger.Admit`, `episodeledger.MarkSchedulerItemAdmitted` |
| Admission key | 32-byte episode+item identity bound at admission | `Request.AdmissionKey` | `episodes.admission_key` |
| Claim | Fencing the oldest dispatchable episode to a new attempt in one transaction | `Service.RunOnce` / private runner claim | attempt row |
| Attempt | One fenced worker execution with identity and fence | `episodeledger.Identity` | `episode_attempts` |
| Re-bind | Re-pointing a stale admitted episode at the live situation version | private `Assembler.Rebind` | `episodes.stale_rebind_count` |
| Freshness | Dispatch-time recheck that the bound version is still live | `bindLiveSituation` | `situations.current_version` |
| Quarantine (episode) | Durable abandonment inside the claim transaction | runner quarantine step | `episode_rejections` |
| Epoch refusal | A killed policy epoch refuses the dispatch | `quarantineRefusedEpoch` | `epoch_control` |
| Outcome | Terminal result of one worker attempt | `Outcome` | attempt terminal state |
| Decision | The validated typed proposal an attempt returns | `persistDecision` | `decisions` |
| Shadow score | Report-only highest-risk policy result, never an executable intent | `ShadowScore` | `shadow_decisions.shadow_score` |
| Validated intent | An accepted active-mode proposal awaiting policy | `persistValidatedIntents` | `intents.policy_status = pending` |
| Wall-time budget | The request's bounded execution budget | `WallTimeBudget` | `request_json.budget` |
| Supersession | A newer episode for the same situation cancels the in-flight one | `watchSupersession` | `episodes.lifecycle_status` |
| Snapshot evidence | The validated immutable situation snapshot an episode reasons over | domain snapshot evidence | `situation_versions` |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| Public `Assembler` / `Runner` / `With*` | `Service`, `New(Config)` | Consumers receive complete construction, not mutable partial wiring |
| Public `Rebind` | Private runner rebind use case | No production consumer requires an independently callable operation |
| `FakeExecutor` | `executor/fixture.Executor` | Deterministic demo reasoning is a concrete executor adapter |
| Domain `NullString` | Reconciliation-status string | SQL null decoding belongs in store |
| Inline failure reason mapping | domain failure classification | The decision is a rule, not a transaction detail |
