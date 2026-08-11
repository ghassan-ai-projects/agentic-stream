# 10. Patterns Catalog, Coverage Map, and Clone Roadmap

The preceding chapters dissected mechanisms; this closing chapter abstracts them. Part A restates the mechanisms of Chapters 3–9 as handbook-liftable pattern entries, maps them onto the modules that instantiate them, and names the canonical patterns the system deliberately declines. Part B assembles the per-chapter "Clone notes" callouts into a staged build roadmap for the agent core. All code citations are pinned to this study's snapshot (HEAD `4c9628e`, July 2026).

## 10.1 Catalog Admission Rules and Taxonomy Frame

### 10.1.1 Admission rule and reference frames

A pattern enters the catalog only if instantiated in code at the snapshot, however small the instance; patterns claimed in documentation or expected from the literature but absent from the code go to the ABSENT section (§10.3.2), never into entries. Entries are graded CANONICAL — a known pattern from the reference frames, instantiated here — or PIONEER, where no clean precedent exists and novelty is argued from the taxonomy evidence, not asserted.

Four frames supply the vocabulary: Anthropic's workflow/agent split and five workflow patterns [^1^][^2^]; Ng's four cognitive patterns — Reflection, Tool Use, Planning, Multi-Agent [^3^]; the CoALA memory taxonomy with context engineering as curation of the working tier [^4^][^5^][^6^]; and the long-running-agent literature — ReAct,[^7^][^8^] approval tiers,[^9^][^10^] heartbeat/cron proactivity,[^11^][^12^] and the SKILL.md standard.[^13^][^14^] Entries cite these under Related patterns; Chapters 3–9 remain the evidence base.

## 10.2 The Entries

Template, fixed order: Name (with literature aliases); Status; Intent; Problem/Context; Solution; Structure; Code evidence; Trade-offs; Related patterns. Each entry back-references its host chapter, which carries the full mechanism and figures.

### 10.2.1 Loop and tool-system entries (1–6)

**1. Loop-as-State-Machine** (ReAct loop)
- **Status:** CANONICAL.
- **Intent:** one while-loop per turn; control state in data, not graph topology.
- **Problem/Context:** graphs scatter control flow; unbounded loops convert churn into spend.[^7^][^23^]
- **Solution:** `run_conversation` iterates build → call → dispatch → append until a tool-call-free response; state is `_turn_exit_reason` plus `TurnRetryState`'s one-shot restart flags.
- **Structure:** Figure 2 (Chapter 3).
- **Code evidence:** agent/conversation_loop.py:669–6166 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/conversation_loop.py#L669)); agent/turn_retry_state.py:32–79.
- **Trade-offs:** ~3,900 lines; a recovery branch missing its one-shot guard loops forever.
- **Related patterns:** entries 2, 3; ReAct [^7^][^8^].

**2. Prologue/Epilogue Turn Seams**
- **Status:** CANONICAL.
- **Intent:** per-turn setup/teardown in two replaceable seams outside the loop.
- **Problem/Context:** policy interleaved with control flow forces every change into the hot path.
- **Solution:** `build_turn_context` opens each turn (sanitization, prompt restore-or-build, session row, preflight compression); `finalize_turn` closes every exit (budget-summary fallback, persistence); both extracted behavior-neutrally.
- **Structure:** Figure 2 (Chapter 3).
- **Code evidence:** agent/turn_context.py, invoked at agent/conversation_loop.py:729–748; agent/turn_finalizer.py:69 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_finalizer.py#L69)).
- **Trade-offs:** prologue cost is paid every turn; a seam bug degrades all turns uniformly.
- **Related patterns:** entries 1, 3, 9.

**3. Iteration Budget with Consume/Refund**
- **Status:** CANONICAL.
- **Intent:** bound per-turn spend without charging recovery restarts against progress.
- **Problem/Context:** AutoGPT-class runaway is the canonical autonomy failure [^23^]; flat caps punish legitimate recovery.
- **Solution:** thread-safe `consume()` gates each iteration (default 90; 50 subagents); targeted `refund()` for compression/redirect/rebuild restarts; exhaustion degrades to one tool-less summary call.
- **Structure:** loop-top gate plus finalizer fallback (Chapter 3).
- **Code evidence:** agent/iteration_budget.py:37–45 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/iteration_budget.py#L37)); fallback at agent/turn_finalizer.py:127–142.
- **Trade-offs:** refunds are an unenforced convention — a forgotten one burns the budget recovery exists to protect.
- **Related patterns:** entries 1, 9; circuit breaker.

