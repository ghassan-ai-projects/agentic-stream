# Playbook ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Playbook | The compiled file of human-approved rules (`spec.CompiledPlaybook`), with its own digest. | `Config.Playbook`, `Service.Digest` | `playbook_digest` |
| Rule | One deterministic answer to a recurring, verified kind of case (see the spec's language). Identified by its id and version. | `domain.RuleKey` | `rule_id`, `rule_version` |
| Route | How an episode admission is answered: by the model, by one rule, as an audit of a rule, or not yet (an audit that met a live episode stays queued for a later pass). Decided in the admission's transaction from the immutable snapshot, so a replay routes the same way. | `Route`, `RouteModel`, `RouteRule`, `RouteAudit`, `RouteSkip` | — |
| Match | A rule whose condition holds over the snapshot's features; inside its envelope or not; with the features it read. A rule with an open circuit never matches. | `domain.Match` | — |
| Fire | One episode a rule answered: exactly one active rule matched, inside its envelope, under its rate cap. | `domain.Fire` | `rule_fires` |
| Recommendation | What a rule recommends: its identity, the playbook digest, the diagnosis code, the intent and preset with its parameters, and the features it read. It becomes the request's rule section. | `Recommendation` | `recommendation_json` |
| Shadow evaluation | What a shadow rule would have recommended beside a model episode, judged against the model's stored decision. | `domain.NewShadowEvaluation` | `rule_evaluations` (`kind = shadow`) |
| Audit | A shadow-dispatch model episode for the version a rule answered, queued once the rule's episode ended, for the fires the audit sample chose. | `domain.AuditDue`, `AuditRequest`, `AuditQueue` | `rule_evaluations` (`kind = audit`), `rule_fires.audit_due` |
| Audit sample | The deterministic choice of which fires are audited: a hash of the Situation id and version, one in the rule's `auditEvery`. Never random, so a replay reproduces it. | `domain.AuditDue` | `rule_fires.audit_due` |
| Verdict | How an evaluation ended: the model agreed (same diagnosis code, intent and preset), disagreed, or was inconclusive (no accepted decision). | `domain.VerdictAgree`, `VerdictDisagree`, `VerdictInconclusive` | `rule_evaluations.verdict` |
| Observed | What the model's decision said: its diagnosis code, its actionable intent and the preset its parameters name. | `domain.Observed`, `domain.PresetOf` | `rule_evaluations.observed_json` |
| Circuit | A rule's off switch. Opened by an audit disagreement or by reaching the rate cap (later rounds: a kind change, no effect, out of cap); a rule with an open circuit is absent; only a human closes it. Opening it publishes a notification. | `domain.Circuit`, `CauseAuditDisagreement`, `CauseRateCap` | `rule_circuits`, `playbook.rule.circuit_opened` |
| Rate cap | The most fires a rule may have in an hour; the match that would exceed it opens the circuit and goes to the model. | `RateCapPerHour` | `rule_fires.fired_at` |
| Settle | The step after each episode batch: queue the due audits, judge pending evaluations against stored decisions, open the circuit of a rule an audit disagreed with. | `Service.Settle`, `Report` | — |
