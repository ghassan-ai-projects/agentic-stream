# Playbook ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Playbook | The compiled file of human-approved rules (`spec.CompiledPlaybook`), with its own digest. | `Config.Playbook`, `Service.Digest` | `playbook_digest` |
| Rule | One deterministic answer to a recurring, verified kind of case (see the spec's language). Identified by its id and version. | `domain.RuleKey` | `rule_id`, `rule_version` |
| Route | How an episode admission is answered: by the model, by one rule or as an audit of a rule. Decided in the admission's transaction from the immutable snapshot; an audit that meets a live episode is refused by the episode ledger and the whole admission waits for a later pass. | `Route`, `RouteModel`, `RouteRule`, `RouteAudit` | — |
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
| Case | One occurrence of a Situation (until it resolves) whose decision dispatched an action, keyed by diagnosis code, kind digest, intent and preset. The only evidence rule proposals count. | `domain.Case`, `CaseCandidate` | `case_verifications` |
| Acknowledged | The case's command succeeded or awaits manual review after the device's own receipt and result: the device took the order. A failed or unknown command is not acknowledged. | `CaseCandidate.Acknowledged` | `commands.status` |
| Settle window | How long, in event time from the decision, a case must stay resolved: a version that is not resolved on the entity before it ends, or a Situation that never resolves, makes the case not verified. Declared per diagnosis code in the spec; a rule's code must have one. | `Case.Verify`, `CaseEvidence` | `window_ends_at` |
| Verified case | A case whose action was acknowledged and whose Situation resolved and stayed resolved through its settle window. | `CaseVerified` | `case_verifications.verdict` |
| No effect | The circuit cause of a rule whose own case was not verified. | `CauseNoEffect`, `NoEffectCircuit` | `rule_circuits.cause` |
| Envelope and cap | The envelope is the range of each feature a rule's verified source cases showed; the cap, which the approver may tighten, is the wider range a rule may grow into later. A match outside the envelope goes to the model and is recorded with whether it fell inside the cap. | `Match.InEnvelope`, `Match.InCap` | `rule_evaluations.in_envelope`, `in_cap` |
| Out of cap | The circuit cause of a rule whose recent matches (24 h, at least four) mostly fell outside its cap. | `CauseOutOfCap`, `CapCounts` | `rule_circuits.cause` |
| Key | Diagnosis code, kind digest, intent and preset: what a case answered and on which kind. Drafts, proposals and rules are per key. | `domain.Key`, `KeyedCase` | `case_verifications`, `rule_drafts`, `rule_proposals` |
| Eligible key | A key with enough verified cases (`--draft-after`, three by default), none contradicting, no open draft or proposal, no rule answering it, and newer evidence after a rejection. | `domain.Eligible` | — |
| Draft | One call asking the worker to generalize a key's verified cases into a condition (see `draftport`). | `Draft`, `domain.Draft` | `rule_drafts` |
| Proposal | A runtime-built rule from a checked draft reply, with its replay report, awaiting a human. | `domain.Proposal`, `ProposalPresentation`, `ProposalResolution` | `rule_proposals`, `playbook.rule.proposed` |
| Replay report | What a proposed rule would have done over the stored Situation history: versions examined, would fire, decided by the model, agreed and disagreed, and how the key's source cases ended. | `domain.ReplayReport` | `rule_proposals.replay_json` |
| Binding | What an approver signs beside the proposal digest: the cap, the shadow exit, the audit rate and the rate cap. | `domain.Binding`, `Proposal.SigningBytes` | `rule_proposals.approval_digest` |
| Playbook writer | Writes the next playbook version atomically, only if the file is still the one loaded; approvals add a rule in shadow, a met shadow exit marks it active. Takes effect at the next start. | `playbookFile`, `ErrPlaybookChanged`, `Promote` | `<site>.playbook.yaml` |
| Shadow exit | Agreeing and disagreeing shadow evaluations that move a shadow rule to active (ten and none by default). | `domain.ExitMet` | `rules[].shadowExit` |

