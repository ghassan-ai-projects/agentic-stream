# Documentation improvement review — 4 October 2026

Audience: maintainers. Scope: public documentation clarity, teaching, visuals,
source alignment, and runnable local proof. This is an editorial self-review;
it is not independent release approval or hardware qualification.

## Baseline and findings

The worktree was clean on branch `improve-doumentation`. Existing docs-check
passed for 59 public Markdown pages. The current source, rather than the older
review or release-status date, supplies the evidence for this pass.

| ID | Severity | Finding | Planned resolution |
| --- | --- | --- | --- |
| D1 | P1 | No sequential learning path for concepts, how, and why | Add `learn/` with one question per page |
| D2 | P2 | Entry diagrams cross too many boundaries at once | Start with four stages; split detailed flows |
| D3 | P2 | Design summaries give mechanics with little rationale | Add concrete examples and tradeoffs |
| D4 | P2 | Overview/status omit live socket ingress already in CLI | Align overview, contracts, manifest, and roadmap |
| D5 | P2 | Migration reference and durability say 28; source has 30 | Correct count and explain added families |
| D6 | P2 | Quickstart lacks concrete output interpretation and copyable replay comparison | Verify examples; show expected counters and hash comparison |
| D7 | P2 | Troubleshooting refers to a supported redrive path without saying it is internal | State tooling boundary directly |
| D8 | P3 | Concept glossary is dense and difficult to scan | Use plain definitions, examples, and deeper links |
| D9 | P1 | Proxy wording implies non-loopback CLI binding can be enabled | State unconditional loopback restriction and proxy forwarding |
| D10 | P2 | Event catalog omits Shipment and Thermal zone families in current JSON registry | Add source-verified family summaries |

## Reader acceptance questions

Each question needs a linked answer, not just a matching term.

1. Why keep stream processing separate from model reasoning?
2. How does a sensor reading become a Situation?
3. What is the difference between a Situation and one of its versions?
4. Why distinguish event time from arrival time?
5. What does a watermark mean, and what happens to late evidence?
6. What makes the scheduler start or delay an episode?
7. What happens when the snapshot becomes stale while a worker is reasoning?
8. How do a Decision, Intent, Command, and outcome differ?
9. Why does a timeout require reconciliation instead of an automatic retry?
10. Why start with one process and SQLite, and what are the limits?

## Round 1 — audit and acceptance contract

Reviewed the public map, overview, learning gaps, design summaries, quickstart,
walkthrough, operations, references, existing validator, fixture, migration
inventory, CLI source, scheduler timing, and synthetic pipeline tests.
Historical design decisions were read for rationale; current code and tests
remain the authority for implemented behavior.

Validation: baseline `make docs-check` passed. The round adds acceptance gates
and a bounded plan; no product capability or release status is changed. Text and visuals are the
priority; diagram detail increases gradually. Working records are kept here
under `docs/`, as requested, rather than in the public reading path.

## Round 2 — guided learning and first visual

Added a seven-page learning path, rewrote the glossary for quick lookup, and
linked the path from public home, overview, getting-started, and design.
The learning pages answer one question each and use a concrete motor story.
The first diagram has four stages; the later learning diagrams have four or
five nodes and explain one boundary at a time.

Reviewed claims against the motor fixture, pipeline synthetic test, scheduler
not-before/cooldown implementation, migration inventory, existing contracts,
and replay tests. D1 and D8 are closed. D2/D3 are closed for the learning path;
existing design diagrams and summaries are addressed in the next round.

Visual review: rendered the SVG at 720 px and inspected it. Long labels crowded
the last two boxes; shortened them to “What next?”, “Action”, and “Checked
outcome”. Shapes, arrows, and labels remain understandable without color.
The SVG has title/description, a Mermaid source, and prose equivalents on both
pages embedding it. New learning pages are all under 180 lines.

Validation: `make docs-check` passes for 66 public Markdown pages;
`git diff --check` passes. Runtime behavior was not changed. A focused source
regression suite and executable examples are being checked for round 3.

## Round 3 — design clarity, source alignment, and executable proof

Split noisy architecture, security, and action diagrams into bounded questions.
Simplified the worker sequence and the replay boundary. Added rationale and
tradeoffs to the design pages. Collapsed the advanced dependency map, with
short package labels and an ownership table for the details.

Corrected live socket ingress, device-boundary qualification wording, migration
count/head, catalog families, redrive tooling boundaries, and loopback/proxy
behavior. Updated the public status manifest without changing the unreleased
posture. Improved authoring instructions and quickstart outputs; the runnable
examples now use fresh temporary databases. Removed duplicate audience-link
lists from the home page so its three starting points and topic map have clear,
different jobs.

D2–D7, D9, and D10 are closed. No recorded P0/P1/P2/P3 finding remains open.

### Reader acceptance results

