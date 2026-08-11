# Hermes Agent: An Architectural Study and Agentic Patterns Catalog

## Executive Summary (~400 words)
### Key Findings
#### Thesis: hermes-agent is a single synchronous ReAct-style loop whose complexity lives in self-authored data (skills, memory, cron, toolsets) rather than orchestration graphs
#### Five load-bearing verified facts: run_conversation() is the real loop (agent/conversation_loop.py:669+); 74 registered tools by literal call-site count; frozen-snapshot memory + api_content sidecar exist for prompt-cache parity; the learning loop writes only whitelisted bounded Markdown; the RL layer was amputated in one PR (#26106) without destabilizing the runtime
#### Five stale-source corrections (C1–C5) resolved to code at HEAD, snapshot July 2026
#### Payoff: a 15-entry patterns catalog with 4 PIONEER entries, and a staged 10-step clone roadmap at ~10–15% of the codebase

## How to Read This Study (~200 words)
### Citation and Evidence Conventions
#### All file:line references and permalinks pin the July-2026 snapshot at HEAD commit 4c9628e (/blob/4c9628e/<path>#L<x>); the RL-removal claim cites GitHub API evidence (commit 5af672c753, PR #26106, 2026-05-15)
#### Three evidence grades: verified-in-code, documented-but-unverified, [INFERRED] (preserved verbatim); single-source counts carry counting-method attribution

## 1. Positioning and the Loop-Centric Thesis (~1,400 words, 1 figure, 1 table)
### 1.1 The Subject and the Evidence Base
#### 1.1.1 hermes-agent by Nous Research: Python, MIT, snapshot @ 4c9628e (July 2026); README/docs treated as claims to verify, not facts
#### 1.1.2 Currency caveats: fast-moving repo; docs lag code in three verified places (C1–C3); most volatile sections named
### 1.2 The Central Architectural Bet
#### 1.2.1 One loop, not a graph: no planner–executor split, no DAG workflows, no tree search, no debate anywhere in the tree — deliberate absence as design
#### 1.2.2 Behavior encoded as data the loop reads (skills, memory, cron jobs, toolsets) vs behavior encoded as orchestration topology — forces and trade-offs
#### 1.2.3 One platform-agnostic AIAgent serves CLI, gateway, batch, ACP — why this matters for the clone scope (channels named only to be excluded)
### 1.3 Component Decomposition Map (Figure 01)
#### 1.3.1 agent/ (loop, prompt, learning, curator, provider glue), tools/ (registry, 35 tool modules, environments), cron/, hermes_state.py, hermes_cli/ — responsibility per top-level area
#### 1.3.2 God-file decomposition: conversation_loop.py:1–15 and run_agent.py:498–500 forwarder — with the C1 stale-docs callout naming the refactor
### 1.4 Architectural Positioning vs Peer Agents
#### 1.4.1 Comparison table: hermes-agent vs OpenClaw vs Claude Code vs AutoGPT — rows limited to loop topology, tool model, memory model, self-improvement, proactivity (architecture-level, web-cited)
#### 1.4.2 CoALA memory vocabulary preview (semantic/episodic/procedural/working) as the report's shared language
#### 1.4.3 Reader's map: Profile A (pattern evaluator) vs Profile B (clone builder) reading paths

## 2. Cross-Cutting Insights (~700 words)
### 2.1 The Seven Insights
#### 2.1.1 Prompt-cache economics is the hidden architect — five subsystems contain byte-stability mechanisms; load-bearing, not optimization (hosts: ch5, 6, 7, 8, 9)
#### 2.1.2 Loop, not graph — the thesis restated (stated in ch1; every later chapter is evidence)
#### 2.1.3 Self-improvement is disciplined text gardening — safety by constriction of the mutation surface; transferable guardrail stack (host: ch7)
#### 2.1.4 Capability subtraction beats instruction — hard guards at the toolset layer, soft at the prompt layer; Hermes chooses hard (hosts: ch4, 8, 9)
#### 2.1.5 Concentric extensibility rings — capability, cost, trust decrease outward; self-modification confined to the outermost ring (hosts: ch4, 6, 9)
#### 2.1.6 Research-readiness as seams, proven by amputation (host: ch9)
#### 2.1.7 Minimal viable clone ≈ 10–15% of the tree, convergent across dimensions (host: ch10 Part B)

