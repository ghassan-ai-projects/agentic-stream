# Plan: Hermes Agent Architecture — Full Study Report

## Goal
Produce a senior-architect-grade study of the **Hermes agent** (GitHub repo) architecture and design patterns, with diagrams/charts, suitable as the next chapter of ghassan-alhamoud.com's AI agents patterns handbook. Deliverable: .docx (+ .md).

## Stage 0 — Identify the repo
- Clarify/verify which "Hermes agent" GitHub repo (likely NousResearch/hermes-agent or similar). Confirm via web search.

## Stage 1 — Research (deep-research-swarm)
- Load skill: `/app/.agents/skills/deep-research-swarm/SKILL.md`
- Deploy parallel research agents:
  1. Repo structure & core architecture (event loop, orchestration, model layer)
  2. Agentic patterns used (ReAct, tool use, planning, memory, context management, routing, multi-agent, etc.)
  3. Code-level deep dive: key modules, classes, flows (clone/read GitHub code)
  4. Comparison to canonical agent patterns taxonomy (for handbook context)
- Cross-validate findings. Output: validated research brief with file/line references.

## Stage 2 — Diagrams
- Generate architecture diagrams (Mermaid where suitable, plus matplotlib/graphviz charts) for:
  - High-level architecture
  - Agent loop / control flow
  - Tool execution pipeline
  - Memory/context subsystem
  - Pattern map

## Stage 3 — Writing (report-writing)
- Load skill: `/app/.agents/skills/report-writing/SKILL.md`
- Write senior-architect study: overview, architecture, patterns catalog (each pattern: intent, structure, Hermes implementation, trade-offs), extension guide for cloning, diagrams embedded.
- Output: `/mnt/agents/output/hermes-architecture-study.md`

## Stage 4 — Formatting (docx)
- Load skill: `/app/.agents/skills/docx/SKILL.md`
- Convert final markdown to .docx with diagrams embedded.
- Output: `/mnt/agents/output/hermes-architecture-study.docx`
