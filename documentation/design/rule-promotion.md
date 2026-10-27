# Rule promotion: the agentic-stream plan

- **Status**: planned, built in rounds (nothing below is running yet)
- **Date**: 2026-10-27
- **Repository**: agentic-stream, branch `rule-promotion` (cut from main `d9f87164`)
- **Session order**: the repositories are worked one at a time, never in parallel:
  1. streams-simulator: the streams-simulator repository, `docs/rule-promotion/PLAN-streams-simulator.md`
  2. **agentic-stream, rounds AS-0 to AS-8 (this plan)**
  3. tamoz: the tamoz repository, `docs/RULE_PROMOTION_PLAN.md`
  4. **agentic-stream again, round AS-9**: the first-slice run with the real Tamoz worker
- **Background, not required to start**: the streams-simulator repository's `docs/rule-promotion/`
  (`README.md`: the 18 owner decisions; `SCENARIOS.md`: the acceptance campaign; `PLAN.md`: findings F1-F9)

This page is self-contained: a session that reads it and this repository's `AGENTS.md` can do the work. Before
starting, read the simulator plan's §8 (status and handoff) for what session 1 delivered.

## 1. What rule promotion is

Today every decision about *what to do* goes to a Tamoz episode. The SituationSpec has static rules for when a
situation opens, how its phase moves, when to reason and which actions are allowed, but no static rule picks the
action (`intent.baseline.byPhase` is used only by replay and shadow).

Rule promotion adds a **playbook**: deterministic rules, each mapping a recurring, verified kind of case to a
recommended intent and preset. Tamoz drafts a rule from cases it solved and that were verified; Agentic Stream
checks it, replays it and asks a human; once approved and proven in shadow, the rule handles its cases with **no
LLM call**. The LLM handles what no rule covers. The playbook is customer data: a file in the customer workspace,
never in a repository.

The simulator (streams-simulator) proves it: a simulated pump station with sealed truth runs as a campaign of
learning, holdout and disturbance runs against four arms. **This repository owns the campaign driver, the
scripted approvers and the campaign score**; the simulator owns the world and the grading of each run.

## 2. Decisions that bind this repository

| # | Decision |
|---|---|
| 1 | A human approves every new rule; nothing promotes itself. Tamoz never activates a rule. |
| 2 | A rule has a `when` in the existing restricted CEL, an `envelope` over its features, a `recommend` limited to a declared intent and preset (no free values), a `mode` (shadow or active) and provenance. Two matching rules: none fires, the case goes to Tamoz. |
| 3 | Each rule is pinned to the device `kind_digest` it was learned on; a device born with another digest puts the rule back to shadow until a human approves it again. |
| 4 + F1 | The playbook is its own file (`<site>.playbook.yaml`) next to the spec, compiled **against** the spec but with **its own digest**. The spec digest, and the policy digest derived from it (`policy.DigestForVersion(compiled.Digest)`), stay unchanged: a rule cannot change policy, and approvals, commands and gateway allow-lists bind to the policy digest. Every rule decision, proposal, approval and run artifact records the playbook digest. In the first slice the playbook is read at start. |
| 5 | A per-rule circuit switches a rule off on: kind digest change, a sampled audit disagreement, its action having no effect, or most matching cases falling outside its approved cap. Switching back on needs a human. Circuit state lives in the database. Audit rate per rule, default **1 in 3**; the first disagreement alerts a human. |
| 6 | Tamoz may be asked to draft after **3 verified cases** (per-site setting) with the same key and no contradicting case; a replay report goes with every proposal; one pending proposal per key; no new proposal after a rejection without new evidence. |
| 9, 14 + F4 | Agentic Stream decides eligibility and asks Tamoz through a separate, feature-gated **`DraftPlaybookRule`** call; Tamoz has no access to the workspace. Agentic Stream checks the reply, computes the envelope, replays and stores it as pending. |
| 10 + F2 | The case key is the **diagnosis code**, kind digest, intent and preset. A spec keeps one situation type; the decision gains an optional, catalog-checked `diagnosis_code`. `unknown` never becomes a rule; a rule's decision carries the rule's code. |
| 11 | One approval covers the rule and its exit from shadow (for example 10 agreeing shadow matches and none disagreeing); the move to active is a deterministic check. A disagreement in shadow keeps the rule in shadow and asks again. |
| 12, 13 | The human may edit a rule before approving it; hand edits to the playbook file are allowed and recorded; Agentic Stream writes the file only if its digest is still the one it loaded. *(After the first slice.)* |
| 15 | A case is **verified** only when its action took effect and no new situation opens on that entity within the **settle window of its diagnosis code** (for example 24 h for bearing faults, 2 h for overload). An optional remedy map limits which intents may answer which diagnosis. |
| 16 | The envelope covers only the `when` features and declared context features; the approver approves a cap; growth within the cap with walls and weekly coverage come after the first slice. |
| 17 | The playbook compiler refuses any rule that recommends an intent at **R3 or above**, an intent marked **emergency**, or one marked part of a **safety function**. Each rule has a rate cap; reaching it trips the circuit. |
| 18 | A fleet cap per rule (at most N entities in a window; above it the rule fires on none and one fleet-level case is raised). *(After the first slice.)* |
| F3 | An active rule runs as an in-process **`rule` executor** episode, so its decision takes the existing decision, intent, policy and verification path. A rule in shadow writes a `rule_evaluations` record compared with Tamoz's decision. The audit is a `dispatch_policy: shadow` Tamoz episode admitted after the rule's episode ends (one live episode per situation). |
| Scope | Build the **first slice**: scenarios S1 (repeat offenders), S2 (cavitation look-alike), S7 (new probes). The first slice's fault families are `bearing_wear`, `misalignment` and `sensor_drift`, all answered by `schedule_maintenance` with a work order naming the fault, plus `transient_none` (cavitation, no action) as the negative class. |

