## Chapter 9 — Patterns Catalog

### 9.1 Why this catalog

OpenCode is a pattern-dense reference implementation: a production AI coding agent whose loop, tools, permissions, and storage layers each realize several canonical design patterns — often the same pattern twice, at different layers of the stack. This chapter consolidates every pattern observed in Chapters 3–8 into a single catalog, naming each in classic design-pattern vocabulary *and* in the AI-agent pattern language of the companion handbook (ReAct, planning, multi-agent orchestration, guardrails, memory/context management). Every entry carries a file:line citation verified against dev @ a19b52e85bf2; cross-references point to the chapter where the mechanism is treated in depth, so the prose here frames rather than repeats.

### 9.2 Agent-loop patterns

The core cycle is a **hand-rolled ReAct loop** (`packages/opencode/src/session/prompt.ts:1088`, Chapter 3): reason → act → observe, repeated until the model stops requesting tools, with the AI SDK confined to single turns so that exit tests, continuation, and step limits remain OpenCode's own. Around it sit four agent-native patterns. The **doom-loop guard** (`session/processor.ts:356–380`) is an anomaly detector: three identical consecutive tool calls escalate to a `doom_loop` permission, a guardrail against degenerate repetition. **Sliding-window memory with anchored summarization** (`session/compaction.ts:188–239`) together with **tool-output pruning** (`session/compaction.ts:243–287`) implements context management as a first-class loop concern rather than a prompt hack. **Subagent-as-tool orchestration** (`tool/task.ts:104–214`) recurses into the *same* loop under a derived permission ruleset and a depth cap — delegation without a second engine. Finally, **plan/act separation** is realized not as two algorithms but as two agents: `plan` is a permission-restricted profile (`agent/agent.ts:156–181`), and `plan_exit` is a human-approved gate that injects a synthetic agent-switch message (`tool/plan.ts:29–70`) — planning expressed through the permission system (Chapter 6).

### 9.3 Structural patterns

Five classic patterns recur. **Registry**: tools (`tool/registry.ts:96–114`), LSP servers, formatters, and providers are name-keyed registries that configuration merges into and overrides. **Adapter**: every foreign shape is normalized at the boundary — MCP tools (`mcp/catalog.ts:42–83`), plugin zod tools (`tool/registry.ts:120–176`), AI SDK streams (`session/llm/ai-sdk.ts:76+`), LSP diagnostics — an anti-corruption layer per integration (Chapter 8). **Strategy** appears twice with different intent: the nine-replacer cascade in edit (`tool/edit.ts:682–729`) buys tolerance for imperfect model output, while per-model prompt variants (`session/system.ts:27–42`) buy behavioral portability across model families. **Decorator**: `Tool.define` wraps every `execute` in schema validation, tracing, and truncation without tools opting in (`tool/tool.ts:99–149`). **Template method**: the request `compile()` pipeline fixes its phases — options merge → cache policy → body build → validation → transport — with protocol-specific hooks (`packages/llm/src/route/client.ts:344–359`). Beneath everything, **dependency injection** via Effect `Context.Service` plus `LayerNode.make` with declared `deps` (e.g. `mcp/index.ts:998–1002`) keeps the whole service graph explicit and testable.

### 9.4 Concurrency & control patterns

Structured concurrency is where OpenCode diverges most from naive agent scripts. A per-session **Runner finite-state machine** (`effect/runner.ts:33–37`) guarantees one in-flight loop per session and coalesces duplicate prompts by joining their `Deferred`. Human-in-the-loop is a **Deferred rendezvous**: permission and question asks block a fiber on a `Deferred` correlated by request ID over the event bus (`permission/index.ts:98–106`, `question/index.ts:88–114`) — synchronous consumer, asynchronous producer, no polling (Chapter 6). A **keyed mutex** (per-file `Semaphore(1)`, `tool/edit.ts:35–45`) serializes read-modify-write per path. Cancellation is a **fiber-interrupt cascade** running from session to background jobs to the in-flight HTTP request itself (`session/run-state.ts:77–143`, `session/llm.ts:361–364`). The next-generation coordinator adds **wake coalescing**: a `pendingWake` flag collapses redundant drains into one run (`packages/core/src/session/run-coordinator.ts:51–65`).