**4. Self-Registering Tool Registry with AST Discovery** (plugin registry)
- **Status:** CANONICAL.
- **Intent:** capabilities as data — schema, handler, toolset, probe — with zero-config discovery.
- **Problem/Context:** central import lists rot across five entry points.
- **Solution:** modules call `registry.register()` at top level; discovery imports only `tools/*.py` with a module-body `register()` call, behind a text prefilter and `ast.parse`; failures degrade one toolset, never the process.
- **Structure:** acyclic chain registry ← tools ← model_tools (Chapter 4).
- **Code evidence:** tools/registry.py:43–84 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/registry.py#L43)); trigger at model_tools.py:194–217.
- **Trade-offs:** import-time side effects every startup; conditional or aliased registration is silently skipped.
- **Related patterns:** entries 5, 6; extensibility rings (Chapter 4, Figure 4).

**5. Capability Gating at Schema Emission** (check_fn gating)
- **Status:** CANONICAL.
- **Intent:** a tool whose probe fails is absent from the schema — physically uncallable.
- **Problem/Context:** prompt-level "do not use" is ignorable; runtime capability errors teach workarounds.
- **Solution:** `check_fn` evaluated at schema emission behind a 30 s TTL cache with 60 s last-good grace; failures filtered before the OpenAI envelope wrap.
- **Structure:** probe → filter → envelope (Chapter 4).
- **Code evidence:** tools/registry.py:143–206, :558–576 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/registry.py#L558)); probe example tools/file_tools.py:2104–2107.
- **Trade-offs:** availability up to 30 s stale; a wedged probe hides working tools.
- **Related patterns:** entries 4, 18; insight 4.

**6. Progressive Disclosure** (tool search; three-tier skill loading)
- **Status:** CANONICAL.
- **Intent:** routing signal stays cheap; capability detail loads on demand.
- **Problem/Context:** 74 static tools plus MCP/plugin surfaces and ~120 skills cannot all fit the prompt.
- **Solution:** past ~10% of the window, non-core tools collapse behind synthesized `tool_search`/`tool_describe`/`tool_call` bridges; skills disclose index → body → support files, the index riding the stable tier under a mandatory-load preamble.
- **Structure:** tier table (Chapter 6).
- **Code evidence:** model_tools.py:551–579 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/model_tools.py#L551)); skills index at agent/prompt_builder.py:1732–1758.
- **Trade-offs:** a badly described capability silently never loads; no embedding retrieval catches the miss.
- **Related patterns:** entries 4, 16; Agent Skills standard [^13^][^14^].

### 10.2.2 Context and storage entries (7–11)

**7. Three-Tier Prompt Assembly with Prefix-Cache Engineering**
- **Status:** CANONICAL.
- **Intent:** stable bytes first, so provider KV caches hit across turns.
- **Problem/Context:** prefix caches match leading bytes; a full miss costs ~75% input-token delta plus latency (Chapter 5).
- **Solution:** prompt assembled once per session as stable → context → volatile, rebuilt only after compression; date-only timestamps; Anthropic `system_and_3` breakpoints, three rolling with the frontier.
- **Structure:** tiers joined `\n\n`; Chapter 5.
- **Code evidence:** agent/system_prompt.py:10–19, :504–511 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/system_prompt.py#L504)); agent/prompt_caching.py:109–116.
- **Trade-offs:** information loss (wall-clock needs a tool call); model or credential switches zero the cache.
- **Related patterns:** entries 8, 11; context engineering [^6^].

**8. Byte-Identical History Replay** (api_content sidecar)
- **Status:** CANONICAL.
- **Intent:** what turn N sends is exactly what turn N+1 replays.
- **Problem/Context:** ephemeral injections must not persist, yet replaying clean content diverges the prefix at the injected point.
- **Solution:** composed wire bytes stamped as `api_content`, persisted beside clean content by one helper so drift is impossible; later turns substitute the sidecar.
- **Structure:** compose once → persist dual → replay sidecar (Chapter 5).
- **Code evidence:** agent/turn_context.py:50–66 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_context.py#L50)); substitution at agent/conversation_loop.py:1022–1036.
- **Trade-offs:** injected messages stored twice; pure overhead without prefix caching.
- **Related patterns:** entries 7, 11; insight 1.

