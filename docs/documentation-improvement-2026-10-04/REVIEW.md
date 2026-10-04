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

## Review method

After the edits, perform separate completeness, correctness, code-alignment,
writing/visual, and newcomer-readiness passes. Record the evidence and any
repair in this file. These passes are performed by the editing agent; there is
no claim of an independent reviewer or a new production release gate result.

## Next reads

- [Improvement plan](PLAN.md)
- [Quality bar](QUALITY_BAR.md)
- [Documentation home](../../documentation/README.md)