## 3. Starting state (read on 2026-10-27)

- **Spec**: `internal/spec` compiles one `SituationSpec` (`CompiledSpec.Situation` is a single struct,
  `internal/spec/internal/domain/spec.go:15`); CEL env with `features`, `situation`, `delta`, `event_time`,
  `watermark` (`cel.go:15-28`); `specExpressions` compiles every CEL source (`cel.go:53-65`); `sealSpec` digests
  canonical JSON (`compiler.go:60-68`); `Intent` has `Risk` (R0-R4), `Presets`, `ModelWritableFields`
  (`spec.go:220-231`); errors are `CompileError{Path}`.
- **Policy digest**: derived from the spec digest at start (`cmd/agentic-stream/serve_source.go:44-56`) and
  checked on every command (`internal/actions/internal/app/authorize.go:70-90`); `DCMD`s carry it
  (`internal/plantwire/internal/domain/command.go:154`). The spec is loaded once; there is no reload.
- **Cognition**: triggers evaluate inside the situation-version transaction
  (`internal/cognition/internal/app/service.go:86-97`); `Admit` enqueues a scheduler item.
- **Episodes**: one `Executor` port (`internal/episodes/internal/domain/execution.go:14`) with `fixture`, `native`
  and `remote` (Tamoz over gRPC) in `internal/executor/`; decisions are checked against
  `internal/contractsv1/internal/domain/schemas/v1/decision-v1.json`, which has free-text `primary_hypothesis`
  and no diagnosis code; intents reference decisions, decisions reference episodes; episodes carry
  `dispatch_policy` `active` or `shadow` (`migrations/024_episode_modes.sql`); one live episode per situation
  (`migrations/003_lifecycle_fencing.sql:101`). The diagnosis catalog is passed to the worker and only digested
  in Go (`internal/episodes/internal/domain/assembly_executor.go:37-54`).
- **Rate limits**: hourly per intent (`migrations/022_intent_rate_limits.sql`); nothing per rule.
- **Verification**: read-back of commanded metrics from the device's own readings
  (`internal/plantwire/host.go:160-170`); verdicts verified, refuted, inconclusive
  (`internal/actions/internal/domain/reconciliation.go:112-132`).
- **Kind digests**: per commissioned kind (`internal/plantwire/internal/domain/inventory.go:42`, `kind.go:36`),
  not visible outside `plantwire` (`ingest.go:18-30`).
- **Approvals**: Ed25519-signed through a relay; `GET/POST /v1/approvals/{id}`
  (`cmd/agentic-stream/serve_approvals.go:22-28`); approvals are bound to intents (`approvals.intent_id NOT
  NULL`); authority is entity, risk class and role (`migrations/013`).
- **Replay**: trace replay, recorded mode, shadow mode against a worker (`internal/replay`). I-13 (spec lint,
  test, diff) is **not built**; only `validate` exists.