## 3. The Core Runtime: The Conversation Loop (~1,700 words, 1 figure, 1 table, 3 excerpts)
### 3.1 Where the Loop Actually Lives (C1 callout)
#### 3.1.1 run_conversation() in agent/conversation_loop.py:669–6166 (~3,900 lines) is the real loop; AIAgent is a facade post god-file decomposition; CONTRIBUTING.md is stale (grep-verified, no _run_agent_loop symbol)
### 3.2 Turn Lifecycle (Figure 02)
#### 3.2.1 Prologue seam build_turn_context → outer budget loop → inner retry loop → epilogue seam finalize_turn; the two seams as primary extension points
#### 3.2.2 The implicit state machine: _turn_exit_reason strings + TurnRetryState restart flags (table of exit reasons)
### 3.3 Budget, Concurrency, Interrupt
#### 3.3.1 IterationBudget: thread-safe consume/refund, 90 parent / 50 subagent (iteration_budget.py:17–59); tool-less summary call at exhaustion (turn_finalizer.py:127–142); refunds for execute_code/compression/redirect restarts — a subtle correctness surface
#### 3.3.2 Synchronous loop; parallel tool batches on DaemonThreadPoolExecutor ≤8 workers, 420 s batch deadline (tool_executor.py:95–98); daemon rationale
#### 3.3.3 Cooperative 3-layer interrupt (agent flag + per-thread bits + socket abort); streaming single-writer fence
### 3.4 Errors and Honest Assessment
#### 3.4.1 Loop-level error classification feeding the inner retry loop; seam to the provider FailoverReasons taxonomy (cross-ref ch9)
#### 3.4.2 The 3,900-line loop as design risk: god-file pressure moved, not eliminated; load-bearing vs incidental regions
#### 3.4.3 Clone notes: loop + provider path is milestone one

## 4. The Tool System (~1,400 words, 1 figure, 1 table, 3 excerpts)
### 4.1 Registration and Discovery
#### 4.1.1 Self-registration at import time + AST-gated auto-discovery: only modules with top-level registry.register() are imported (tools/registry.py:43–84) — forces: zero-config discoverability vs import side effects
#### 4.1.2 C4 callout: 74 built-in tools, 57 static toolsets, 25 hermes-* bundles by literal call-site count; README "40+" is conservative marketing
### 4.2 Schemas, Toolsets, Gating
#### 4.2.1 OpenAI-format schemas wrapped at emission; check_fn availability gating at schema-emission time — a tool absent from the schema is physically uncallable
#### 4.2.2 Toolset composition and platform bundles; MCP tools merge as mcp-* toolsets with collision-skipping; Tool Search progressive disclosure over the MCP/plugin surface
### 4.3 Execution Pipeline
#### 4.3.1 Segment planner (parallel-safe vs barrier) → executor → guardrails → approval CoR integration (full gate in ch9); where execution meets the ch3 thread pool
#### 4.3.2 Extension story: "2 files to add a tool" end-to-end; toolset-taxonomy table
#### 4.3.3 Extensibility rings (Figure 04): registry → toolsets → plugins → MCP → skills — capability, cost, trust decrease outward (insight 5 primary home)
#### 4.3.4 Clone notes: registry + minimal toolset is milestone three; ~50-line check_fn advice

## 5. Context Engineering and Prompt-Cache Economics (~1,700 words, 1 table, 3 excerpts)
### 5.1 The Byte-Stability Doctrine (insight 1 argued in full)
#### 5.1.1 Three-tier prompt assembly stable→context→volatile, built once per session; date-only timestamps because minute precision invalidates prefix-cache KV (system_prompt.py:503–511)
#### 5.1.2 Byte-identical replay: api_content sidecar in SQLite replays exact bytes sent when the turn was live (conversation_loop.py:1022–1036; turn_context.py:50–66)
#### 5.1.3 Anthropic system_and_3: up to 4 cache_control breakpoints — system prompt + last 3 eligible messages (prompt_caching.py:84–119)
### 5.2 Compression and Budgeting
#### 5.2.1 Dual triggers (gateway 85% hygiene; in-loop 50%); 4-phase compress() with iterative summary rehydration; ContextEngine ABC plugin contract
#### 5.2.2 Three-layer tool-result budget: per-tool cap → sandbox spill with ≤1,500-char preview → per-turn 200K aggregate scaled to 15%/30% of model window, 8K/16K floors (tool_result_storage.py:1–25; budget_config.py:84–114); read_file pinned to infinite to break persist→read→persist loops
### 5.3 The Cost Model
#### 5.3.1 Cache-stability mechanism map table: api_content sidecar, date-only timestamps, frozen memory snapshot, cache-parity fork, idle-only completion delivery → host subsystem per row
#### 5.3.2 Inverse payoff: a provider without prefix caching lets a clone delete ~15–20% of system complexity (sidecars, replay machinery, cache-parity forks)

