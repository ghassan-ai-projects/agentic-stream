# Documentation improvement plan

Audience: maintainers. Scope: review, writing, visuals, validation, and commits.


This pass improves the existing public set. Reader-facing edits are confined to
`documentation/`. This working folder under `docs/` owns the plan, quality bar,
and review records; source code, root entrypoints, and existing archive files
are read-only inputs. Preserve reference ownership rather than copying flag tables into
learning pages.

### Review findings

The existing 59-page set passes `make docs-check`, but structural checks do not
prove that a reader understands the system. The concepts page is primarily a
vocabulary list, the home page offers many links before a short explanation,
and some flow diagrams introduce ten or more nodes at once. Design summaries
need concrete examples and explicit tradeoffs. A source audit also found stale
migration counts and overview descriptions that omit live socket ingestion.

Why is the reading path hard? It starts with runtime terms. Why are those terms
hard? Definitions arrive before a concrete problem. Why is the problem hard to
follow? A single diagram crosses several boundaries. Why does that persist?
The existing bar checks coverage and correctness more precisely than teaching.
Why change the structure? A focused learning path gives explanations one owner
and leaves reference pages responsible for exact contracts.

### Committed rounds

| Round | Outcome | Gate |
| --- | --- | --- |
| 1 | Review, bounded plan, measurable learning/visual bar | Baseline docs-check, whitespace, scope review |
| 2 | Learning section, simple overview visual, guided concepts, home navigation | Reader questions, source review, visual inspection, docs-check |
| 3 | Simpler design diagrams, tradeoff explanations, corrected status/reference/guide details | Executable examples, focused tests, all review lenses, docs-check, scope review |

Repeat a repair round if a gate fails. Commit only reviewed documentation
changes. Record actual results and remaining limitations in the
[current review](REVIEW.md), without backdating the earlier
review or treating the documentation pass as deployment qualification.

### Explanation ownership

- `documentation/learn/`: why the runtime exists, a worked story, time and state, reasoning,
  authority, and design tradeoffs.
- `documentation/overview/concepts.md`: concise vocabulary with links to those explanations.
- `design/`: deeper mechanics, bounded diagrams, tradeoffs, and source evidence.
- `getting-started/` and `guides/`: runnable tasks and expected results.
- `reference/` and `contracts/`: exact names and wire/storage boundaries.

## Execution result

All three rounds are complete. The new learning path and design summaries,
source-alignment repairs, runnable proofs, browser visual review, and five
self-review lenses meet [this pass's quality bar](QUALITY_BAR.md).
See [the review](REVIEW.md) for the actual evidence and completion boundary.