| Question | Answer location | Result |
| --- | --- | --- |
| 1. Why separate stream processing and reasoning? | [Purpose](../../documentation/learn/why.md) | Explains backlog, time rules, repeatability, and bounded attention |
| 2. How does a sensor reading become a Situation? | [Worked story](../../documentation/learn/how-it-works.md) | Follows validation, features, reduction, and publication |
| 3. Situation versus version? | [Time and state](../../documentation/learn/time-and-state.md) | Current state changes; published versions remain immutable |
| 4. Event time versus arrival time? | [Time and state](../../documentation/learn/time-and-state.md) | A two-reading illustration explains delayed arrival |
| 5. Watermark and late evidence? | [Time and state](../../documentation/learn/time-and-state.md) | Explains progress, missing evidence, and explicit correction policies |
| 6. Start or delay reasoning? | [Reasoning](../../documentation/learn/reasoning.md) | Separates trigger eligibility from queue timing and admission |
| 7. Stale reasoning? | [Reasoning](../../documentation/learn/reasoning.md) | Explains supersession, cancellation, attempt fencing, and current-state checks |
| 8. Decision, Intent, Command, outcome? | [Proposals](../../documentation/learn/safe-actions.md) | Table gives each record a meaning and next owner |
| 9. Timeout and reconciliation? | [Proposals](../../documentation/learn/safe-actions.md) | Lost reply can hide success; provider identity and evidence matter |
| 10. One process and SQLite? | [Design choices](../../documentation/learn/design-choices.md) | Explains local atomicity, bounded concurrency/capacity, and deferred distribution |

### Five review lenses

These are separate self-review passes, performed after writing. They do not
claim independent reviewer approval required for a later release.

| Lens | Evidence and repairs | Result |
| --- | --- | --- |
| Completeness | All ten reader questions have linked answers; existing references/contracts/operations remain reachable | Pass |
| Correctness | Reviewed fixture behavior, source timing, migration files, event registry, local-ingress and loopback source; fixed drift | Pass |
| Code alignment | Built binary, ran exact guide snippets, compared replay reports, ran focused package suites | Pass |
| Writing and visuals | Plain definitions and concrete story; little math; browser-rendered diagrams; long labels repaired; removed duplicate home lists | Pass |
| Newcomer readiness | Home provides learn/run/reference entries; glossary links deeper explanations; examples show expected results and proof limits | Pass |

### Browser validation

Rendered all 15 public Mermaid diagrams with Mermaid 11 in a temporary local
browser preview, plus the overview SVG. Every diagram rendered successfully.
Inspected them in the browser for label size, arrows, wrapping, and layout.
The initial pass found overlong horizontal labels and a compressed worker
sequence; shortened labels, reduced the worker sequence to its three owners,
and rendered and inspected the diagrams again. No parser error or crowded
label remained in the final inspected set.

Also rendered the current Markdown home, learning index, and action learning
page. Checked the first visual with surrounding prose and the learning table.
The preview is verification tooling, not a published website or a committed
rendering pipeline. Markdown and SVG/Mermaid sources remain the deliverables.

Saved browser evidence:

- [Documentation home](evidence/documentation-home-browser.jpg)
- [Action explanation and diagram](evidence/action-learning-browser.jpg)

### Executable validation

| Check | Observed result |
| --- | --- |
| `make build` | Pass |
| Motor `validate` | `motor_bearing_degradation`, `0.1.0`, `agentic-stream/v1` |
| Replay twice in fresh temporary databases | One processed event and one Situation version in each; identical history hash |
| Local live opening batch | One ingested/processed event; zero admitted/executed episodes, evaluated Intents, or dispatched Commands |
| Exact quickstart shell blocks | Pass, including report comparison |
| Exact walkthrough run block | Pass |
| Runtime, replay, cognition, episodes, policy, actions, engine, eventlog, spec tests | All nine packages pass |
| CLI package tests | Pass after rerunning with local listener permission; initial sandbox run could not bind loopback |
| Ingress, device, authority, qualification, migrations tests | All five packages pass with local socket permission |
| `make docs-check` | 66 public Markdown pages and volatile surfaces pass |
| Local Markdown heading links | All ten explicit heading targets resolve |
| Learning page focus | All seven learning pages are 52–83 lines, below 180 |
| `git diff --check` | Pass |
| Scope audit | Only `documentation/` and this working record folder under `docs/` changed |

The current verified replay history hash is
`7c34206dfdce8fee3f2529d7d8a08b0b516b778cd32c636a5b830088c93519b4`.
This is evidence from this run, not a new fixed contract in the tutorial.

### Completion boundary

The improvement-pass quality bar is met. No runtime, root README, schema,
fixture, existing archive source, or validation-tool change is included.
Full `make ci-check`, lint, and whole-repository tests were not run: this is a
documentation-only change with a build, executed examples, and focused tests
for the claims touched. Independent release reviews, deployment readiness,
and physical hardware qualification remain separate, unchanged gates.

## Review method

After the edits, perform separate completeness, correctness, code-alignment,
writing/visual, and newcomer-readiness passes. Record the evidence and any
repair in this file. These passes are performed by the editing agent; there is
no claim of an independent reviewer or a new production release gate result.

## Next reads

- [Improvement plan](PLAN.md)
- [Quality bar](QUALITY_BAR.md)
- [Documentation home](../../documentation/README.md)