## 6. Memory and Skills: The Self-Authored Data Layer (~1,800 words, 2 tables, 4 excerpts)
### 6.1 Agent-Curated File Memory
#### 6.1.1 C5 callout: files at $HERMES_HOME/memories/MEMORY.md + USER.md (memory_tool.py:55–57); §-delimited entries, char budgets (2,200/1,375), substring targeting, drift guards
#### 6.1.2 Frozen-snapshot invariant: injected once per session, never mid-session; live writes hit disk immediately (memory_tool.py:123–132, 625–633); rationale cross-ref ch5
### 6.2 Session Store and Retrieval
#### 6.2.1 One SQLite state.db per profile, WAL, three FTS5 indexes (unicode61, trigram excl. role='tool', CJK bigram), schema v23 marker-gated reindexing (hermes_state.py:994–1392)
#### 6.2.2 C3 callout: session_search makes zero LLM calls — summary mode removed (session_search_tool.py:1–29 docstring); README claim stale
#### 6.2.3 MemoryProvider ABC: exactly-one-external-provider exclusivity, 8 bundled providers; CoALA mapping table (semantic/episodic/procedural/working)
### 6.3 Skills as Procedural Memory
#### 6.3.1 SKILL.md packages: agentskills.io-compatible front matter + metadata.hermes.* extensions — an interop bet
#### 6.3.2 Three-tier progressive disclosure (index→body→support files); mandatory-load preamble in the stable prompt tier; token economics
#### 6.3.3 skill_manage CRUD with provenance ContextVar, ownership guards, 100 KB caps, read-before-write; write-trigger cadences (forward to ch7)
#### 6.3.4 Skills Hub: 9 SkillSource adapters, quarantine → regex scan → trust tiers; skills as the outermost ring and the only layer the agent writes
#### 6.3.5 Clone notes: memory is milestone six, skills milestone seven; both individually omittable

## 7. The Learning Loop: Self-Improvement as Disciplined Text Gardening (~2,300 words, 1 figure, 1 table, 3 excerpts)
### 7.1 The Honest Frame (insight 3)
#### 7.1.1 Self-organizing, not self-modifying: the mutation surface is whitelisted, bounded, human-readable Markdown — never code, weights, or runtime prompts; safety by constriction
### 7.2 The Background Reflection Fork (Figure 03)
#### 7.2.1 Cadences: memory nudge every 10 user turns (turn_context.py:554–561), skill review every 10 tool iterations (turn_finalizer.py:576–581); fires after response delivery, uninterrupted turns only
#### 7.2.2 The fork: full second AIAgent on daemon thread, memory/skills-only whitelist, _persist_disabled, compression off, auto-deny approvals (background_review.py:617–953)
#### 7.2.3 Cache-parity pins: byte-identical system prompt/tools/session_id rides the parent's warm prefix cache — cited ~26% end-to-end cost saving (background_review.py:765–789)
#### 7.2.4 /learn foreground distillation; the learning graph; artifacts re-enter the main loop as prompt data, closing the loop
### 7.3 Verification and the Curator
#### 7.3.1 Verification ledger + verify-on-stop — the only in-loop verifier; quality is prompt-mediated, degrades gracefully with model quality
#### 7.3.2 Curator: 168 h interval, 2 h idle gate, active→stale(30d)→archive(90d), archive-never-delete, tar.gz snapshot + undoable rollback, opt-in LLM consolidation (curator.py:233–283, 305–383)
### 7.4 Fitness Measurement Lives Elsewhere
#### 7.4.1 DSPy+GEPA companion repo (hermes-agent-self-evolution): the only fitness-measured optimization, PR-gated, human-reviewed; main repo = experience→markdown, external repo = markdown→measurably better markdown (web-sourced)
#### 7.4.2 Guardrail stack table (guard, enforcement point file:line, failure mode prevented) + residual-risk assessment with severities
#### 7.4.3 Clone notes: learning fork is stage 8 and first omission candidate; guardrail stack non-negotiable if kept

