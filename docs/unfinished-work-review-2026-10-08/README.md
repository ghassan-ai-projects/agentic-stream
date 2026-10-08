# Unfinished work review (2026-10-08)

Status: **review complete, implementation not started.** Branch:
`finish-the-not-done-features`, base `be37f6b`.

The reference-module refactor (2026-10-05 to 2026-10-06) showed that many
planned capabilities were implemented and tested but never connected to a
production path. Others are only design text. This folder records, for each
one, whether to **complete** it, **delete** it from the code, or **remove** it
from the plans, and why. It is broken into small tasks so each can be built,
reviewed and tracked on its own.

It supersedes the "owner decision" columns of
[remaining-migration-2026-10-06/DEADCODE.md](../remaining-migration-2026-10-06/DEADCODE.md),
[DEADCODE_REPORT.md](../remaining-migration-2026-10-06/DEADCODE_REPORT.md) and
[reference-module-migration-status-2026-10-06/DEADCODE.md](../reference-module-migration-status-2026-10-06/DEADCODE.md):
the decisions are made here.

## The experiment comes first

The real-world-sensor experiment (`agent-research-lab/real-world-sensor`, with
Tamoz and Streams Simulator) is the product's core proof. Two documents govern
everything below:

- [EXPERIMENT_DESIGN.md](EXPERIMENT_DESIGN.md): what blocks the live closed loop
  (sensor → Situation → Tamoz → governed command → board → verification) and
  the decided solution. **The core finding:** policy, approval and dispatch
  treat every new Situation version as making an intent stale, so the loop
  closes only when the feed pauses. A live sensor never pauses. The fix (D1)
  is to use cognition's existing material-change rule.
- [EXPERIMENT_COMPATIBILITY.md](EXPERIMENT_COMPATIBILITY.md): what the
  experiment uses from this repository, two breaks that are already on `main`,
  and the impact of every task. No task ships if it breaks the experiment;
  [X01](tasks/X01-experiment-compatibility-guard.md) enforces this in CI.

## Working rules (owner, 2026-10-08)

- **This repository first.** Tasks in Tamoz and the research repository (X02,
  X04, X06, and the bench half of X07) wait until the Agentic Stream tasks
  are done.
- **Modularity.** Every change stays inside the module that owns it. Other
  modules use only its facade; no implementation detail leaks through an
  export, a shared file read or a cross-module SQL join. Tests follow the same
  rule: a pin or fixture lives in the module that owns the surface.
- **One commit per round**, with focused tests, lint and the architecture
  gates green before each commit.

## How the findings were collected

1. **Unreachable from `main`.** `deadcode ./...` without `-test` lists every
   function that no production path reaches: **189** at `be37f6b` (raw output:
   [deadcode-2026-10-08.txt](deadcode-2026-10-08.txt)). `deadcode -test ./...`
   reports **0**, so every one of them runs only in tests. Each symbol is mapped
   to a task in [SYMBOL_MAP.md](SYMBOL_MAP.md). The tool cannot see reflection,
   so every classification was confirmed by reading the callers.
2. **Tables without a writer or reader.** Every `CREATE TABLE` in `migrations/`
   was checked for production `INSERT/UPDATE/DELETE` and `SELECT/JOIN` sites.
3. **Design surfaces that were never built.** `docs/design/` (README MVP list,
   TECHNICAL_DESIGN §14–16 and §24, IMPLEMENTATION_PLAN M3.5, BUILD_COMPLETION_BAR),
   the design contract schema, `documentation/` status pages and
   `release-status.json` were compared with the CLI, the HTTP routes and the
   embedded runtime schema.
4. **Markers in code.** `placeholder`, `not implemented` and `TODO` in production
   Go files.

## Decision rules

A capability is **completed** when the design names it as a release-blocking
invariant, an MVP acceptance item or a safety lever, *and* most of the code
already exists. Without a production path it is untested-in-production code on a
safety path, which is worse than having no code.

It is **deleted** when nothing in the design requires it, nothing produces its
input, or the production path already covers the same need another way. Its
tests go with it. Deleting is preferred to keeping "for later": git history
keeps the code.

It is **moved** when it is real test support (a fake worker, fixtures) that
lives in a production package. Test support goes in `internal/testsupport/` or a
`<module>test` subpackage, so production packages contain no test-only
functions.

It is **removed from the plans** when the design promises a surface that has no
code, no current consumer and no acceptance item. The design then describes what
exists plus a short, explicit deferred list (TECHNICAL_DESIGN §24).

Every operator command added here follows one rule ([U13](tasks/U13-operator-command-foundation.md)):
an offline command that mutates runtime state first claims the runtime owner
lease, so it can never race a running `serve`/`run-live`. The interlock **trip**
is the only exception: it only blocks, so it must work when the owner is hung.

## Headline findings