- **Worker proto**: `proto/agenticstream/runtime/v1/runtime-v1.proto`: `Handshake` with
  `requested_features`/`supported_features`, `Execute` streaming; kinds DIAGNOSE and RECONSIDER only.
- **Experiment tests** already start `serve` in-process and run the simulator binary from `STREAMSIM_BIN`
  (`cmd/agentic-stream/experiment_plantwire_*_streamsim_test.go`); `internal/testsupport` has `plantwirefake`,
  `workerfake`, `executorconformance`.
- **Rotating-machinery example** (`examples/rotating-machinery/`): absolute thresholds, one type
  `bearing_degradation`, ticket intents only. Not reused; the first slice gets a new pump-station example.

## 4. Inputs from session 1 (streams-simulator)

From the streams-simulator repository's `examples/pump-station/` on its `rule-promotion` branch:
`pump-station.domain.json` and `pump-station-probe-b.domain.json` (the world and its probe variant; the right
`schedule_maintenance` work order, equal to the fault id, repairs a fault after about an hour), `kinds/pump.kind.json`
and `kinds/pump-probe-b.kind.json`, the first-slice campaign spec, and the generated campaign folder
(`campaign.json` plus one session plan per run, produced by `streamsim suite campaign`). A run is driven with
`streamsim plantwire connect --plan <run plan> --domains-dir <examples/pump-station> --out <run dir>`, graded
after it ends with `streamsim score` / `streamsim report <run dir>`. Check the simulator plan's §8 for the final
paths, digests and any deviation.

## 5. Rounds

Every round: map the existing path before writing, extend the owning module, keep logic out of `cmd/`, data in
JSON (never Go literals), register any genuinely new package in the architecture gates
(`internal/architecture`) with the kind and layer the rules prescribe, never loosen a gate, write tests at the
layer they prove, run focused tests while working and `make ci-check` before the session ends. One commit per
improvement on `rule-promotion`.

### AS-0 Design note for the `playbook` module (docs, size S)

Write `documentation/design/playbook.md` in the style of `plant-wire-adapter.md`: responsibility, kind and layers,
callers, the facade's use cases, its tables, how it hooks into cognition (admission) and episodes (the `rule`
executor), the F1-F4 decisions above, and what is deferred. Link it from `documentation/design/README.md`. The
design states the dependency direction (who calls `playbook`, what `playbook` reads through which facades) so the
import rules can be set before code.

### AS-1 Spec and contracts (size M)

1. **Playbook compiler** in `internal/spec`: a playbook schema (embedded JSON schema), a parser for
   `<site>.playbook.yaml`, and `CompilePlaybook(compiledSpec, bytes)` returning a compiled playbook with **its own
   digest** (a new canonical-JSON domain). A rule: `id`, `version`, `diagnosis_code`, `when` (CEL), `recommend`
   (`intent`, `preset`), `envelope` and `cap` (per-feature ranges), `pinned_kind_digest`, `mode`
   (`shadow`/`active`), `shadow_exit` (default 10 agreeing, 0 disagreeing), `audit_rate` (default 1 in 3),
   `rate_cap` (per hour), `provenance` (source episode ids, approval digest).