**9. Dual-Trigger Compression with Summary Rehydration** (compaction)
- **Status:** CANONICAL.
- **Intent:** bound working-memory growth without losing the task thread.
- **Problem/Context:** long sessions overflow; naive truncation severs tool pairs and early goals.[^6^]
- **Solution:** an 85% gateway pass above the in-loop 50% default; prune old results free; keep head plus budgeted tail aligned to tool groups; summarize the middle — re-compression *updates* the prior summary, rehydrated from fossils; swappable engine contract.
- **Structure:** Figure 2 restart path (Chapter 3); Chapter 5.
- **Code evidence:** agent/context_compressor.py:4256–4650; agent/context_engine.py:89–351 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/context_engine.py#L89)).
- **Trade-offs:** lossy; each compression invalidates the cached prefix for a turn or two.
- **Related patterns:** entries 3, 7; Chapter 5.2.

**10. Three-Layer Tool-Result Budgeting** (spill-to-store)
- **Status:** CANONICAL.
- **Intent:** no tool result — and no turn aggregate — may flood the window.
- **Problem/Context:** tools return unbounded output; truncation destroys capability.
- **Solution:** tools pre-truncate; oversized results spill to sandbox temp files behind ≤1,500-char previews recoverable via `read_file` (pinned infinite, so the recovery tool never spills); a per-turn budget force-spills the rest; budgets scale with the model window.
- **Structure:** tool cap → per-result persist → per-turn enforcement (Chapters 5, 9.4).
- **Code evidence:** tools/tool_result_storage.py:3–10 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/tool_result_storage.py#L1)); tools/budget_config.py:84–114, pin :10–13.
- **Trade-offs:** each spill costs a read round-trip; a wrong recovery-tool threshold recreates the spill-read loop.
- **Related patterns:** entries 6, 9; research seam (Chapter 9.4).

**11. Frozen-Snapshot Context Injection** (session-scope freeze)
- **Status:** CANONICAL.
- **Intent:** inject memory once per session; never mutate the live prompt.
- **Problem/Context:** mid-session prompt edits invalidate the prefix cache on every later turn.
- **Solution:** `_system_prompt_snapshot` captured at load, never mutated; live writes persist to disk immediately; the snapshot enters the volatile tier; the next session refreshes.
- **Structure:** snapshot → prompt block → session-boundary refresh (Chapter 6).
- **Code evidence:** tools/memory_tool.py:123–132, :626–632 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/memory_tool.py#L626)); injection at agent/system_prompt.py:483–492.
- **Trade-offs:** facts learned mid-session are prompt-invisible until next session — freshness traded deliberately.
- **Related patterns:** entries 7, 15; insight 1.

### 10.2.3 Learning-loop and autonomy entries (12–14) plus PIONEER entries

**12. Zero-LLM Structured Retrieval over Session History** (deterministic FTS recall)
- **Status:** CANONICAL.
- **Intent:** cross-session recall that is deterministic, inference-free, inspectable.
- **Problem/Context:** LLM-summarized recall drifts and costs; embeddings add infrastructure and misses.[^4^][^5^]
- **Solution:** `session_search` runs BM25 over three FTS5 indexes (unicode61; trigram over a tool-role-excluding view — ~90% of bytes; CJK bigram), returning real messages in four shapes as an ordinary tool result.
- **Structure:** tool → SQLite → ranked windows (Chapter 6).
- **Code evidence:** "No LLM calls anywhere," tools/session_search_tool.py:1–29 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/session_search_tool.py#L1)); hermes_state.py:1187–1392.
- **Trade-offs:** lexical recall misses paraphrases; marker-gated reindexing is deferrable complexity.
- **Related patterns:** entries 6, 15; CoALA episodic [^4^].