## 8. Delegation and Proactivity: Subagents, execute_code, and Cron (~1,800 words, 1 table, 3 excerpts)
### 8.1 Subagent Delegation
#### 8.1.1 In-process thread-based subagents: fresh AIAgent, no shared history, summaries-only fan-in; top-level delegation forced background at the dispatch intercept (run_agent.py:6493–6523)
#### 8.1.2 Hard guards: DELEGATE_BLOCKED_TOOLS strips delegate_task/clarify/memory/send_message/cronjob in code (delegate_tool.py:46–54); depth-1 default, unbounded if raised — risk callout with severity
#### 8.1.3 Durable claim/ack completion; idle-only re-entry as new turns preserving role alternation and cache integrity (async_delegation.py:15–22); shared DaemonThreadPoolExecutor across four subsystems
### 8.2 Programmatic Tool Calling
#### 8.2.1 execute_code: scrubbed subprocess, token-authenticated AF_UNIX RPC into the real dispatcher, 7-tool allowlist, ≤50 calls, ≤50 KB stdout, whole-script pre-approval (code_execution_tool.py:62–70, 1223–1240)
### 8.3 Cron: Proactivity with Hard Isolation
#### 8.3.1 Fresh isolated AIAgent session per fire; cronjob/messaging/clarify toolsets stripped in code (scheduler.py:156–176) — second canonical capability-subtraction instance
#### 8.3.2 The LLM as schedule parser (four structured shapes) — an unusual design bet, interrogated
#### 8.3.3 At-most-once via advance-before-execute; the executions ledger "is not a retry queue" (executions.py:1–6, verbatim); consent-first suggestions and blueprints
#### 8.3.4 Three-surface comparison table (subagent vs execute_code vs cron): isolation, restriction mechanism, delivery semantics, guard type
#### 8.3.5 Clone notes: delegation stage 9, cron stage 10; both omittable in minimal core

## 9. The Platform Layer: Environments, Security, Providers, and Research Seams (~1,800 words, 3 tables, 3 excerpts)
### 9.1 The Trust Model, Quoted and Taken Seriously
#### 9.1.1 SECURITY.md:53–59 verbatim: "The only security boundary against an adversarial LLM is the operating system" — no in-process sandbox pretense; containment delegated to the environment layer
#### 9.1.2 Approval gate: chain-of-responsibility with hardline floor below yolo, fail-closed defaults (timeout denies, absent human denies); HERMES_YOLO_MODE and redaction frozen at import to defeat prompt-injection escalation (approval.py:32–35; redact.py:61–69) — third capability-subtraction instance
### 9.2 Six Backends, Two Methods
#### 9.2.1 BaseEnvironment subclasses implement only _run_bash() + cleanup() (environments/base.py:390–396); file tools as facade over the shell contract (file_operations.py:793–799); backend comparison table (local, Docker, SSH, Singularity, Modal, Daytona) with containment characteristics
### 9.3 The Five-Layer Provider Architecture
#### 9.3.1 Auth registry → declarative ProviderProfile plugins (33 bundled, fail-closed trust flags) → runtime resolver → api_mode transports → native adapters; api_mode as central discriminator; OpenAI-shaped internal representation as invariant
#### 9.3.2 Resolution priority chain (hermes_cli/auth.py:1847+): OAuth demoted to last resort; auxiliary client lane; ~25 FailoverReasons driving retry/fallback/credential-pool (seam cross-ref ch3)
### 9.4 Research Seams and the Amputation (C2 callout, insight 6)
#### 9.4.1 RL environments absent at HEAD; removal verified via GitHub API: commit 5af672c753, 2026-05-15, PR #26106; external articles and the live docs page are stale — one historical paragraph only
#### 9.4.2 Surviving seams table: register_task_env_overrides (terminal_tool.py:1127–1142), per-task_id sandbox isolation, _run_async bridge (model_tools.py:98–122); durable assets: ShareGPT trajectories, 3-layer budgeting, sandbox-per-task
#### 9.4.3 Architectural lesson: design training/eval coupling as injectable seams; motive/impact marked [INFERRED]
#### 9.4.4 Clone notes: provider path is stage 1, environment+approval stage 4; seam-design lesson carried into ch10 Part B

