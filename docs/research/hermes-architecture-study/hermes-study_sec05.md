# 5. Context Engineering and Prompt-Cache Economics

The organizing claim of this chapter is insight 1 of this study: prompt-cache economics is the hidden architect of Hermes Agent. Five mechanisms in otherwise unrelated subsystems — storage schema, prompt assembly, memory injection, the background reflection fork, and async completion delivery — exist for one reason: to keep the byte prefix of every API request identical to the previous request's, so provider KV caches hit and the operator is not re-billed for re-prefilling thousands of tokens per turn. This is a load-bearing constraint, not an optimization. What follows specifies the doctrine, the machinery that bounds what it protects, and a cost model for how much of it to keep.

## 5.1 The Byte-Stability Doctrine

Anthropic-style prompt caches and OpenAI-style implicit prefix caches both match on leading bytes; any early mutation invalidates everything after it. Hermes prices a full prefix miss on Anthropic routes at roughly a 75% input-token cost delta, plus re-prefill latency. Hence the doctrine: order every request stable bytes first, volatile bytes last — then never touch the leading bytes for the life of a session.

### 5.1.1 Three-tier prompt assembly

The system prompt is assembled once per session as three tiers joined with `\n\n` — `stable → context → volatile` — cached on `agent._cached_system_prompt`, and rebuilt only after compression (`agent/system_prompt.py:10-19`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/system_prompt.py#L10)). Stable carries identity (SOUL.md or the default), tool guidance, the skills index, and environment/platform hints probed once and never re-probed. Context carries the caller-supplied `system_message` plus at most one project context file (`.hermes.md`, `AGENTS.md`, `CLAUDE.md`, or `.cursorrules`, first match wins). Volatile carries the frozen memory snapshot, user profile, external memory-provider block, and the timestamp line. The in-code contract states that Hermes never reinjects parts of the block mid-session, "which is the only way to keep upstream prompt caches warm across turns" (`agent/system_prompt.py:535-540`).

Ordering does the work. A volatile-tier change (a memory write, a new day) invalidates only the cache tail; a stable-tier change would invalidate everything, so mid-session additions go into the user message or tool results, never the prompt. Session resume likewise restores the persisted prompt verbatim (`agent/conversation_loop.py:434-464`).

The timestamp line is the doctrine in miniature: minute precision would change the prompt on every rebuild path and invalidate prefix-cache KV each time. Hermes renders the date only, `agent/system_prompt.py:503–511`:

```python
    now = _hermes_now()
    # Date-only (not minute-precision) so the system prompt is byte-stable
    # for the full day.  Minute-precision changes invalidate prefix-cache KV
    # on every rebuild path (compression boundary, fresh-agent gateway turns,
    # session resume without a stored prompt).  The model can still query the
    # exact wall-clock time via tools when it actually needs it.
    # Credit: @iamfoz (PR #20451).
    timestamp_line = f"Conversation started: {now.strftime('%A, %B %d, %Y')}"
```

The trade-off is deliberate information loss — the model must call a tool for wall-clock time — against up to 24 hours of byte-stability per rebuild path: the smallest, most liftable instance of the doctrine.

### 5.1.2 Byte-identical replay: the `api_content` sidecar

The doctrine's hardest problem is per-turn injection. Hermes appends ephemeral context — `pre_llm_call` plugin output, external-memory prefetch — to the current turn's user message, not the system prompt. On the next turn that message is history: replay it without the injection and the prefix diverges at exactly that point; persist the injection and it leaks into search, trajectories, and future sessions.

The resolution is a sidecar. The composed wire bytes are stamped onto the message as `api_content` and persisted to the session SQLite store alongside the clean content; one helper produces both, so the sidecar can never drift from the bytes on the wire — the invariant being "what turn N sends must be what turn N+1 replays" (`agent/turn_context.py:50-66`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_context.py#L50)). Later turns substitute the sidecar when building `api_messages` (`agent/conversation_loop.py:1022-1036`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/conversation_loop.py#L1022)); assistant rows can carry an analogous sanitize-divergence sidecar. Three dimension reports verified the mechanism independently across loop, prompt, and storage layers.

The trade-off is byte-fidelity replay versus storage bloat: every injected message is stored twice, once clean and once as sent. That is cheap in SQLite; the alternative — re-deriving injections at replay time — would couple replay to every plugin, so any plugin behavior change would silently break replay. Hermes pays the storage.

### 5.1.3 Anthropic `system_and_3`

Byte-stability keeps the prefix cacheable; Anthropic's explicit cache-control API additionally requires marking where the boundaries are, at a maximum of four breakpoints per request. `system_and_3` places up to four: one on the system prompt, one on each of the last three eligible non-system messages, all at one TTL (`5m` default, `1h` optional). The selection logic, `agent/prompt_caching.py:109-116`:

```python
    remaining = 4 - breakpoints_used
    non_sys = [
        i
        for i in range(len(messages))
        if messages[i].get("role") != "system"
        and _can_carry_marker(messages[i], native_anthropic=native_anthropic)
    ]
    for idx in non_sys[-remaining:]:
```

([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/prompt_caching.py#L84)). The functions are pure transforms — input deep-copied, annotated, returned — so stored history is never decorated. The eligibility predicate `_can_carry_marker` encodes provider reality: on the envelope/OpenAI-wire layout (OpenRouter, Nous Portal, Alibaba), markers on empty-content messages are silently ignored or hang the route, so they are skipped (`agent/prompt_caching.py:52-73`). A provider-policy matrix (`anthropic_prompt_cache_policy`, `agent/agent_runtime_helpers.py:1813-1888`) decides per route whether caching applies and in which layout; the measured justification is that including Kimi on OpenRouter moved cache hits from 1% to 97% within a turn (#25970).

The rolling three-message tail is what lets the strategy survive history churn: tail breakpoints move with the frontier as tool turns append messages, the system-prompt breakpoint anchors the large static block, and after compression invalidates the middle, caching re-establishes within one or two turns. OpenAI-style routes get implicit prefix caching for free from byte-stability alone [INFERRED — no explicit marker path for non-Anthropic routes exists]. The cost to note: model and credential identity participate in the cache key, so a mid-session model switch or credential rotation zeroes the cache.

## 5.2 Compression and Budgeting

Byte-stability determines what must not change; compression and budgeting determine how much of it there is. Together they bound growth of the cached prefix from two directions: total tokens (compression) and per-turn tool output (the three-layer budget).

### 5.2.1 Dual triggers, four phases, and the engine contract

Compression fires from two layers. A gateway-side hygiene pass triggers at 85% of the context window before the agent sees an inbound message — a safety net deliberately set above the in-loop threshold so long sessions do not compress every turn (`gateway/run.py:12832-12861`). The in-loop `ContextCompressor` defaults to 50% of the **main model's** window (never the auxiliary summarizer's), with per-model overrides and a raise-only 0.75 floor under 512K windows against thrash. Three in-loop sites check pressure: a turn-prologue preflight, a pre-API check on the fully assembled request (catching turns that grew via huge tool results), and a post-response check preferring API-reported `prompt_tokens` — completion and reasoning tokens do not consume window (#12026).

`compress()` runs four phases (`agent/context_compressor.py:4256-4650`): (1) prune old tool results over 200 chars outside the protected tail to a placeholder, at no LLM cost; (2) compute boundaries — head is the system prompt plus the first three non-system messages, tail is cut by token budget (20% of threshold) walking backward, both aligned so tool_call/tool_result groups are never split; (3) summarize the middle with the auxiliary client — or, on re-compression, *update* the previous summary with new turns, rehydrated from persisted "fossil" messages after a resume; (4) assemble head + summary + untouched tail, sanitize orphaned tool pairs, and invalidate the cached prompt. Iterative summarization is the main defense against summary drift: each compaction edits an accumulating document instead of re-compressing a moving window. The loop side — the restart that refunds the consumed iteration — is covered in ch3.

The compressor sits behind a plugin contract. `ContextEngine` (`agent/context_engine.py:89-351`) requires a `name`, `update_from_response(usage)`, `should_compress(prompt_tokens)`, and `compress(...)`, plus mandatory token-accounting attributes; lifecycle hooks and engine-exposed tools are optional with safe defaults. Selection is config-driven (config setting → repo-shipped plugins → general plugins → built-in fallback); plugin engines are never auto-activated. The contract lets a lossless engine — one that pages context to disk rather than summarizing — replace the summarizer without host changes.

### 5.2.2 The three-layer tool-result budget

Compression is reactive; the tool-result budget is preventive, and the most production-transferable pattern in this chapter. The doctrine is stated verbatim at the top of `tools/tool_result_storage.py:3-10`:

```python
Defense against context-window overflow operates at three levels:

1. **Per-tool output cap** (inside each tool): Tools like search_files
   pre-truncate their own output before returning. This is the first line
   of defense and the only one the tool author controls.

2. **Per-result persistence** (maybe_persist_tool_result): After a tool
   returns, if its output exceeds the tool's registered threshold
```

([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/tool_result_storage.py#L1)). Layer 1 is decentralized: each tool pre-truncates against config-tunable caps (terminal at 50,000 chars with a 40/60 head/tail split; `read_file` at 2,000 lines). Layer 2, `maybe_persist_tool_result`, fires when a result exceeds its registered threshold: the full output is written *into the sandbox* temp dir — via a stdin pipe, dodging the 128 KB `MAX_ARG_STRLEN` exec-arg ceiling so the spill is reachable on any backend — and the in-context content becomes a ≤1,500-char preview in `<persisted-output>` tags with the path; the model recovers the full text with `read_file` when needed. Layer 3, `enforce_turn_budget`, catches the aggregate case: if one assistant turn's results exceed 200K chars, the largest non-persisted results are force-spilled until the total fits.

Budgets scale to the model window. `budget_for_context_window()` (`tools/budget_config.py:84-114`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/budget_config.py#L84)) computes per-result and per-turn budgets as 15% and 30% of `context_length × 4 chars`, clamped so large models keep the historical 100K/200K defaults as caps and small models floor at 8K/16K — the fix for a 65K-window model blowing its context on one persisted result (#23767). The one pinned threshold is the load-bearing detail: `read_file` is pinned to infinite (`tools/budget_config.py:10-13`). If `read_file` output could itself be spilled, the model would spill a file, read it back, spill the read, and loop; the pin exempts the recovery tool from the mechanism it recovers from. Two dimension reports independently verified the three layers.

## 5.3 The Cost Model

### 5.3.1 The cache-stability mechanism map

The doctrine's footprint spans five mechanisms in five subsystems. The table maps each to its host subsystem and the chapter covering it in depth; this chapter covers the first two and cross-references the rest.

| Mechanism | Host subsystem | In-depth chapter | What it protects |
|---|---|---|---|
| `api_content` sidecar | Session storage / loop replay (`agent/turn_context.py:50-66`) | §5.1.2 | Byte-identical replay of injected messages |
| Date-only timestamps | Prompt assembly (`agent/system_prompt.py:503–511`) | §5.1.1 | Day-long prompt byte-stability |
| Frozen memory snapshot | Memory subsystem (build-time freeze) | ch6 | Volatile-tier immutability within a session |
| Cache-parity fork | Learning loop (background reflection) | ch7 | Child agent reuses parent's warm cache prefix |
| Idle-only completion delivery | Delegation / async events | ch8 | Role-alternation invariant and tail-byte stability |

Read as a set, the five rows make insight 1's case better than any single mechanism can. A storage schema, a timestamp format, a memory refresh policy, a process-fork design, and a message-delivery schedule share no code, no module, and no rationale except one: keep the provider prefix byte-stable so caches hit. Remove the constraint and each mechanism looks like an oddity; restore it and all five are the same decision at different layers. For a builder of any agent, the map doubles as a checklist: whichever of these subsystems you build, the cache-stability question will arrive inside it, and it is cheaper to answer at design time than to retrofit. The trade-offs are real in every row — duplicate storage, lost clock precision, stale facts, fork complexity, delayed notifications — and Hermes pays each one, because the alternative is full prefill cost on every turn.

### 5.3.2 Inverse payoff

The cost model runs both directions. Ignoring cache stability does not break a clone functionally, but it materially raises per-turn cost and sacrifices prefill latency on every provider that offers caching. Conversely, the apparatus is only worth its complexity where a cache exists to hit: a clone targeting providers without prefix caching can delete roughly 15–20% of the system's complexity, as the sidecars, replay machinery, frozen-snapshot discipline, cache-parity fork, and breakpoint placement all collapse into "send the history."

> **Clone notes.** Lift in this order: (1) date-only timestamps and tiered assembly — one day of byte-stability for one line; (2) the three-layer tool-result budget with the `read_file: inf` pin — provider-independent overflow defense that preserves capability via spill-and-preview instead of truncating it; (3) the `ContextEngine` contract so compression policy stays swappable; (4) the `api_content` sidecar only once you route to a prefix-caching provider — until then it is pure storage overhead. The inverse payoff, stated plainly: a provider without prefix caching lets you delete ~15–20% of what this chapter describes. Byte-stability is a bet on your provider's cache; size the bet accordingly.
