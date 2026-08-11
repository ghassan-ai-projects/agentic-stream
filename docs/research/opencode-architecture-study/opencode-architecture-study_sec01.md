## Chapter 1 — Executive Summary

OpenCode (`anomalyco/opencode`) is the open-source, MIT-licensed AI coding agent: provider-agnostic by design, it runs as a terminal UI, desktop app, IDE extension, and headless server against 24 bundled provider packages and the models.dev catalog. With roughly 187k GitHub stars as of July 2026 and a full v2 rewrite in flight, it is one of the very few production-grade agent codebases whose internals — loop, tools, permissions, provider layer, storage — can be read end to end. This study analyzes the dev branch at commit a19b52e85bf2 (v1.18.3, analyzed 2026-07-20), and every claim cites file and line so the reader can verify it against the source. OpenCode rewards this scrutiny because it has already solved, in public, the problems every agent builder hits in private — and because its mistakes and migrations are as instructive as its successes.

Five findings headline the study:

| # | Headline finding | Treated in |
|---|---|---|
| 1 | Own the loop: a hand-rolled `while (true)` ReAct loop beats SDK steppers for control | Chapter 3 |
| 2 | Everything is a permission-gated tool behind a fail-closed ordered-rule engine | Chapters 4, 6 |
| 3 | Client–server plus an event-sourced core makes every channel replaceable | Chapters 2, 7 |
| 4 | Pattern density: ~35 canonical design patterns in one codebase | Chapter 9 |
| 5 | The tree is mid-migration: the v1 production loop runs atop a staged Effect-TS v2 core | Chapters 2, 3, 10 |

Each finding is load-bearing. Owning the loop (`session/prompt.ts:1088`) is what makes exit tests, step accounting, and mid-stream overflow truncation expressible at all — the AI SDK's `maxSteps` cannot see the event-sourced history those mechanisms need. The permission engine reduces safety to an ordered rule array with last-match-wins semantics and a default of `ask`: a gate, an audit trail, and a steering channel in thirteen lines. Because every mutation is a durable event committed with its projections, fork, revert, compaction replay, and live multi-client streaming fall out of a single commit path rather than being features in their own right. The pattern census (Chapter 9) quantifies the study's central observation: a production coding agent is roughly 20% cognition and 80% control engineering. And the migration warning is practical, not academic — readers must distinguish what serves production today (`packages/opencode/src/session/`) from staged successors (`packages/core`, `packages/llm`, `packages/protocol`) before copying anything.

The clone takeaway, in three sentences: clone the behavioral core — the loop, the tool set, the prompt pipeline, the permission gate, and compaction — which Chapter 10 estimates at roughly 2,000 disciplined lines to reproduce what makes OpenCode useful; defer the heavy infrastructure (additional channels, Effect-TS, event sourcing, the config cascade) until you feel its pain, because OpenCode itself shipped without them; and remember that the MIT license makes the source itself the documentation — read it, don't reinvent it.

To read this study: Chapters 2–3 establish the system shape and the agent loop; Chapters 4–6 dissect the working surfaces (tools, models and providers, permissions); Chapters 7–8 cover the server, event, storage, and extensibility layers; Chapter 9 synthesizes the patterns catalog; and Chapter 10 distills everything into a clone blueprint. Interpretation is always marked as such; everything else is code evidence.
