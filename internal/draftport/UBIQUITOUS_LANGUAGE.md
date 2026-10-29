# Draft port ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Draft | One call asking a worker to generalize the verified cases of a key into a rule condition. | `Request`, `Drafter` | `rule_drafts` (owned by `playbook`) |
| Key | Diagnosis code, kind digest, intent and preset: what the cases have in common. | `Key` | `rule_drafts.diagnosis_code`, … |
| Cited case | A verified case sent with the draft: its answer, verdict, the features at decision time and the model's summary (untrusted evidence). | `Case` | `rule_drafts.request_json` |
| Proposal | The worker's drafted rule: the key and a condition over the allowed features, with the cases it cites. The runtime checks it and sets everything else. | `Proposal` | `rule_drafts.reply_json` |
| Draft feature | The handshake feature a worker announces when it can draft. | `Feature`, `ErrUnsupported` | `draft_playbook_rule` |
