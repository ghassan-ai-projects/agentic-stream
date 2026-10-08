# U23 — Inspect an episode, an intent and a command

Status: done · Decision: **complete** · Priority: P2 · Size: M · Depends on: U22

## Finding

The MVP's explainability item covers "action outcome" as well as Situations and
triggers. The chain episode → attempts → Decision → Intents → policy evaluation
→ approval → command → outcome → verification is durable but has no reader
outside tests and the run export. `watch_fires` and `notification_audits` are
written and never read. TECHNICAL_DESIGN §15.2 promises `episode show`.

## Decision and reasoning

Complete it with two read-only commands that follow the chain from either end:

```text
agentic-stream episode show --db <db> <episode-id> [--json]
agentic-stream intent show  --db <db> <intent-id>  [--json]
```

- `episode show`: snapshot digest and Situation version, attempts with fence and
  terminal reason, cost, the Decision, and its intents with their policy status.
- `intent show`: policy evaluations with reasons, approval request and
  resolution, command, outcome, verification, and any watch the command
  installed with its fires.
- `commands list` already comes from U18; both commands link to it by ID.

Kept separate from U22 so each lands as one reviewable commit with its own
module read ports (episodes/episodeledger; policy/approvalledger/actions/watch).

The design's `intent approve|deny` CLI is **not** added: approval is a signed
HTTP flow with a relay and an independent approver, and a CLI shortcut would
bypass that separation ([PLAN_CHANGES](../PLAN_CHANGES.md) P02).

## Done when

- Test: for an approved ticket intent in the predictive-maintenance flow, the
  output names the approval, the command, its outcome and verification.
- `watch_fires` has a production reader.
- The limitations page's "Operator inspection" section reflects U16, U18, U22 and
  U23, or is removed if nothing internal-only remains.

## Result

`agentic-stream episode show` and `intent show`, read-only. New read ports, each
behind its module's facade and app layer: `episodeledger.Episode` (episode,
attempts, rejections), `episodes.Decisions`, `policy.Intent`/`DecisionIntents`
(with policy evaluations), `approvalledger.Approvals`, `actions.IntentCommands`
(commands, outcomes, verifications) and `watch.Watch` (condition and
`watch_fires`). The watch is found through the command outcome's
`provider_result.watch_id`, which is how the watch effector reports it.

Test: the experiment end-to-end test follows its dispatched device intent from
`episode show` to `intent show`, which names the policy evaluation, the
command, its outcome and its verification. The experiment flow has no approval
step, so approvals are covered by the store read, not the end-to-end test.
