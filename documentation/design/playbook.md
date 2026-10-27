# Playbook

Status: accepted design, built in rounds (see [rule promotion](rule-promotion.md),
rounds AS-1 to AS-8). This page is the decision record the `playbook` module is
built against; the rule promotion plan holds the owner decisions it cites
(numbered 1 to 18, findings F1 to F4).

## Context

Every decision about what to do goes to a model episode today. The
SituationSpec decides when a situation opens, how its phase moves, when to
reason and which actions are allowed, but nothing static picks the action.
Many cases repeat: the same diagnosis on the same kind of device, answered by
the same intent and preset, and verified to work. A **playbook** turns such a
case into a deterministic **rule** that a human approved, so the next case of
that kind is answered without a model call.

## Decision

Add one module, `internal/playbook`, that owns the rules' runtime state and
evidence, and one executor, `internal/executor/rule`, that turns a matched rule
into a Decision through the existing episode path. The playbook file itself is
compiled by `internal/spec`, beside the SituationSpec.

| Question | Answer |
| --- | --- |
| Responsibility | Decide, for each admitted episode, whether an active rule answers it, an audit re-asks the model, or the model answers; keep each rule's mode, circuit, fires, evaluations and verified cases; decide when a key is eligible for a draft and when a shadow rule may go active. |
| Kind | Business module with tables, reference shape (facade, `internal/app`, `internal/domain`, `internal/store`). |
| Callers | `runtime` (episode admission and the settle step) and `cmd` (`validate`, `serve --playbook`, the rule approval endpoint). |
| Reads through facades | `spec` (compiled spec and playbook), `situations` (the feature map of a snapshot), `episodes` (stored decisions), `cognition` (queue an audit), `notify` (alerts). |
| Never | Writes intents, commands, approvals of intents or another module's lifecycle tables; never changes the spec or policy digest; never calls a model. |

### The playbook file (F1, decision 4)

A playbook is its own file, `<site>.playbook.yaml`, next to the spec in the
customer workspace and never in a repository. `spec.CompilePlaybook(spec,
bytes)` compiles it **against** the compiled spec and seals it under its own
canonical JSON domain, so it has its own digest. The spec digest and the policy
digest derived from it (`policy.DigestForVersion(spec.Digest)`) do not change
when a playbook is added or edited: a rule cannot change policy, and approvals,
commands and gateway allow-lists keep binding to the policy digest. Every rule
evaluation, fire, proposal, approval and run artifact records the playbook
digest. In the first slice the playbook is read at start; a written version
takes effect at the next start.

A rule has an `id`, a `version`, a `diagnosis_code`, a `when` in the existing
restricted CEL, a `recommend` (`intent` and `preset`, no free values), an
`envelope` and a `cap` (per-feature ranges), a `pinned_kind_digest`, a `mode`
(`shadow` or `active`), a `shadow_exit` (default 10 agreeing, 0 disagreeing),
an `audit_rate` (default 1 in 3), a `rate_cap` per hour, and `provenance`
(source episode ids, approval digest).

The compiler refuses, each with a `CompileError` path: a `when` that does not
compile or reads a feature that is neither a spec feature nor a declared
context feature; an unknown intent or preset; a diagnosis code outside the
catalog or equal to `unknown`; an intent at R3 or above, marked `emergency` or
marked `safetyFunction` (decision 17); an envelope outside its cap.

The spec gains `settleWindows` (diagnosis code to duration), `contextFeatures`,
an optional `remedyMap` and the two intent flags. A Decision gains an optional
`diagnosis_code`, checked against the catalog when the spec declares one
(decision 10, F2).

### Layers

| Package | Holds |
| --- | --- |
| `internal/playbook/internal/domain` | Pure rules: matching (exactly one active rule inside its envelope and under its caps, otherwise none), the preset an intent's parameters name, shadow agreement, deterministic audit sampling, circuit causes, the shadow exit, the case key, verification of a case against its settle window, eligibility and the envelope. |
| `internal/playbook/internal/store` | Transactions joined from the caller and every playbook SQL statement; owner of the playbook tables. |
| `internal/playbook/internal/app` | Use cases: route an admission, settle (compare, audit, verify, check exits), record approvals and write the playbook file. |
| `internal/playbook` | Facade: `New(Config)`, `Route`, `Settle` and the approval operations. |
| `internal/executor/rule` | The `rule` executor: reads the rule section of the episode request and returns the Decision; no I/O, no model. |

### Tables

| Table | Holds |
| --- | --- |
| `rule_evaluations` | What a rule would have recommended for a situation version (shadow rules), and the verdict once the model's decision is stored. |
| `rule_fires` | Each active fire (episode, version, time): the rate cap and the audit sample. |
| `rule_circuits` | Circuit state per rule (open or closed), its cause and evidence. |
| `case_verifications` | One record per case, an occurrence of a Situation whose decision dispatched an action: its key (diagnosis code, kind digest, intent, preset), the rule that handled it if any, the features at decision time, the settle window's end, and its verdict and reason (AS-5). |
| `rule_drafts`, `rule_proposals` | Draft calls and the proposals with their replay reports and approvals (AS-7). |

