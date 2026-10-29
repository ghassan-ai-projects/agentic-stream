# Rule executor ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Rule executor | The in-process executor of playbook rules. It decides a rule-bound request from the request alone, calls no model, owns no lifecycle or effects, and hands every other request to the model executor it wraps. | `Executor`, `New` | — |
| Rule-bound request | An episode request the playbook routed to a rule: executor name `rule`, executor version the playbook digest, and a `rule` section with the recommendation. | `ExecutorName`, `Section` | `episodes.request_json`, `episodes.executor_name` |
| Rule decision | The Decision of a rule-bound request: the rule's diagnosis code, one intent with the rule's preset parameters (plus the episode's entity and Situation when the intent declares them), the features the rule read as facts, and a summary naming the rule, its version and the playbook digest. It takes the ordinary validation, policy and dispatch path. | `decide` | `decisions` |
| Intent lifetime | How long the intent of a rule decision stays valid, from the executor's clock. | `IntentLifetime` | `intents.expires_at` |
