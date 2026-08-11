# OpenCode Architecture Study — Outline
Report file base: opencode-architecture-study | Language: English | Style: technical
Subject: anomalyco/opencode (formerly sst/opencode), dev branch @ a19b52e85bf2, v1.18.3, 2026-07-20
Audience: readers of ghassan-alhamoud.com AI Agents Patterns handbook; senior engineers intending to clone the agent core.
Research inputs: /mnt/agents/output/opencode-study/findings/{A1,A2,A3,A4,A5,A6,R1}-*.md
Diagrams: /mnt/agents/output/opencode-study/diagrams/{d1..d6}.png
Chapter files: /mnt/agents/output/opencode-study/opencode-architecture-study_sec{NN}.md

## Ch 1 — Executive Summary (sec01, ~600 words, ROUND 3)
Positioning, five headline architectural findings, the pattern-density argument, clone takeaway.

## Ch 2 — System Architecture & Design Philosophy (sec02, ~1400 words, ROUND 1)
2.1 What OpenCode is (positioning vs Claude Code/Cursor; provider-agnostic)
2.2 Client–server topology: one process, many channels (d1-container-map.png)
2.3 Monorepo layering: packages/opencode (v1) vs core/llm/protocol/server (v2, Effect-TS)
2.4 Internal component architecture (d2-component-architecture.png)
2.5 Design philosophy: Effect-TS DI, event-driven, "HTTP API is the extension boundary"
2.6 Version context: v1.18.3, experimentalNativeLlm, v2 CONTEXT.md direction
Inputs: R1, A5, A1

## Ch 3 — The Agent Loop (sec03, ~1900 words, ROUND 1)
3.1 Session & message model (MessageV2 parts, ULID ids, sessions tree)
3.2 The hand-rolled while(true) loop vs AI SDK maxSteps (prompt.ts:1088)
3.3 Streaming pipeline: LLMEvent → parts → durable events (d3-agent-loop-sequence.png)
3.4 Abort/interrupt: Runner FSM
3.5 Retries: Effect Schedule, Retry-After, backoff, error classification
3.6 Context management: overflow triggers, compaction (25% tail, anchored summary), tool-output pruning
3.7 Subagents: task tool, child sessions, background jobs
Inputs: A1 (+A2 for task tool details)

## Ch 4 — The Tool System (sec04, ~1900 words, ROUND 1)
4.1 Tool contract: Tool.define, Effect Schema, ctx (sessionID/abort/metadata/ask)
4.2 Registry: built-ins, custom tools, plugin tools, per-agent gating (hidden vs ask)
4.3 Built-in tool inventory (table: tool, purpose, permission, truncation)
4.4 Edit/write safety: 9-strategy replacer cascade, file semaphore, CRLF/BOM, diff-before-ask
4.5 Shell tool: tree-sitter parsing, spawner, timeouts, output spill
4.6 Interaction tools: question, plan enter/exit, task
4.7 Tool description prompt-engineering (.txt files) — patterns
4.8 Output truncation service
Inputs: A2 (+A4 for permission interplay)

## Ch 5 — Model & Provider Abstraction (sec05, ~1600 words, ROUND 1)
5.1 Two stacks: AI SDK v6 production vs @opencode-ai/llm (experimentalNativeLlm)
5.2 Provider registry: ~26 bundled SDK packages, custom loaders, quirks
5.3 Model catalog: models.dev, capabilities/cost/limits, variants
5.4 Message transforms: per-provider normalization
5.5 Prompt caching policy (breakpoints, promptCacheKey)
5.6 Auth: auth.json, OAuth plugin hooks
5.7 Agent definitions: build/plan/general/explore + hidden agents; per-agent model/prompt/tool overrides
5.8 System prompt assembly pipeline (family .txt + <env> + AGENTS.md + MCP/skills)
Inputs: A3 (+A1 for prompt pipeline)

## Ch 6 — Permissions & Security (sec06, ~1400 words, ROUND 1)
6.1 Rule engine: ordered allow/ask/deny, findLast, wildcard, default ask
6.2 Arity dictionary & bash pattern synthesis
6.3 Approval flow: Deferred rendezvous, permission.asked/reply, always-reply
6.4 Bash granular checks via tree-sitter; honest limits (no sandbox, env secrets)
6.5 External-directory boundary
6.6 Per-agent profiles (plan read-only, explore), subagent inheritance
6.7 doom_loop circuit breaker; headless behavior (d4-permission-flow.png)
Inputs: A4 (+A2)

## Ch 7 — Server, Events & Storage (sec07, ~1500 words, ROUND 1)
7.1 Effect HttpApi server; route surface; OpenAPI /doc
7.2 SSE-only streaming; PTY websocket exception; port/mdns/auth
7.3 Event architecture: EventV2 (event sourcing) + GlobalBus + bridge (d5-event-sourcing.png)
7.4 CQRS projectors; dual storage (legacy JSON + SQLite/drizzle)
7.5 SDK generation (hey-api) and sdk-next; ACP adapter
7.6 Multi-project instances, lifecycle
Inputs: A5

## Ch 8 — Extensibility & Infrastructure (sec08, ~1700 words, ROUND 1)
8.1 Plugin system: hooks surface, loader, limits (dead permission.ask hook), v2 API
8.2 MCP integration: transports, OAuth, tool namespacing
8.3 LSP integration: lazy spawn, diagnostics into tool output
8.4 Config cascade (8 layers), {env:}/{file:}, Effect-Schema
8.5 Skills & commands: markdown declarative extensions
8.6 Snapshot/revert: shadow-git memento
8.7 Formatters feedback loop
Inputs: A6

## Ch 9 — Patterns Catalog (sec09, ~1600 words, ROUND 2)
Master table: pattern name (canonical) → where in code → why it matters → handbook mapping.
Groups: agent-loop patterns (ReAct, doom-loop guard, compaction), structural (registry, adapter,
strategy, decorator, DI/layers), concurrency (FSM, Deferred rendezvous, keyed mutex, fiber abort),
data (event sourcing, CQRS, memento/shadow-git), reliability (retry schedule, circuit breaker,
fail-closed), prompting (description engineering, cache breakpoints, prompt templates).
Cross-reference chapters 3–8 (pass completed chapter files).
Inputs: ALL findings + completed chapters 2–8

## Ch 10 — Clone Blueprint (sec10, ~2000 words, ROUND 2)
10.1 Clone boundary: what the agent is (d6-clone-blueprint.png)
10.2 Minimal viable architecture (module-by-module, with effort/complexity table)
10.3 The 20% that delivers 80% (loop, tools, prompt, permission)
10.4 What to simplify safely (skip Effect-TS, event sourcing, 8-layer config)
10.5 What NOT to cut (truncation, edit replacer cascade, permission engine, compaction)
10.6 Suggested tech stack for a clone (TS/Python variants), phased roadmap
10.7 Pitfalls observed in OpenCode (docs drift, dead hooks, dual-stack complexity)
Inputs: A1–A6 + completed chapters

## Appendix (assembled by orchestrator)
Methodology (commit, analysis process), references (repo, docs URLs), mermaid sources for website reuse.