1. **Safety bug: `policy: deny` and `policy: simulate` fail open.** The spec
   schema accepts four intent policies, but only `approval` reaches the policy
   plane. An R0/R1 intent declared `deny` is dispatched automatically.
   [U01](tasks/U01-reject-unenforced-intent-policies.md) fixes this first.
2. **The governed approval path cannot be used without hand-written SQL.**
   Nothing writes `principals`, `roles`, `principal_roles` or
   `approval_authorities`, so every approval resolution fails in a fresh
   deployment ([U15](tasks/U15-approval-principal-provisioning.md)). R2 intents
   always need approval, because the calibrated-automation route reads a table
   nothing writes ([U24](tasks/U24-remove-calibrated-automation-route.md)).
3. **The emergency stop has no trigger.** `interlock.Set` has no production
   caller, so the final dispatch gate always passes ([U14](tasks/U14-interlock-operator-command.md)).
4. **Two MVP acceptance items exist only in tests**: "reproduce the accepted
   decision with recorded cognition" and "run a new model or prompt in shadow
   mode" (93 replay symbols, [U20](tasks/U20-recorded-replay-mode.md),
   [U21](tasks/U21-shadow-replay-mode.md)). The HIL Phase 02 plan claims the
   shadow path is implemented; as a library it is, but no operator can run it.
5. **Operator tooling the release status calls "partial" has no entry point**:
   quarantine release/redrive, notification retention, manual reconciliation of
   unknown outcomes, and explainability over the audit tables, which are
   currently write-only.
6. **The design promises more than the runtime accepts**: 9 operator,
   aggregate, reducer and window kinds plus `retention`/`telemetry` spec blocks,
   12 CLI commands and about 15 HTTP endpoints that do not exist, a
   content-addressed artifact store, and three tables nothing uses.

## Task board

Status values: `todo`, `in progress`, `done`, `dropped` (with a reason in the
task file). Update the row and the task file's status line in the same commit
as the change.

| ID | Task | Decision | Priority | Size | Depends on | Status |
| --- | --- | --- | --- | --- | --- | --- |
| X01 | [Experiment compatibility guard](tasks/X01-experiment-compatibility-guard.md) | Complete | P0 | M | — | done |
| X02 | [Repair experiment references](tasks/X02-repair-experiment-references.md) | Fix references (research repo) | P0 | XS | — | todo |
| X03 | [Material freshness (ADR-018)](tasks/X03-material-freshness.md) | Complete | P0 | M | X01, ADR | done |
| X04 | [One thermal intent vocabulary](tasks/X04-thermal-intent-vocabulary.md) | Tamoz change + pin | P0 | S | — | pin done; Tamoz pending |
| X05 | [Checked-in experiment specs](tasks/X05-checked-in-experiment-specs.md) | Complete | P0 | S | X01 | done |
| X06 | [Bench mapping and heartbeat](tasks/X06-bench-mapping-and-heartbeat.md) | Research repo config | P0 | S | X05 | todo |
| X08 | [The live pipeline advances on a clock](tasks/X08-live-pipeline-clock.md) | Fix (found by X01) | P0 | S | — | done |
| X09 | [Episodes run beside ingestion](tasks/X09-episodes-beside-ingestion.md) | Complete (design §11.4–11.5) | P0 | M | X03 | done |
| X07 | [Joined rehearsal: simulator, then bench](tasks/X07-joined-rehearsal.md) | Run the proof | P0 | M | X03–X06, U15 | todo |
| U01 | [Reject unenforced intent policies](tasks/U01-reject-unenforced-intent-policies.md) | Fix | P0 | S | — | done |
| U02 | [Delete counterfactual replay](tasks/U02-delete-counterfactual-replay.md) | Delete | P2 | S | — | done |
| U03 | [Delete eventlog gap writer and map quarantine](tasks/U03-delete-eventlog-gap-writer-and-map-quarantine.md) | Delete | P2 | S | — | done |
| U04 | [Delete engine per-partition run path](tasks/U04-delete-engine-partition-run-path.md) | Delete | P2 | S | — | todo |
| U05 | [Delete native batch runner and artifact store](tasks/U05-delete-native-batch-runner-and-artifact-store.md) | Delete | P2 | S | — | todo |
| U06 | [Move reference worker server to test support](tasks/U06-move-reference-worker-server-to-testsupport.md) | Move | P2 | M | — | todo |
| U07 | [Remove test-only facade exports](tasks/U07-remove-test-only-facade-exports.md) | Delete / move | P2 | M | — | todo |
| U08 | [Resolve unused exported constants](tasks/U08-resolve-unused-exported-constants.md) | Delete / keep | P3 | XS | — | todo |
| U09 | [Delete `config effective` placeholder](tasks/U09-delete-config-effective-placeholder.md) | Delete | P3 | XS | — | todo |
| U10 | [Define the primary-hypothesis delta key](tasks/U10-remove-primary-hypothesis-delta-key.md) | Keep key, drop from plan (revised for the experiment) | P3 | XS | — | todo |
| U11 | [Drop unused tables](tasks/U11-drop-unused-tables.md) | Delete | P3 | S | U02 | todo |
| U12 | [Dead-code gate](tasks/U12-deadcode-gate.md) | Complete | P1 | S | U02–U07 | todo |
| U13 | [Operator command foundation](tasks/U13-operator-command-foundation.md) | Complete | P1 | S | — | done |
| U14 | [Interlock trip/clear command](tasks/U14-interlock-operator-command.md) | Complete | P1 | S | U13 | done |
| U15 | [Approval principal provisioning](tasks/U15-approval-principal-provisioning.md) | Complete | P0 (experiment: R2 fan approval) | M | U13 | done |
| U16 | [Quarantine list/release/redrive](tasks/U16-quarantine-operator-commands.md) | Complete | P1 | M | U13, U03 | todo |
| U17 | [Notification retention command](tasks/U17-notification-retention-command.md) | Complete | P1 | S | U13 | todo |
| U18 | [Manual command reconciliation](tasks/U18-manual-command-reconciliation.md) | Complete | P1 | M | U13 | todo |
| U19 | [`run --repeat N`](tasks/U19-replay-repeat-flag.md) | Complete | P2 | XS | — | todo |
| U20 | [Recorded replay mode](tasks/U20-recorded-replay-mode.md) | Complete | P1 | M | U02 | todo |
| U21 | [Shadow replay mode](tasks/U21-shadow-replay-mode.md) | Complete | P1 | M | U20 | todo |
| U22 | [Explain situation and trigger](tasks/U22-explain-situation-and-trigger.md) | Complete | P1 | M | U13 | todo |
| U23 | [Inspect episode, intent and command](tasks/U23-inspect-episode-intent-command.md) | Complete | P2 | M | U22 | todo |
| U24 | [Remove the calibrated-automation route](tasks/U24-remove-calibrated-automation-route.md) | Remove from plan + code | P2 | S | — | todo |
| P01–P08 | [Plan and documentation changes](PLAN_CHANGES.md) | Remove from plans | P2 | S each | per row | todo |