### 9.5 Data & persistence patterns

Persistence is **event sourcing**: durable events with per-aggregate sequences committed in a single SQLite `immediate` transaction (`packages/core/src/event.ts:205–366`). **CQRS projections** are built by projectors inside that same transaction, and all reads hit the projected tables (`packages/core/src/session/projector.ts`, Chapter 7). **Memento** is implemented as shadow-git tree hashes — snapshots taken before every step make revert/unrevert a selective restore that never touches the user's repository (`snapshot/index.ts:318–347`, `session/revert.ts:38–88`). The V1-over-V2 **dual-write façade** (`event-v2-bridge.ts:35–61`) is a migration pattern: the legacy session service publishes V2 durable events while a bridge reshapes them for legacy consumers — a rewrite without a flag day. Reads page through **cursor pagination** on `(time, id)` (`session/message-v2.ts:425–467`).

### 9.6 Reliability patterns

Retries are deliberately two-layered: an unbounded session-level **retry schedule** honoring `Retry-After` headers with exponential fallback (`session/retry.ts:26–66,176–199`), and a bounded, jittered transport retry with aggressive credential redaction (`packages/llm/src/route/executor.ts:345–364`). Circuit breaking appears twice: the doom-loop guard above, and the LSP **broken set** that permanently stops respawning failing servers (`lsp/lsp.ts:208–297`). Authorization is **fail-closed** — unmatched rules default to `ask` (`permission/index.ts:33–36`), and headless runs auto-reject unless `--auto` is passed. **Defensive truncation** caps every tool output and spills the remainder to disk with a model-readable pointer (`tool/truncate.ts:15–16,129–131`). The native-LLM runtime is a **graceful fallback**: unsupported providers or auth modes fall back to the AI SDK path with a logged reason (`session/llm.ts:225–275`).

### 9.7 Prompt-engineering patterns

Prompt behavior is engineered as code, not prose. **Tool-description engineering** uses colocated `.txt` files with when-to-use/when-not-to-use sections, cross-tool disambiguation, and error-contract priming (Chapter 4). **Cache-breakpoint placement** is policy-driven: three ephemeral breakpoints at the stable prefix boundaries of a turn (`packages/llm/src/cache-policy.ts:18–22`), mirrored by per-SDK marks in the legacy path (`provider/transform.ts:335–384`). **Per-model prompt variants** and capability-based filtering keep one codebase serving 24 bundled provider packages (`session/system.ts:27–42`, `session/llm/request.ts:208–214`). **Layered instructions** aggregate global, project, and remote `AGENTS.md`-style files into attributable blocks (`session/instruction.ts:121–169`). And **error-as-feedback** turns failures into model input: permission rejections carrying user commentary become `CorrectedError` (`packages/core/src/v1/permission.ts:13–19`), and schema violations become prose `InvalidArgumentsError` (`tool/tool.ts:24–34`) — a lightweight reflection loop with no critic model.
### 9.8 The master table

The catalog below consolidates all 35 patterns. "Handbook mapping" gives the closest classic AI-agent pattern; "— (enabler)" marks infrastructure patterns with no direct agent-pattern analog — they are what makes the agent patterns survivable in production.