## 10. Patterns Catalog, Coverage Map, and Clone Roadmap (~2,700 words, 2 figures, 2 tables)
### 10.1 Catalog Admission Rules and Taxonomy Frame
#### 10.1.1 A pattern enters only if instantiated in code at the snapshot; claimed-but-absent patterns go to the ABSENT section; taxonomy frames: Anthropic workflow patterns, Ng's four, CoALA memory (web-footnoted)
### 10.2 The Fifteen Entries (canonical template: Name, Status, Intent, Problem/Context, Solution, Structure, Code evidence file:line, Trade-offs, Related patterns)
#### 10.2.1 CANONICAL entries 1–6: Loop-as-State-Machine; Prologue/Epilogue Turn Seams; Iteration Budget with Consume/Refund; Self-Registering Tool Registry with AST Discovery; Capability Gating at Schema Emission (check_fn); Progressive Disclosure (Tool Search + skills index)
#### 10.2.2 CANONICAL entries 7–11: Three-Tier Prompt Assembly with Prefix-Cache Engineering; Byte-Identical History Replay (api_content sidecar); Dual-Trigger Compression with Summary Rehydration; Three-Layer Tool-Result Budgeting (spill-to-store); Frozen-Snapshot Context Injection
#### 10.2.3 CANONICAL entries 12–14 + PIONEER entries: Zero-LLM Structured Retrieval over Session History; Background Reflection Fork with Cache Parity (novel mechanic inside a canonical reflection entry); Curator Archive-Never-Delete Lifecycle; PIONEER #1 Agent-Curated File Memory; PIONEER #2 Self-Authored SKILL.md Procedural Memory; PIONEER #3 NL Cron with Isolated Sessions; PIONEER #4 Capability-Subtraction Guardrails (standalone entry — strongest transfer case)
### 10.3 Coverage Map and Deliberate Absences (Figure 05)
#### 10.3.1 Coverage-map walk-through: every entry points at its module(s), every core module points back at its pattern(s)
#### 10.3.2 Deliberately absent: planner–executor split, DAG orchestration, tree search, debate, in-loop fitness-measured self-modification, vector memory, exactly-once cron — each with one line on why it matters for the clone decision
### 10.4 Part B — Clone Roadmap (Figure 06)
#### 10.4.1 Scope and seam: agent core only; what the core expects (turn input, environment, approval decisions, delivery) and exposes (events, artifacts, state.db); channels named only to be excluded
#### 10.4.2 Minimal viable core: provider path + loop + registry with a handful of tools + one environment + approval — the subset that runs one turn
#### 10.4.3 Staged 10-step build order with LOC estimates (flagged as convergent estimates) and per-step risk: provider (~350) → loop (~600) → registry+tools (~300) → environment+approval (~500) → context (~300) → memory (~200) → skills (~250) → learning fork (~300) → delegation (~270) → cron (~300)
#### 10.4.4 Defer-list with justifications (MCP, provider plugins, Skills Hub, execute_code RPC, 3-index FTS5, MoA, gateway, datagen, GEPA) and the copy-early guards (check_fn, archive-never-delete, provenance, fail-closed approval, frozen-at-import toggles)
#### 10.4.5 Decision register: interfaces to preserve verbatim (tool schema shape, SKILL.md format, api_content sidecar if caching matters) vs replace (state storage, delivery routing); re-verify list given drift
#### 10.4.6 Phasing table: phase, milestone, copy/stub/re-implement, effort, risk

# References
## hermes_dim01–12.md
- **Type**: Research corpus (12 dimension reports, code-grounded)
- **Description**: Deep-dive evidence base with file:line citations for every chapter
- **Path**: /mnt/agents/output/research/

## hermes_insight.md / hermes_cross_verification.md
- **Type**: Synthesis artifacts
- **Description**: 7 cross-dimension insights; confidence tiers + resolved conflict zones C1–C5
- **Path**: /mnt/agents/output/research/

## outline_requirement_analyst.md / outline_artifact_analyst.md / outline_structure_designer.md / outline_content_planner.md
- **Type**: Outline design artifacts
- **Description**: Requirements, research synthesis, chapter skeleton, per-chapter content specs
- **Path**: /mnt/agents/output/research/

## diagrams 01–06
- **Type**: Figures (PNG)
- **Description**: High-level architecture; turn control flow; learning loop; extensibility rings; patterns coverage map; clone roadmap
- **Path**: /mnt/agents/output/diagrams/