**13. Background Reflection Fork with Cache Parity** (post-hoc reflection)
- **Status:** CANONICAL — Ng's Reflection [^3^]; the cache-parity fork is the novel mechanic inside this canonical entry.
- **Intent:** learn from every turn without competing with it — or doubling its cost.
- **Problem/Context:** in-loop critics steal attention; a naive second transcript pass doubles spend.[^8^]
- **Solution:** cadence counters (10 user turns; 10 tool iterations) fire a whitelisted second `AIAgent` on a daemon thread after successful turns — persistence disabled, compression off, approvals auto-denied — pinning the parent's cached prompt, `session_start`, and `session_id` for cache reuse (~26% saving; single-source in-code figure).
- **Structure:** Figure 3 (Chapter 7).
- **Code evidence:** agent/background_review.py:617–953; pins :779–789 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/background_review.py#L779)).
- **Trade-offs:** without parity, spend roughly doubles on review turns [INFERRED]; the pins couple the fork to prompt internals.
- **Related patterns:** entries 14, 16; evaluator-optimizer [^1^].

**14. Curator Archive-Never-Delete Lifecycle** (decay with tombstones)
- **Status:** CANONICAL.
- **Intent:** decay an accumulating skill library without chores or data loss.
- **Problem/Context:** bias-to-act learning produces sediment; deletion makes bad passes unrecoverable.
- **Solution:** inactivity-triggered runs (168 h; first run deferred) transition agent-created skills 30 d → stale, 90 d → archive, reuse reactivates; pinned, cron-referenced, never-used, and hub skills exempt; archive only, never delete; five-deep snapshot with rollback. LLM consolidation off by default.
- **Structure:** Figure 3 maintenance path (Chapter 7).
- **Code evidence:** agent/curator.py:233–283, :305–383 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/curator.py#L233)); never-delete :17; snapshot :1544–1551.
- **Trade-offs:** archives accumulate; the anti-bloat consolidation pass is the one judgment guard left off.
- **Related patterns:** entries 13, 16.

**15. Agent-Curated File Memory** — PIONEER #1
- **Status:** PIONEER. Memory frames converge on vector or provider-backed stores [^4^][^5^]; agent-edited bounded Markdown without a similarity index has no clean precedent.
- **Intent:** durable cross-session semantic memory the agent curates, at zero infrastructure and cache cost.
- **Problem/Context:** vector memory adds weight and misses; append-to-prompt memory busts caches.
- **Solution:** `MEMORY.md` (2,200 chars) / `USER.md` (1,375), §-delimited; one bounded `memory` tool edits by unique substring under locks, atomic renames, drift guards — deliberately no `read` action.
- **Structure:** tool → atomic file → frozen snapshot (entry 11) → next session (Chapter 6).
- **Code evidence:** tools/memory_tool.py:3–25, :55–57 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/memory_tool.py#L55)); substring errors :398–399.
- **Trade-offs:** no semantic recall (entry 12 covers recall); the ceiling holds only what the model judged worth compressing; quality is model-dependent.
- **Related patterns:** entries 11, 12; OpenClaw workspace files as nearest peer [^17^][^19^].

**16. Self-Authored SKILL.md Procedural Memory** — PIONEER #2
- **Status:** PIONEER. The skills standard assumes human authors,[^13^][^14^] and ReAct's named limitation is unpersisted learning [^8^]; self-authorship under provenance and decay is unframed.
- **Intent:** convert episodic success into reusable, portable procedural knowledge.
- **Problem/Context:** trajectories end; each session rediscovers procedures.
- **Solution:** the reflection fork and `/learn` author standard `SKILL.md` packages (≤60-char description, fixed sections, literal `author: Hermes`); ContextVar provenance marks `agent_created`; read-before-write; 100 KB cap; decay applies only to agent sediment, never user or hub skills.
- **Structure:** Figure 3 (Chapter 7); Figure 4 ring position (Chapter 4).
- **Code evidence:** tools/skill_provenance.py:37–45; tools/skill_manager_tool.py:488, :1421–1427 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/skill_manager_tool.py#L1421)).
- **Trade-offs:** wrong-but-plausible procedures accumulate unmeasured; one-offs harden into self-cited refusals.
- **Related patterns:** entries 6, 13, 14.