## How it hooks in

### Admission (F3)

Cognition still admits scheduler items exactly as today. When the runtime
admits a due item, it assembles the model request as before and then asks
`playbook.Route` in the **same transaction**, with the request's immutable
snapshot. The feature map is the one cognition evaluates
(`situations.CELFeatures` over the snapshot's facts), so a replay of the same
snapshot routes the same way. Route answers one of:

- **rule**: exactly one active rule matches, inside its envelope, under its rate
  cap, with a closed circuit. The episode is rebound to the `rule` executor:
  executor name `rule`, executor version the playbook digest, dispatch policy
  active, and a `rule` section in the request (rule id, version, playbook digest,
  diagnosis code, intent, preset parameters and the features the rule read).
  The fire is recorded.
- **audit**: the item was queued by the playbook to audit a rule episode. The
  request goes to the model with dispatch policy `shadow`. An audit item whose
  Situation has a live episode waits in the queue for a later pass.
- **model**: anything else. For every shadow rule that matches, a
  `rule_evaluations` row is written.

The `rule` executor builds the Decision from the request's rule section: its
`diagnosis_code`, the intent with the rule's preset, `facts_used` from the
rule's features and a summary naming the rule id, version and playbook digest.
The Decision then takes the existing path: decision validation, policy,
approvals, outbox, effect and verification. The runtime composition routes a
request to the `rule` executor by its executor name; every other request goes
to the configured executor. A rule episode whose situation version is stale
when it is claimed is re-assembled for the live version like any other episode,
so it loses its rule binding and goes to the model: a rule only ever answers the
snapshot it matched.

### Settle

After each batch the runtime calls `playbook.Settle`, which, in order:

1. compares each pending shadow evaluation with the model decision stored for
   its version (diagnosis code, intent, preset) and records agree or disagree;
2. for each concluded rule episode chosen by the audit sample (a pure function
   of the situation version id, so replay reproduces it), asks cognition to
   queue an audit item for the same version once the rule episode has ended
   (one live episode per situation). Any due item whose Situation already has a
   live episode, an audit or an ordinary trigger, is deferred to the next
   admission pass instead of failing the batch;
3. compares each concluded audit with its rule's recommendation; a disagreement
   opens the rule's circuit and publishes a notification;
4. opens a case for each newly dispatched decision and verifies pending cases
   in event time: a case is verified only when its action was acknowledged,
   its Situation resolved and no version that is not resolved appeared on the
   entity before `acted_at + settleWindow(diagnosis code)`; the log's latest
   event time says when the window has passed. A rule-handled case that is not
   verified opens the rule's circuit (`no_effect`) (AS-5);
5. checks shadow exits and eligibility for a draft (AS-7).

### Circuit (decision 5)

A rule whose circuit is open is treated as absent. Causes: `audit_disagreement`,
`rate_cap`, `kind_changed` (AS-4), `no_effect` (AS-5) and `out_of_cap` (AS-6).
Closing a circuit needs a human.

### Kind pinning (decision 3)

Plant wire remembers the kind digest each kind was last born with
(`plantwire.Host.BornKindDigests`, a query the composition hands to the
playbook; no callback). A rule pinned to another digest for that kind opens its
circuit with cause `kind_changed`, at the next routing or settle, so it stops at
the first birth on the new kind, whether that kind is commissioned or
quarantined.

### Drafts and approvals (decisions 1, 6, 9, 11, F4)

The playbook decides when a key (diagnosis code, kind digest, intent, preset)
is eligible and asks the worker through the feature-gated `DraftPlaybookRule`
call; the worker has no access to the workspace. The reply is checked, its
envelope computed and replayed over the database's history, and stored as a
pending proposal. A human approves it through the signed approval endpoint; the
playbook writer then writes the next playbook version, only if the file's
digest is still the one loaded. The rule enters in shadow and becomes active by
a deterministic check of its shadow exit.

## Dependency direction

```text
cmd ──► runtime ──► playbook ──► cognition, episodes, situations, spec, notify
                 └► executor/rule ──► episodes
```

`playbook` imports no `runtime`, `policy`, `actions`, executor or adapter.
`episodes`, `cognition` and `policy` never import `playbook`. The `rule`
executor imports only the episode contract.

## Deferred

A fleet cap per rule (decision 18), envelope growth, walls and weekly coverage
(decision 16), editing a rule before approval and the hand-edit ledger
(decisions 12, 13), a playbook-only reload while running, the remedy map's
enforcement in proposals, and I-13 (lint, test and diff over traces).

## Next reads

- [Rule promotion](rule-promotion.md)
- [Cognition](cognition.md)
- [Decisions and actions](decisions-and-actions.md)
- [Plant wire adapter](plant-wire-adapter.md)