Sizes: XS under an hour, S about half a day, M one to two days.

## Suggested order

1. **X01 and X02**: protect the experiment and repair the two existing breaks.
   X01 then runs on every later commit.
2. **The experiment path, X03 with X09, X05, U13, U15, then X07**: material freshness,
   one intent vocabulary, checked-in specs, bench mapping, approval
   provisioning, then the joined rehearsal in the simulator and on the bench.
3. **U01** (fail-open policy values), small and independent; it can land any time.
4. **Gate-related tasks for the experiment's later gates**: U21 (shadow, G3),
   U14 (software interlock, G4c), U18 (manual reconciliation for fault runs).
5. **Cleanup, U02–U11 and U24**, then **U12** (dead-code gate). These are
   independent, one commit each, and must keep X01 green. They take 75 of the
   189 unreachable functions out of production packages.
6. **The rest**: U16, U17, U19, U20, U22, U23, and P01–P08 alongside the task
   they belong to.

After U02–U21, `deadcode ./...` should list only the allow-listed
`internal/testsupport/` packages.

## Not in scope here

Structural follow-ups from the migration (owner read ports, typed records,
transaction scope, layer table) stay in
[FOLLOW_UPS.md](../reference-module-migration-status-2026-10-06/FOLLOW_UPS.md).
Two items there are correctness gaps rather than unfinished features and should
be scheduled next to U01: **#3b** (owner-lease time encodings compared as text)
and **#3** (recovery silently skips cost release when no settler is configured).

Open owner decisions that affect tasks here:

- **ADR-016** (Tamoz is the sole production reasoner) is still *proposed*. U21
  works with any conformant worker, but the HIL G3 claim needs Tamoz as the
  shadow side. If ADR-016 is accepted, the OpenAI-compatible native provider
  wired into `serve --model-endpoint` stops being a production path and becomes
  another cleanup task.
- **Interlock clear fencing** (U14): the recommendation is owner-fenced clear and
  unfenced trip; confirm before implementing.

## Files

- [EXPERIMENT_DESIGN.md](EXPERIMENT_DESIGN.md): closed-loop gaps and decisions.
- [EXPERIMENT_COMPATIBILITY.md](EXPERIMENT_COMPATIBILITY.md): what must not break.
- [SYMBOL_MAP.md](SYMBOL_MAP.md): all 189 unreachable functions → task.
- [PLAN_CHANGES.md](PLAN_CHANGES.md): what to remove or rewrite in the plans.
- [tasks/](tasks/): one file per task.
- [deadcode-2026-10-08.txt](deadcode-2026-10-08.txt): raw `deadcode ./...` output.