**17. Natural-Language Cron with Isolated Sessions** — PIONEER #3
- **Status:** PIONEER. Proactivity frames offer heartbeats and fixed cron [^11^][^12^]; the model as schedule parser plus a fresh capability-stripped session per fire is unframed.
- **Intent:** proactivity as a tool the agent itself wields, safely.
- **Problem/Context:** fixed cron is rigid; heartbeats block and drift; unattended agents can recursively self-schedule.
- **Solution:** the model emits one of four validated schedule shapes; each fire builds a fresh historyless `AIAgent` with skills injected; `next_run_at` advances before execution (at-most-once); cron/messaging/clarify toolsets stripped in code; provider pinned fail-closed.
- **Structure:** fresh session → delivery → next-run update (Chapter 8).
- **Code evidence:** cron/jobs.py:512–609 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/jobs.py#L512)); cron/scheduler.py:156–176, :4029–4035.
- **Trade-offs:** well-formed-but-wrong schedules fail silently; a post-advance crash is a missed run, never a duplicate.
- **Related patterns:** entries 13, 18; heartbeat/cron frames [^11^][^12^].

**18. Capability-Subtraction Guardrails** — PIONEER #4
- **Status:** PIONEER. HITL frames center approval gates [^9^][^10^]; schema-level capability removal as the *primary* control — instruction only as backup — is argued as distinct; the strongest transfer case.
- **Intent:** make runaway or recursive behavior physically impossible, not discouraged.
- **Problem/Context:** prompt-level refusals are ignorable under injection; spawned contexts inherit the parent's full surface.
- **Solution:** `DELEGATE_BLOCKED_TOOLS` strips delegation, memory, cron, messaging from subagents; cron sessions lose three toolsets in code with `check_fn` as a second layer; yolo/redact toggles freeze at import.
- **Structure:** Chapter 8's three-surface table; Chapter 9.1.
- **Code evidence:** tools/delegate_tool.py:47–54 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L47)); cron/scheduler.py:156–176; tools/approval.py:32–35.
- **Trade-offs:** subtraction is coarse — it removes legitimate uses too — and each new spawn context must re-derive its strip.
- **Related patterns:** entries 5, 13, 17; insight 4.

## 10.3 Coverage Map and Deliberate Absences

### 10.3.1 Coverage-map walk-through

Figure 5 is the chapter's anchor artifact: every entry points at the module(s) instantiating it, and every core module points back at its pattern(s). The table is the same map in tabular form.

![Figure 5: Patterns-to-modules coverage map — each catalog entry points at its primary module(s); core modules point back at their pattern(s); substrate modules (environments, providers, approval) sit beneath the patterns they carry.](/mnt/agents/output/diagrams/05_patterns_map.png)

| Pattern (entry) | Primary module(s) | Host chapter |
|---|---|---|
| Loop-as-State-Machine (1) | `agent/conversation_loop.py`, `agent/turn_retry_state.py` | Chapter 3 |
| Prologue/Epilogue Turn Seams (2) | `agent/turn_context.py`, `agent/turn_finalizer.py` | Chapter 3 |
| Iteration Budget Consume/Refund (3) | `agent/iteration_budget.py` | Chapter 3 |
| Self-Registering Registry (4) | `tools/registry.py`, `model_tools.py` | Chapter 4 |
| Capability Gating, check_fn (5) | `tools/registry.py:143–206, 558–576` | Chapter 4 |
| Progressive Disclosure (6) | `model_tools.py`, `agent/prompt_builder.py` | Chapters 4, 6 |
| Three-Tier Prompt Assembly (7) | `agent/system_prompt.py`, `agent/prompt_caching.py` | Chapter 5 |
| Byte-Identical Replay (8) | `agent/turn_context.py`, `agent/conversation_loop.py` | Chapter 5 |
| Dual-Trigger Compression (9) | `agent/context_compressor.py`, `agent/context_engine.py` | Chapter 5 |
| Three-Layer Result Budget (10) | `tools/tool_result_storage.py`, `tools/budget_config.py` | Chapters 5, 9.4 |
| Frozen-Snapshot Injection (11) | `tools/memory_tool.py` | Chapters 5, 6 |
| Zero-LLM Retrieval (12) | `hermes_state.py`, `tools/session_search_tool.py` | Chapter 6 |
| Background Reflection Fork (13) | `agent/background_review.py` | Chapter 7 |
| Curator Lifecycle (14) | `agent/curator.py`, `agent/curator_backup.py` | Chapter 7 |
| Agent-Curated File Memory (15) | `tools/memory_tool.py` | Chapter 6 |
| Self-Authored SKILL.md (16) | `tools/skill_manager_tool.py`, `tools/skill_provenance.py` | Chapters 6, 7 |
| NL Cron, Isolated Sessions (17) | `cron/jobs.py`, `cron/scheduler.py` | Chapter 8 |
| Capability-Subtraction (18) | `tools/delegate_tool.py`, `cron/scheduler.py`, `tools/approval.py` | Chapters 8, 9 |

