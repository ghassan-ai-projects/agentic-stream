# Plan — OpenCode Architecture Study (Senior Architect Report)

## Goal
Produce a comprehensive architecture study of OpenCode (sst/opencode coding agent) for
ghassan-alhamoud.com's AI Agents Patterns handbook follow-up. Scope: the AGENT itself
(core loop, tools, sessions, providers, permissions, extensibility) — not distribution
channels (TUI/desktop/web are analyzed only as clients of the agent core). Output must
support the reader's intent to build a clone of the agent.

## Stage 1 — Codebase Acquisition & Orientation (Orchestrator)
- Shallow-clone sst/opencode from GitHub (current main branch, as of 2026-07).
- Map monorepo structure, package boundaries, tech stack, entry points.
- Output: repo map + decomposition of analysis areas for swarm agents.

## Stage 2 — Parallel Deep Code Analysis (explore subagents, background, parallel)
Each agent gets the local clone path + a scoped mission; read-only.
- A1: Core agent loop — session lifecycle, message processing, step/tool-call loop,
  streaming, compaction, retry/error handling.
- A2: Tool system — built-in tools, tool definition/registration pattern, execution,
  schema validation (zod/AI SDK), output truncation, tool-context passing.
- A3: Model/provider abstraction — provider registry, model resolution, AI SDK usage,
  prompt construction (system prompts, agents/modes), token/context management.
- A4: Permission & security model — permission engine, approval flows, sandboxing,
  bash safety, external directory rules.
- A5: Server/API + client boundary — HTTP/SSE server, event bus, TUI/SDK as clients;
  identify exactly the "agent-only" subset the user wants to clone.
- A6: Extensibility & infra patterns — plugin system, MCP integration, LSP integration,
  config loading, storage, file watcher, VCS integration.
Also:
- R1 (web): Official docs (opencode.ai) — agents, modes, permissions, plugins, MCP,
  config schema, design philosophy; cross-validate code findings.

## Stage 3 — Validation & Gap Fill (Orchestrator + verifier if needed)
- Cross-check subagent findings against the actual code; resolve contradictions.
- Fill gaps (e.g., patterns catalog cross-reference).

## Stage 4 — Diagrams
- Architecture diagrams (C4-style context/container/component), agent-loop sequence
  diagram, tool-execution flow, permission flow, event flow. Mermaid sources + rendered
  PNG/SVG images for the report.

## Stage 5 — Report Writing (report-writing skill)
- Long-form Markdown report: executive summary, architecture overview, each subsystem,
  patterns catalog (mapped to the handbook's agent patterns), clone blueprint
  (minimal reimplementation guide), references.
- Output: /mnt/agents/output/opencode-study/opencode-architecture-study.md

## Stage 6 — Delivery (docx skill)
- Convert final .md to .docx with embedded diagrams.
- Deliver .md + .docx in /mnt/agents/output/opencode-study/
