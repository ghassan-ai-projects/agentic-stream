# Ubiquitous language: policy governance

| Term | Meaning | Code / storage |
| --- | --- | --- |
| Policy service | Deterministic governance of accepted intents | `Service` |
| Evaluation | One ordered gate pass and durable explanation | `EvaluationRequest`, `policy_evaluations` |
| Intent record | Accepted intent plus its decision, episode and current Situation projection | `IntentRecord`, `intents` |
| Policy result | Stable result/reason and any command/approval identity | `Result` |
| Policy definition | Canonical rules bound to one version | policy document / digest |
| Readiness check | Current action interlock before command publication | `interlock.Reader` |
| Runtime ownership check | Original transaction's owner fence | configured `RuntimeOwner` check |
| Decision epoch check | Refuse the episode's killed/unbound policy epoch | configured `DecisionEpoch` check |
| Approval resolution | Human decision, principals, signature, reason and time | `ApprovalResolution`, `approvals` |
| Approval assertion | Signed, domain-separated binding of the durable request | `ApprovalAssertion` |
| Prepared command | Immutable command bytes and identity before outbox publication | `PreparedCommand`, `commands` |
| Approval context | Bound snapshot digest, trigger delta and accepted decision | `ApprovalContext` |
| Compensation | Intent naming the command it compensates; tenant must match | Intent `compensates` |

| Retired word | Replacement | Reason |
| --- | --- | --- |
| `Gateway`, `NewGateway`, `NewGatewayWithOwner` | `Service`, `New(Config)` | One configured entrypoint. |
| `WithInterlock`, `WithEpochControl`, `WithCalibration` | Constructor configuration | No post-construction safety mutation. |
| `intentRow` / `approvalRow` / `commandDocument` | Named domain records | Describe business facts rather than SQL representation. |