## Round 4 — complete core concepts against the domain and code

The user clarified that “inconsistencies” was a mistaken word: the goal is a
more complete core concept explanation reflecting the domain and current code.
This round therefore adds the missing model and relationships, rather than
limiting the work to terminology repairs.

### Coverage and reader checks

| Reader question | Public explanation | Current implementation evidence |
| --- | --- | --- |
| What is the difference between a motor and its condition? | [Domain model](../../documentation/learn/domain-model.md) | `internal/situations/situations.go`: entity/type and Situation identity |
| What separates a spec, deployment, occurrence, and version? | Domain identity table and occurrence boundary | `internal/spec/deployments.go`; `newSituation` and `openOccurrence` |
| How do observations become features and retained facts? | Evidence → features → facts explanation with actual motor names | Motor spec reducers; `internal/situations/reducers.go` |
| Why do phases remain stable across small changes? | Hysteresis and minimum-duration explanation | Motor opening/closing conditions; `applyTransition` |
| Does every fact update publish a snapshot? | Current state versus publication | `internal/situations/evaluate.go` and `materialize.go` |
| Do completeness and confidence mean the same thing? | [Time/state](../../documentation/learn/time-and-state.md) status table and domain limitations | Feature statuses, reducer copy behavior, confidence initialization |
| Does a schema field establish an enforced cognitive gate? | Explicit completeness-field boundary | `internal/cognition/engine_trigger.go`: actual gate ordering |
| What is material delta compared with? | [Reasoning](../../documentation/learn/reasoning.md) | `markVersionReasoned` advances after evaluation regardless of outcome |
| How can one episode use a newer version without changing history? | Attempt/rebinding explanation | ADR-013; runner claim/assembler; `rebind_test.go` |
| Does coalescing merge requests or cancel work on every publication? | Learning and cognition controls distinguish replacement from publication | Scheduler supersession and schedule/episode ledgers |
| Does correction undo a succeeded effect? | Correction/reconsideration/compensation distinction | Reconsideration selection, per-Command deduplication, governed Intent checks |
| What separates stream state, reasoning output, and effect authority? | [Ownership](../../documentation/learn/runtime-boundaries.md) | Architecture boundaries and package owners |
| How do targets, capabilities, sessions, and authority relate? | Entity/target explanation and physical qualification boundary | `internal/device`, `internal/authority`, control dispatch gate |
| What do leases, fences, epochs, and watches establish? | Recovery table and bounded follow-up explanation | Episode/control owners; watch firing and expiry rules |
| What is explainable history versus deployment qualification? | Ownership page and existing replay/observability pages | Notifications, outcomes, qualification owner, release posture |

All fifteen questions now have an accessible public answer and implementation
evidence. The glossary adds definitions and ownership links; the learning
path adds two focused pages. Existing action/replay explanations remain the
owners of their detailed contracts. No new math section is introduced.

### Review repairs and boundaries

The source review refined publication behavior, replacement/coalescing, and
snapshot rebinding rather than presenting each as an unconditional pipeline
step. It also distinguishes a reason to reconsider a succeeded Command from
a proof that the action was wrong, and compensation from automatic reversal.

Explicit current limits are documented: no automatic fresh occurrence after
resolution, no calibrated Situation-confidence update, no changing primary
hypothesis tracking for subsequent versions, and no independent enforcement
of the trigger completeness field. This closes explanation gaps; it does not
implement those missing runtime capabilities.

A writing review caught and repaired an incorrect illustrative reduced-fact
name: the motor spec declares `facts.vibration_rms`. Another review clarified
that material delta uses the most recently evaluated version, even if no
episode ran. The new relationship diagram has only three nodes, and its text
equivalent explains that relationships are not event-processing guarantees.

The worktree already contained a first-line edit to the core-concepts heading
before this round. Preserve that user edit in the worktree and exclude it from
the commit; the staged documentation retains the committed heading.

### Validation and acceptance

| Gate | Result |
| --- | --- |
| Domain/code coverage and ownership | All fifteen questions covered; current limitations linked |
| Plain writing and page focus | Nine learning pages; every page under 180 lines (longest 134) |
| Browser review | New domain diagram rendered with Mermaid 11; labels/arrows fit at the preview's 780 px reading width; inspected new ownership page and expanded learning table |
| Browser evidence | [Core domain explanation](evidence/core-domain-browser.jpg) |
| Documentation check | `make docs-check`: 68 public Markdown pages and volatile surfaces pass |
| Local heading links | All thirteen explicit Markdown heading targets resolve |
| Regression evidence | Uncached full suites pass for situations, operators, cognition, episodes, policy, actions, replay, spec, watch, control, authority, qualification |
| Whitespace | `git diff --check` passes |
| Scope | Reader pages only under `documentation/`; plan, bar, review, and evidence only in this working folder |

The core concept completion gate is met. These are self-review passes; no
independent approval or production qualification is claimed. Executable guide
commands were unchanged, so their Round 3 evidence was not rerun. Full CI and
whole-repository tests were not run for this documentation-only round.