Reading the map in both directions is the point. Pattern-to-module, no entry is homeless: every mechanism resolves to at least one file at the snapshot, which makes the admission rule of §10.1.1 checkable. Module-to-pattern, the dense nodes match where the chapters put their weight: `conversation_loop.py` carries entries 1, 2, 8; `tools/registry.py` carries 4 and 5; `tools/memory_tool.py` carries 11 and 15; `cron/scheduler.py` carries 17 and part of 18. Two honest gaps in the reverse direction deserve naming. First, the platform substrate — `tools/environments/`, the provider layers, the approval chain — has no pattern entry: it is the replace surface the patterns run on, treated in Chapter 9 as seam rather than mechanism, and reappears in Part B as the core's expectations. Second, the verify-on-stop ledger (`agent/verification_evidence.py`, `agent/verification_stop.py`) sits inside entry 13's host subsystem but is deliberately not an entry: it verifies truthfulness within a turn, not a reusable cross-turn mechanism — Chapter 7.3.1 scopes it as a floor, not a fitness function.

### 10.3.2 Deliberately absent patterns

The following canonical patterns are not instantiated at the snapshot. Absence is information: each states what the absence means for the clone decision, in the same neutral register as the catalog. The set is consistent with the loop-centric thesis — complexity pushed into data the loop reads, not orchestration scaffolding — not a list of gaps.

- **Planner–executor split.** Planning is loop-implicit, externalized only through kanban/goals tools. Clone decision: skip the planner; the stale-plan failure mode arrives only if you add one.
- **DAG/workflow-graph orchestration.** Control flow lives in the model; skills encode procedures as prose.[^1^][^8^] Clone decision: do not buy a graph framework; loop plus registry replaces it.
- **Tree search / backtracking** (ToT, LATS). Traded for latency and simplicity; `tools/checkpoint_manager.py` gives environment-level undo. Clone decision: copy checkpoint/rollback if your environment is destructive; skip search.
- **Multi-agent debate / role-play.** MoA covers many-perspectives on demand; persistent swarms would conflict with the profile-isolated single-identity model [INFERRED]. Clone decision: defer MoA; add only if voting measurably helps your task mix.
- **In-loop evaluator-optimizer pair.** Replaced by the reflection fork and verify-on-stop; fitness-measured self-modification lives only in the external, PR-gated self-evolution repo.[^15^][^16^] Clone decision: keep it that way — in-loop fitness would demand the human-review gates the project reserves for the companion.
- **Vector/embedding memory as default.** File memory plus FTS5 sessions cover recall; vector stores are opt-in plugins. Clone decision: start dependency-free; adopt embeddings only when recall misses are observed.
- **Exactly-once cron delivery.** At-most-once by design: advance-before-execute makes a crash a missed run, never a duplicate (`cron/scheduler.py:4029–4035`). Clone decision: accept auditable misses; exactly-once costs a durable intent log plus idempotent side effects at every target.

## 10.4 Part B — Clone Roadmap

### 10.4.1 Scope and seam

The roadmap clones the **agent core only**; channels, the gateway, ACP, batch, and the API server are named solely to be excluded — entry points, not core (Chapters 1, 9). The core *expects*: turn input (a user message plus optional system context); an environment honoring the two-method `BaseEnvironment` contract (`_run_bash`, `cleanup`; Chapter 9.2); approval decisions via a callback the gate invokes fail-closed; and a delivery target for output and async completions. The core *exposes*: lifecycle events (the `AIAgent` callback surface, Chapter 3), on-disk artifacts (memory files, skills, trajectories), and `state.db` (sessions, messages, lineage, cost; Chapter 6). A host supplying the four expectations and consuming the three exposures can swap entry points without touching the core — how one `AIAgent` serves five front ends.

### 10.4.2 Minimal viable core

The smallest subset that still runs one full turn: provider path, loop, registry with a handful of tools, one environment, approval — steps 1–4 below, step 5 strongly advised. Everything later is additive data the loop reads. This is insight 7's conclusion, reached independently by every dimension's clone guidance: the minimal viable Hermes is roughly 10–15% of the codebase, converging on the same dependency order. Chapter 3 anchors the claim: the loop's load-bearing regions are small and extractable; the rest of its ~3,900 lines is additive robustness with named, deferrable failures.