| Pattern | Category | Where in code | Problem it solves | Handbook mapping |
|---|---|---|---|---|
| Hand-rolled ReAct loop | Agent loop | `session/prompt.ts:1088` | Exit tests, step limits, and compaction need history access SDK steppers lack | ReAct / tool-use loop |
| Doom-loop guard | Agent loop | `session/processor.ts:356–380` | Identical repeated calls burn tokens and time | Guardrails (circuit breaker) |
| Sliding-window compaction | Agent loop | `session/compaction.ts:188–239` | Finite context window vs long sessions | Memory/context management |
| Tool-output pruning | Agent loop | `session/compaction.ts:243–287` | Stale bulk crowds out live context | Memory/context management |
| Subagent-as-tool | Agent loop | `tool/task.ts:104–214` | Delegation with isolated context | Multi-agent orchestration |
| Plan/act separation | Agent loop | `agent/agent.ts:156–181`; `tool/plan.ts:29–70` | Safe analysis before mutation | Planning |
| Hidden specialist agents | Agent loop | `agent/agent.ts:219–264` | Meta-work (titles, summaries) done cheaply out of band | Multi-agent orchestration |
| Registry | Structural | `tool/registry.ts:96–114` | Late binding and config override of capabilities | Tool-use (extensibility) |
| Adapter / anti-corruption layer | Structural | `mcp/catalog.ts:42–83`; `session/llm/ai-sdk.ts:76+` | Heterogeneous protocols behind one event algebra | Tool-use (interoperability) |
| Strategy (replacer cascade) | Structural | `tool/edit.ts:682–729` | Models emit imperfect match strings | Guardrails (robust execution) |
| Strategy (prompt variants) | Structural | `session/system.ts:27–42` | Model families need different personas | Prompt management |
| Decorator (`Tool.define` wrap) | Structural | `tool/tool.ts:99–149` | Validation, tracing, truncation for free | Guardrails (uniform policy) |
| Template method (`compile()`) | Structural | `packages/llm/src/route/client.ts:344–359` | Fixed request pipeline, pluggable protocols | — (enabler) |
| Dependency injection (Effect layers) | Structural | `mcp/index.ts:998–1002` | Explicit service DAG, scoped lifetimes | — (enabler) |
| Runner FSM | Concurrency | `effect/runner.ts:33–37` | One loop per session; duplicate prompts | Guardrails (execution safety) |
| Deferred rendezvous | Concurrency | `permission/index.ts:98–106` | Suspend the loop for a human decision | Human-in-the-loop |
| Keyed mutex (per-file semaphore) | Concurrency | `tool/edit.ts:35–45` | Concurrent edits to one file | — (enabler) |
| Abort cascade | Concurrency | `session/run-state.ts:77–143` | Partial work left behind on cancel | Guardrails (clean teardown) |
| Wake coalescing | Concurrency | `packages/core/src/session/run-coordinator.ts:51–65` | Redundant drains of the same session | — (enabler) |
| Event sourcing | Data | `packages/core/src/event.ts:205–366` | Auditable, replayable, syncable state | Memory (episodic audit log) |
| CQRS projections | Data | `packages/core/src/session/projector.ts` | The log is write-optimized; queries need tables | — (enabler) |
| Memento (shadow-git snapshots) | Data | `snapshot/index.ts:318–347`; `session/revert.ts:38–88` | Reversible file mutation by an agent | Guardrails (reversibility) |
| Dual-write migration façade | Data | `event-v2-bridge.ts:35–61` | Rewrite the core without a flag day | — (enabler) |
| Cursor pagination | Data | `session/message-v2.ts:425–467` | Stable pages over growing history | — (enabler) |
| Retry schedule with Retry-After | Reliability | `session/retry.ts:176–199` | Rate limits and transient 5xx | Guardrails (self-healing) |
| Bounded transport retry + redaction | Reliability | `packages/llm/src/route/executor.ts:345–364` | HTTP flaps; secret hygiene in errors | — (enabler) |
| LSP broken-set circuit breaker | Reliability | `lsp/lsp.ts:208–297` | Failing servers would respawn forever | Guardrails (circuit breaker) |
| Fail-closed defaults | Reliability | `permission/index.ts:33–36` | Unknown actions must never auto-run | Guardrails (default-to-ask) |
| Defensive truncation + spill files | Reliability | `tool/truncate.ts:15–16,129–131` | Unbounded tool output vs context budget | Memory/context management |
| Graceful runtime fallback | Reliability | `session/llm.ts:225–275` | The new LLM stack's incomplete coverage | — (enabler) |
| Tool-description engineering | Prompt | `tool/*.txt` (e.g. `edit.txt`) | Model misuse of powerful tools | Tool-use (prompt design) |
| Cache-breakpoint policy | Prompt | `packages/llm/src/cache-policy.ts:18–22` | Token cost and time-to-first-token | Memory (prefix reuse) |
| Layered instructions (AGENTS.md) | Prompt | `session/instruction.ts:121–169` | Project conventions delivered at the right scope | Memory/context management |
| Error-as-feedback | Prompt | `packages/core/src/v1/permission.ts:13–19`; `tool/tool.ts:24–34` | Failures must teach the model, not just stop it | Reflection (feedback loop) |
| Capability-based filtering | Prompt | `session/llm/request.ts:208–214` | Models see only what they can actually do | Guardrails (least exposure) |