2. **Checks** (each a `CompileError` with a path): the `when` compiles in the existing CEL env and reads only
   allowed features (the spec's features plus its declared `contextFeatures`); the intent and preset exist; the
   diagnosis code is in the catalog and is not `unknown`; **denylist**: the intent's risk is below R3 and it is
   not marked `emergency` or `safetyFunction` (two new optional intent flags); envelope inside cap.
3. **Spec additions**: `settleWindows` (diagnosis code to duration), `contextFeatures`, optional `remedyMap`
   (diagnosis code to allowed intents), the intent flags. The spec digest changes only when these change.
4. **Decision schema**: an optional `diagnosis_code` in `decision-v1.json`; when the spec declares a catalog, the
   decision validator rejects a code outside it. This is a **shared contract**: Tamoz carries a copy in
   `gems/tamoz-stream/contracts/schemas/decision-v1.json` and updates it in its session (TZ-1).
5. `agentic-stream validate --spec <f> --playbook <f>` prints both digests and the unchanged policy digest
   (`cmd` calls one facade function).
6. Tests: compile success and every refusal; the policy digest is identical with and without a playbook; a
   decision with an unknown code is refused when a catalog exists.

### AS-2 The pump-station example (data, size M)

In `examples/pump-station/`:
1. `pump-station.situation.yaml`: one situation type `pump_condition` per pump (`entityKey` the device),
   opening on **per-pump relative features**: for vibration, axial vibration and current, a long-window mean (24 h)
   against a short-window mean (15 min), as a CEL ratio or difference over two operator outputs (the engine
   partitions per entity); a diagnosis catalog with `bearing_wear`, `misalignment`, `overload`, `lubrication_due`,
   `sensor_drift`, `sensor_failure`, `transient_none`, `unknown`; `settleWindows` (24 h for the maintenance faults,
   2 h for `overload`); `contextFeatures` (the load).
2. Intents: `schedule_maintenance` (risk R1, policy automatic, rate-limited) with one **preset per work order**
   (`bearing_wear`, `misalignment`, `sensor_drift`, `sensor_failure`, `lubrication_due`; the work-order value is
   the simulator's fault id, which is what its maintenance responder matches); `reduce_load` (R2, approval);
   `stop_motor` (R3, `emergency`; never a rule target).
3. Plant wire files modelled on `examples/real-world-sensor/testdata/plant-wire/`: the `pump` kind files from the
   simulator, a capability catalog, a site file and an inventory for the 8 pumps.
4. Tests: the spec compiles; an experiment test (skipped without `STREAMSIM_BIN`) runs one pump-station session
   with a `bearing_wear` fault and sees a `pump_condition` situation open on the right pump.

### AS-3 The `playbook` module and the `rule` executor (size L; several commits)

1. `internal/playbook` (facade over `internal/domain`, `internal/app`, `internal/store`) with a migration: rule
   state per rule id and version (mode, pinned kind digest), `rule_evaluations` (what a rule would have
   recommended, per situation version), `rule_circuits` (state, cause, evidence), `rule_fires` (for the rate
   cap).
2. **Matching** (pure domain): evaluate each rule's `when` over the situation version's features; exactly one
   active match inside its envelope and under its caps gives a rule; anything else gives none.
3. **Admission hook**: when cognition admits work for a situation version, ask `playbook` first. A rule match
   admits an episode run by a new **`internal/executor/rule`** executor (no model call): its decision carries the
   rule's `diagnosis_code`, intent and preset, `facts_used` from the rule's features and a summary naming the rule
   id, version and playbook digest; the episode records executor `rule`, the rule reference and the playbook
   digest. No match: the normal Tamoz episode.
4. **Shadow**: for rules in shadow, write a `rule_evaluations` row; when Tamoz's decision for that situation
   version is stored, compare diagnosis code, intent and preset and record agree or disagree. The shadow exit
   (decision 11) is a deterministic check over these rows.
5. **Audit**: after a `rule` episode ends, admit a `dispatch_policy: shadow` Tamoz episode for the same version at
   the rule's audit rate. Sampling must be **deterministic** (from the situation version id), never random, so a
   replay reproduces it. A disagreement trips the rule's circuit and sends a notification through the existing
   notification path.
6. **Circuit** causes in this round: audit disagreement and rate cap. A tripped rule is treated as absent.
7. **Loading**: `serve --playbook <file>` compiles it at start; the run artifact binds the playbook digest next
   to the spec digest (`internal/runartifact`).
8. Tests: matching (one, none, two); the rule executor's decision passes the decision validator and policy; the
   one-live-episode rule holds with audit episodes; deterministic sampling; circuit trips; replay of a run with a
   playbook reproduces it.

### AS-4 Kind pinning (size S-M)

1. A query on the `plantwire` facade: the kind digest of a device; and a notification when a device is born with
   a digest other than the one recorded for it.
2. `playbook` trips (cause `kind_changed`) every rule pinned to the old digest for that device's kind, so the
   rule is back in shadow until approved again.
3. Test with the simulator's two worlds (S7): rules pinned to `pump.kind.json` stop at the first birth on
   `pump-probe-b`.

### AS-5 Verified cases and the settle window (size M)

1. A `case_verifications` record per case: the action was dispatched and acknowledged, the situation closed, and
   **no new situation opened on the same entity before the settle window of the case's diagnosis code ended**.
   A reopen inside the window, or a situation that never closes inside the window, makes the case not verified.
2. Circuit cause `no_effect`: a rule-handled case that is not verified.
3. These records are the only evidence the proposal flow (AS-7) counts.
4. Tests: closes and stays closed (verified); reopens inside the window (not verified); masking (vibration falls,
   the fault goes on and a new situation opens within 24 h) is not verified.

### AS-6 The envelope (size M)

1. From the verified source cases, read the features the rule's `when` uses plus the context features, at
   decision time, from the stored situation versions (`situation_versions.snapshot_json`); the envelope is their
   range per feature; the proposed cap widens it by a spec-declared margin, which the approver may tighten.
2. Matching (AS-3) requires the features inside the envelope; a case inside the cap but outside the envelope goes
   to Tamoz; most matching cases outside the cap trips the circuit (cause `out_of_cap`).
3. Growth within the cap, walls and the weekly coverage report are after the first slice.

### AS-7 Proposal flow and rule approval (size L; several commits)

1. **Eligibility** (deterministic, after each case verification): 3 or more verified cases with the same key
   (diagnosis code, kind digest, intent, preset), no contradicting case (a verified case with the same diagnosis
   code and another intent or preset, or a non-verified case with the same key), no pending proposal for the key,
   and no rejected proposal without newer evidence.
2. **Proto**: add `rpc DraftPlaybookRule(DraftRequest) returns (stream DraftEvent)` to the `EpisodeWorker`
   service, used only when the worker announces the handshake feature `draft_playbook_rule`. `DraftRequest`: up to
   20 cited cases (episode id, diagnosis code, intent, preset, verdict, the features at decision time, the
   summary as untrusted evidence), the current rules with the same key, the allowed feature names, a budget. The
   reply ends in exactly one of `PlaybookProposal` (diagnosis code, intent, preset, `when`, cited ids, rationale;
   no mode, envelope or free values) or `NoProposal` (a reason). Regenerate and keep `make proto-check` clean;
   add a `rule_drafts` ledger (attempts, fences, status, digests); teach `internal/testsupport/workerfake` to
   answer with canned proposals.
3. **Checks on the reply**: the `when` compiles over allowed features; the intent and preset exist and pass the
   denylist; every cited id was sent; **the `when` matches every cited case's recorded features**. Then compute
   the envelope (AS-6).
4. **Replay report** (the first slice evaluates over the database's own history, not a trace export): for every
   past situation version of that kind, would the rule have fired, did the stored decision agree, and what were
   the reopen rates of the source cases.
5. **Approval**: a new approval subject for a playbook rule on the signed approval endpoint, binding the proposal
   digest, the cap, the shadow exit, the audit rate and the rate cap. On approval, the **playbook writer** writes
   the new playbook version atomically, only if the file's digest is still the one loaded; the rule enters in
   shadow. When the shadow exit is met, the writer sets the rule active. In the first slice a written version
   takes effect at the next start.
6. Tests: eligibility edges; every reply check; a fake-worker proposal through approval to a written playbook;
   refusal when the file changed underneath.

### AS-8 The campaign driver (test support, size L)

1. `internal/testsupport/campaign` (registered in the architecture import rules as test support) and a `cmd`
   test `campaign_streamsim_test.go`, skipped unless `STREAMSIM_BIN` and a campaign folder are given.
2. For each run of `campaign.json`: start `serve` with the pump-station spec and the current playbook, start
   `streamsim plantwire connect` with that run's plan, let the scripted approver act, wait for the end, restart
   for the next run (so a written playbook takes effect), and after the run ask the simulator to grade it.
3. **Scripted approvers** through the real signed endpoint, with principals: **careful** (approves a rule only
   when its replay report shows no disagreement and at least the configured coverage; answers after 4 simulated
   hours) and **approve-all**. Both approve actions the policy sends them. Neither ever sees the truth.
4. **Arms**: 0 = a hand-written static playbook (rules written up front, approved, no Tamoz: unmatched cases get
   no action); A = no playbook (Tamoz every case); B = learning with the careful approver; C = learning with
   approve-all.
5. **Model responses** are recorded (a recording wrapper around the `remote` executor and the draft call) so a
   campaign replays without the model.
6. **Campaign score** from the simulator's per-run grading plus this repository's ledgers: LLM calls, tokens and
   cost per handled fault; damage events; false actions; time to recommendation (wall time); approval requests;
   the share of cases handled by rules per phase; decisions replayable from their rule; S1 pass conditions P1-P7,
   S2 and S7 (see the scenarios, summarised in §7).
7. In this session the driver runs end to end with `workerfake` (canned Tamoz answers); the real worker comes in
   session 4.

## 6. Session 4: AS-9, the first-slice run

After session 3 (Tamoz) delivered the draft path: build `streamsim` (simulator `rule-promotion`) and the Tamoz
worker (tamoz `rule-promotion`), run the first-slice campaign for arms 0, A, B and C (A to C three times each),
keep the run folders and the recorded model responses, and write the result, with every pass condition and the
honest reading of it, to the streams-simulator repository's `docs/rule-promotion/RESULT.md` on the
simulator's `rule-promotion` branch. Then the owner decides on merging the three branches.

## 7. First-slice pass conditions (from the agreed scenarios)

- **S1, holdout, arm B against arm A**: P1 LLM calls per handled fault at most 50%; P2 damage events no more; P3
  false actions no more and none caused by a rule; P4 every rule-handled case correct against the truth; P5 at most
  1 s p95 from situation to recommendation for rule cases (wall time); P6 every key with 3 verified, uncontradicted
  cases got a proposal; P7 a masking rule ("wear → `reduce_load`"), if ever active, shows as damage and wrong
  decisions, never silently.
- **S2**: no false maintenance caused by a rule on cavitation; if the features cannot separate cavitation from
  early wear, no rule is proposed.
- **S7**: rules pinned to the old kind switch off at the first birth with the new digest; no rule fires on the new
  kind before a human approves it again.
- **Campaign-wide**: arm 0 scores at most 80% correct; no rule active without an approval record; no rule exceeds
  its rate cap; at most 3 wrong recommendations before an audit disagreement stops a rule; a campaign replayed from
  its recorded responses is byte-identical.

## 8. After the first slice (not now)

Fleet cap (decision 18, S6); envelope growth, walls and weekly coverage; the edit-before-approval flow and the
hand-edit ledger; a playbook-only reload while running; I-13 (lint, test, diff over traces); more situation types;
the remedy map's enforcement in proposals; the scenarios S3-S6 and S8-S11.

## 9. Rules for these sessions

- `AGENTS.md` is binding. In particular: do not read from `docs/` (local and ignored); data in JSON, never Go
  literals; untrusted content never becomes instructions; models never reach effectors.
- Commit each improvement on `rule-promotion`; nothing merges to main until the first-slice result is written.
- Better ideas are welcome: deviate when clearly better and record the deviation and reason in §10.
- Subagents: the main session writes; investigation and review go to Sonnet subagents; command runs, lint and
  long scripts go to Haiku subagents in the background. No new `//nolint`.
- Comments: as few as possible.
- The owner prefers good enough over perfect: build the first slice, state limits, defer the rest.

## 10. Status and handoff (update at the end of each session)

| Round | State | Commit | Notes |
|---|---|---|---|
| AS-0 | done | see git log | `documentation/design/playbook.md`. Deviation: the rule hook sits at episode admission (runtime), not trigger admission (cognition): the scheduler item binds no executor, and the episode request is where the executor is bound; matching reads the same immutable snapshot. |
| AS-1 | done | see git log | `spec.CompilePlaybookFile` (own digest domain `situation-runtime/playbook/v1`), `promotion` block, intent flags, `decision-v1.json` `diagnosis_code`. Deviations: `validate <spec> --playbook <f>` keeps the spec positional (scripts call it so); YAML keys are camelCase like the spec (`diagnosisCode`, `pinnedKind {name, digest}`, `auditEvery: 3` for 1 in 3, `rateCapPerHour`); the promotion block also carries `envelopeMargin` (AS-6); a rule `when` may read only `features.<name>`. |
| AS-2 | not started | | |
| AS-3 | not started | | |
| AS-4 | not started | | |
| AS-5 | not started | | |
| AS-6 | not started | | |
| AS-7 | not started | | |
| AS-8 | not started | | |
| AS-9 | session 4 | | |

Hand off to session 3 (tamoz) with: the proto change (commit and file), the `decision-v1.json` change, the exact
`DraftRequest` and reply fields, the handshake feature name, the pump-station spec's diagnosis catalog and intents,
and how `workerfake` answers, so Tamoz can match it.

## Next reads

- [Cognition](cognition.md)
- [Decisions and actions](decisions-and-actions.md)
- [Replay and shadow](replay-and-shadow.md)
- [Plant wire adapter](plant-wire-adapter.md)