### 10.4.3 Staged 10-step build order

Figure 6 shows the order as a dependency graph. Each step has a done-state and one primary risk. LOC figures are convergent estimates from the per-dimension clone-guidance sections (insight 7) — estimates, never measurements. Where per-chapter clone notes assume broader scope they run higher (Chapter 4: ~600 registry plus ~1,500 tools; Chapter 6: ~300 skills; Chapter 7: ~400 fork); the figures below assume the minimal core of §10.4.2.

![Figure 6: Clone roadmap — staged build order with dependencies; steps 1–4 form the minimal viable core, steps 5–7 add the data layers, steps 8–10 add learning and autonomy.](/mnt/agents/output/diagrams/06_clone_roadmap.png)

1. **Provider path (~350).** One streaming `chat_completions` transport with basic retry; skip auth registry and plugins. Done: one streaming call round-trips a tool schema. Risk: streaming/tool-call edge cases. Keep the OpenAI-shaped representation as the day-one invariant (Chapter 9).
2. **Core loop (~600).** Outer budget while, tool-call round-trips, `IterationBudget` with summary-call fallback, sequential dispatch. Done: one turn runs end to end. Risk: consume/refund correctness (Chapter 3).
3. **Registry + a handful of tools (~300).** Import-time registration, OpenAI-format schemas, JSON-string handler contract; a static import list is an acceptable v1 substitute for the AST gate. Done: the model calls a file tool in the loop. Risk: handler-contract drift — errors as `{"error": ...}`, never raised (Chapter 4).
4. **Environment + approval (~500).** One local `BaseEnvironment` honoring the two-method contract; fail-closed approval with hardline floor and frozen-at-import toggles. Done: a dangerous command is gated; a timeout denies. Risk: containment trust concentrated in one backend — state it, as Chapter 9 does.
5. **Context pipeline (~300).** Three-tier assembly plus dual-trigger compression behind a swappable engine contract. Done: a long session compresses without losing the task. Risk: boundary bugs splitting tool pairs (Chapter 5).
6. **Memory layer (~200).** `MEMORY.md`/`USER.md`, §-delimited, char budgets, substring targeting, atomic writes, frozen-snapshot injection. Done: the agent edits its own `MEMORY.md`. Risk: none structural; staleness is by design (Chapter 6).
7. **Skills (~250).** ~150-line front-matter parser, one discovery root, `skills_list`/`skill_view`/`skill_manage`, index into the stable tier. Done: `skill_view` loads a body on demand. Risk: ignored load preamble collapses routing (Chapter 6).
8. **Learning fork (~300).** Cadence counters, whitelisted second agent with persistence disabled, curator-lite stale/archive with snapshot/rollback. Done: a fork writes a skill after ten iterations. Risk: fork lifecycle bugs; if time-pressed, start with `/learn`-style foreground distillation (Chapter 7).
9. **Delegation (~270).** Child factory (fresh agent, goal+context prompt, parent toolsets minus blocklist), pool runner, summaries-only fan-in, depth-1 guard. Done: parent receives bounded summaries, never child transcripts. Risk: runaway recursion if the ~50-line blocklist is omitted — implement it day one (Chapter 8).
10. **Cron (~300).** JSON job store with atomic writes, tick loop, fresh-session executor with toolset strip, advance-before-execute. Done: first cron fire runs an isolated session. Risk: silent schedule mistranslation — bound it with the four-shape whitelist (Chapter 8).

### 10.4.4 Defer-list and copy-early guards

Defer, with justification: **MCP and Tool Search** — an external-process trust surface adding failure modes before capability. **Provider plugins beyond one** — one transport proves the invariant; the 33-plugin machinery and OAuth chain come later. **Skills Hub** — untrusted-text intake is a security project of its own. **execute_code RPC scripting** — a second dispatch surface presupposing a stable dispatcher. **Three-index FTS5** — solves scale the clone does not yet have. **MoA, gateway/ACP bindings, batch_runner, datagen** — product and research surfaces outside the core by definition. **GEPA/DSPy evolution** — external to the original repo by design; replicating it is a research program, not a clone step.[^15^][^16^]