Read as a histogram, the table is dominated by **guardrails** (ten mappings) — fitting for an agent whose whole purpose is mutating real files and running real shell commands — followed by memory/context management (six). Three patterns appear at two independent layers (retry, circuit breaking, adaptation), which reads as deliberate defense in depth rather than ad-hoc patching. Equally telling is what the "— (enabler)" rows imply: roughly a quarter of the catalog is conventional infrastructure with no agent-specific novelty, and it is exactly this unglamorous quarter — DI, pagination, migrations, mutexes — that carries the agent-specific two-thirds in production. Note also that every memory pattern here is *structural* (events, projections, summaries, truncation), never learned: OpenCode's context strategy is engineering, not retrieval.

### 9.9 Patterns notably absent

Just as instructive is what a pattern-dense codebase does **not** contain (the reasoning in this section is interpretation, grounded in the cited code facts):

- **No reflexion / self-critique loop.** No critic model ever reviews an answer before it is shown. The nearest analogs are external: LSP diagnostics appended to edit results (`tool/edit.ts:196–201`) and `CorrectedError` rejections carrying human feedback. For code, ground truth is cheap — compilers, tests, and linters verify — so a second model pass would add latency and cost for little signal.
- **No vector/RAG memory.** There is no embedding store anywhere in the repo; codebase search is `grep`/`glob`, and long-term memory is the compaction summary plus the event log. For a local checkout, ripgrep is exact, fresh, and free, while embeddings go stale with every edit.
- **No planner–executor split beyond plan mode.** `plan` is a permission profile, not a search or decomposition algorithm; there is no task tree, no hierarchical planner, no plan-execute-replan cycle. Task decomposition is delegated to the model's own reasoning within one loop.
- **No multi-agent debate or voting.** Subagents return a single answer, and the task tool's own prompt instructs the parent to trust it (`tool/task.ts`, description engineering in `task.txt`) — consensus machinery would multiply cost on a task whose verifier (the build) already exists.

The common thread is economics: interactive latency budgets, deterministic debuggability (an event-sourced core favors reproducible runs), and a domain whose ground truth is executable. Where the handbook's autonomy-maximizing patterns would spend tokens to reduce uncertainty, OpenCode spends them only where no cheaper oracle exists.

### Architect's take

The patterns worth stealing are the boring ones: own the loop, make guardrails first-class (fail-closed permissions, doom-loop detection, truncation at every boundary), and let tools and compilers play the critic that reflexion loops play elsewhere (this is interpretation). OpenCode demonstrates that a production coding agent is roughly 20% cognition and 80% control engineering — cloners should budget their effort accordingly. If you adopt only three rows from the master table, take the hand-rolled loop, the Deferred-rendezvous human gate, and the shadow-git memento: together they determine whether users trust the agent enough to let it run.