Copy early — each is cheap and prevents an entire bug class: `check_fn` gating at schema emission (~50 lines; Chapter 4); archive-never-delete with snapshot/rollback (Chapter 7); the provenance flag on agent-authored writes — unretrofittable once autonomous writes exist (Chapter 6); fail-closed approval with the hardline floor below yolo (Chapter 9); frozen-at-import toggles, so no in-process action can lower a safety setting (Chapter 9).

### 10.4.5 Decision register

**Preserve verbatim** — the interface is the value: the tool schema shape and handler contract; the `SKILL.md` format, because interop is the bet (Chapter 6); the `api_content` sidecar discipline, *if* the target provider offers prefix caching; the OpenAI-shaped message representation (Chapter 9); the consume/refund budget discipline. **Replace freely** — the interface is incidental: state storage (SQLite/FTS5 is a choice behind the session API, not an invariant); delivery routing; the plugin system; profile multiplexing. The insight-1 fork prices this: targeting a provider without prefix caching deletes roughly 15–20% of the system's complexity — sidecars, replay machinery, frozen snapshots, cache-parity fork, breakpoint placement all collapse into "send the history" (Chapter 5).

**Re-verify before building**, given drift from the July 2026 snapshot: self-improvement mechanics and provider integrations are the most volatile surfaces; the static tool count (74 at snapshot, Medium-confidence single counter, Chapter 4) versus the README's "40+"; documentation locating the loop in `run_agent.py`, stale since the god-file decomposition (Chapter 3, conflict zone C1); the surviving research seams after the RL amputation (Chapter 9.4). Pin your own snapshot and re-run the count scripts rather than trusting any article — this one included.

### 10.4.6 Phasing table

| Phase | Milestone (done-state) | Copy / stub / re-implement | Effort (est.) | Risk |
|---|---|---|---|---|
| 1 — Core turn (steps 1–2) | One full turn runs end to end | Copy transport shape and `IterationBudget`; re-implement minimal loop | ~950 LOC | Streaming edge cases; refund correctness |
| 2 — Capability (steps 3–4) | Model calls gated tools in a real environment | Copy registry contract and approval floor; stub AST gate with a static list | ~800 LOC | Handler-contract drift; containment in one backend |
| 3 — Data layers (steps 5–7) | Session compresses; agent edits memory; skills load on demand | Copy tier assembly, memory tool, front-matter parser | ~750 LOC | Compression boundary bugs; ignored load preamble |
| 4 — Learning & autonomy (steps 8–10) | Fork writes a skill; subagent fan-in; first cron fire | Re-implement fork with guardrails verbatim; copy blocklist; port `parse_schedule` | ~870 LOC | Fork lifecycle; recursion without blocklist; schedule mistranslation |

The table compresses §10.4.3 into buildable phases, and the compression is where its numbers should be read with care. Effort figures are convergent estimates, not measurements; per-chapter clone notes run higher where they assume fuller scope (Chapter 4's ~600 + ~1,500; Chapter 6's ~300 skills; Chapter 7's ~400), so treat the totals as a floor with roughly 1.5× headroom, not a budget. The copy/stub/re-implement column carries the roadmap's real opinion: copy wherever a contract is load-bearing (budget, approval floor, blocklist, provenance), stub wherever machinery outruns need (AST discovery, FTS indexes, async delivery), and re-implement only where the original's size is incident-driven rather than design-driven — the loop and the fork, whose thousands of lines encode provider incidents a fresh codebase has not yet had. Phases 1–2 are the point of no return: later mistakes are additive and recoverable, while a mistake in the refund discipline or the approval floor is silent and systemic. The natural stopping point for most clones is the end of Phase 3; Phase 4 is where the system starts writing its own future prompts, and its guards are the non-negotiable part.

The forward-looking implication closes the study. Individual entries are already diffusing — SKILL.md is a cross-vendor standard, and subagent memory scopes show convergent evolution elsewhere [^13^][^22^] — but the durable transfer is not any single pattern; it is the governing bet that produced them: one boring loop, behavior encoded as auditable data, autonomy governed by subtraction. A clone built on this roadmap inherits an architecture whose improvement is measured by an external, human-gated pipeline rather than by in-repo self-modification — and the re-verify list of §10.4.5 is where to watch that bet evolve.
