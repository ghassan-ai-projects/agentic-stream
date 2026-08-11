# 6. Memory and Skills: The Self-Authored Data Layer

Hermes keeps one conversation loop and pushes complexity into data the loop reads (chapter 5 established the token economics). This chapter covers the two data layers the agent curates for itself — bounded Markdown files holding facts and user preferences, and `SKILL.md` packages holding procedures — plus the SQLite session store beneath both, which provides recall with no LLM in the path. The unifying bet is that an agent's long-term state should be a diffable, greppable artifact a human can audit with `git diff` and `grep`; the chapter is equally about what that bet forfeits: semantic-similarity recall and scale beyond what fits in a prompt.

## 6.1 Agent-Curated File Memory

### 6.1.1 Two bounded files, edited by the agent

> **Stale-source note (C5).** Older articles place `memory.md` and `user.md` at the config root. The authoritative location is `$HERMES_HOME/memories/MEMORY.md` and `USER.md`, resolved dynamically per profile by `get_memory_dir()` ([tools/memory_tool.py:55–57](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/memory_tool.py#L55)) — the code comment notes the older module-level constant "could go stale if a profile switch happened after the first import." Code at HEAD, snapshot July 2026.

The two stores have distinct subjects. `MEMORY.md` is the agent's notebook — environment facts, project conventions, tool quirks — default budget 2,200 characters; `USER.md` holds what the agent knows about the user — preferences, style, expectations — default 1,375 (`tools/memory_tool.py:5–14`). Entries are delimited by `\n§\n`, may be multiline, and budgets are character-based because "char counts are model-independent" (`tools/memory_tool.py:69`; overrides at `agent/agent_init.py:1601–1604`).

The write mechanics assume a fallible, concurrent world. Replace and remove target a short unique substring rather than an entry ID; multiple matches return an error with previews ([tools/memory_tool.py:398–399](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/memory_tool.py#L398)). There is deliberately no `read` action — the content is always in the system prompt, so a read would only invite thrash. Every mutation runs under an exclusive file lock with atomic temp-file-plus-rename writes (`tools/memory_tool.py:253–288, 769–798`) and is scanned for injection/exfiltration patterns before it lands (`:88–90`). Before any replace, remove, or batch, the on-disk file is round-trip checked; externally appended content (a shell append, a sister session) aborts the mutation and is preserved to `.bak.<ts>` (`:714–767`, issue #26045). Over-budget writes trigger a self-consolidation loop capped at three consecutive failures per turn (`:138, 379–390`), so a fragile write cannot consume the whole turn.

The architectural statement is worth naming plainly: long-term memory is a bounded text file the agent edits, and every property of the design — substring targeting, character budgets, §-delimiters — exists to keep that file human-auditable and machine-editable at once. What is forfeited is equally plain: no embedding index, no similarity recall, and a 2,200-character ceiling that holds only what the agent judged worth compressing into a few dozen lines. The ceiling is not a limitation being worked around; it is the point — the whole store must be cheap enough to live permanently in the prompt.

### 6.1.2 The frozen-snapshot invariant

`MemoryStore` maintains two parallel states ([tools/memory_tool.py:123–132](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/memory_tool.py#L123)): `_system_prompt_snapshot`, captured once at `load_from_disk()` and never mutated mid-session, and live `memory_entries`/`user_entries` that tool calls mutate and persist to disk immediately. The split is stated in the code's own words at tools/memory_tool.py:626–632:

```python
"""
Return the frozen snapshot for system prompt injection.

This returns the state captured at load_from_disk() time, NOT the live
state. Mid-session writes do not affect this. This keeps the system
prompt stable across all turns, preserving the prefix cache.
"""
```

The snapshot is injected exactly once, during system-prompt assembly, into the volatile tier (`agent/system_prompt.py:483–492`). Writes made at turn 40 hit disk immediately — the next session, and any tool response in this one, sees them — but the running session's prompt keeps the bytes it started with. The rationale is chapter 5's: a mid-session prompt mutation would invalidate the provider prefix cache on every subsequent turn, converting a two-kilobyte edit into a full-prefix recompute per call. Freshness is what is deliberately traded away.

> **Insight 1 (cache parity).** The frozen snapshot is one instance of a system-wide constraint: five unrelated subsystems exist partly to keep the prompt prefix byte-stable. A clone targeting a provider without prefix caching can delete this machinery; a clone that ignores it pays for the deletion in per-turn cost.

## 6.2 Session Store and Retrieval

### 6.2.1 One state.db per profile, three FTS5 indexes

Each profile owns one SQLite `state.db` ([hermes_state.py:154](https://github.com/NousResearch/hermes-agent/blob/4c9628e/hermes_state.py#L154)) in WAL mode, holding `sessions` (metadata, lineage via `parent_session_id`, token and cost counters, a snapshot of the assembled system prompt) and `messages` (full history, including the `api_content` sidecar chapter 5 covers). Full-text recall runs over three FTS5 indexes: `messages_fts` (unicode61, external-content, trigger-synced); `messages_fts_trigram` for substring queries, built over a view excluding `role='tool'` rows — roughly 90% of stored bytes are machine noise, so the index stays about 2.6× smaller; and `messages_fts_cjk`, a loadable tokenizer emitting CJK bigrams so one-to-two-character CJK queries hit index speed instead of multi-second LIKE scans (`hermes_state.py:1187–1392`).

The deepest detail is schema v23's marker-gated online reindexing: while a chunked background rebuild runs, two `state_meta` keys (high-water H, progress P) define which rows are indexed, and every sync trigger's `WHEN` clause gates on that predicate, because firing an external-content delete for an unindexed row is the canonical FTS5 corruption hazard. The base index definition is at hermes_state.py:1187–1194:

```sql
FTS_SQL = """
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    tool_name,
    tool_calls,
    content='messages',
    content_rowid='id'
);
```

Queries are BM25-ranked with optional temporal sort, source and role filters, and rewind exclusion, routed by script across the three indexes (`hermes_state.py:7101–7300`).

### 6.2.2 session_search: recall with no LLM in the path

> **Stale-source note (C3).** The README still claims "FTS5 session search with LLM summarization." The summary path was removed; the module docstring at [tools/session_search_tool.py:1–29](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/session_search_tool.py#L1) is explicit. Code at HEAD, snapshot July 2026, is authoritative; lines 21–28:

```python
All three modes operate on the SQLite session DB via the FTS5 index and
the get_anchored_view / get_messages_around primitives in hermes_state.
No LLM calls anywhere — every shape returns actual messages from the DB.

History: PR #20238 (JabberELF) seeded a fast/summary dual-mode split; the
toolkit expansion in PR #26419 (yoniebans) added the anchored drill-down,
bookends, and sort. This module merges all of that into a single calling
shape with no mode parameter, no summary LLM path, and explicit scroll
```

The tool offers four calling shapes inferred from arguments (the docstring predates the fourth, a READ shape at schema line 955): DISCOVERY — an FTS scan deduplicated by session lineage, returning per-session snippets, a ±5-message window, and first/last-three-message "bookends," so the model gets goal → match → resolution without paying for a whole transcript; SCROLL — windowed paging around an anchor; READ — a whole session, head-and-tailed when large; BROWSE — recent sessions with titles. Results return as an ordinary tool-result message; nothing is injected automatically. The consequence: cross-session recall is deterministic, inference-free, and inspectable — you can run the same SQL the tool ran.

### 6.2.3 The MemoryProvider ABC and one-provider exclusivity

Above the built-in layers sits a plugin interface, `MemoryProvider` (`agent/memory_provider.py:43`), with lifecycle methods (`initialize`, `system_prompt_block`, `prefetch`, `sync_turn`, `shutdown`) and optional hooks (`on_pre_compress`; `on_memory_write`, which mirrors built-in file writes to the external backend). Eight providers are bundled — honcho, mem0, hindsight, supermemory, byterover, holographic, openviking, retaindb — and the manager enforces a hard rule: exactly one external provider may be active; a second registration is rejected "to prevent tool schema bloat and conflicting memory backends" ([agent/memory_manager.py:394–416](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/memory_manager.py#L394)). The exclusivity buys a single retrieval path with no federation ambiguity. Prefetched recall is fenced as `<memory-context>` and scrubbed if it leaks into output; a wedged provider is isolated behind an 8-second prefetch timeout and a single-worker sync executor. [INFERRED] The `"builtin"` provider slot reserved in `MemoryManager` suggests a planned first-party provider, but at the snapshot the built-in files are wired directly via `_memory_store`, so the registry holds zero or one external provider in practice.

The three layers, mapped onto the CoALA memory taxonomy the rest of this study uses:

| Layer / store | Mutability | Injection timing | Retrieval path | Budget / cap |
|---|---|---|---|---|
| File memory (`MEMORY.md`, `USER.md`) | Agent-writable, any turn; snapshot frozen per session | Once per session, system prompt | Always present (no retrieval) | 2,200 / 1,375 chars |
| Session store (`state.db` + FTS5) | Append-only by harness; soft-delete on rewind | Never automatic; only via tool result | `session_search` tool, BM25 over three indexes | Unbounded history; ~300-row discovery scan |
| External provider (one max) | Provider-defined; mirrored built-in writes | Per-turn prefetch, fenced into API copy | Provider tools and/or auto-injected context block | Provider-defined; 8 s prefetch timeout |

The triad partitions the CoALA space cleanly. File memory is the semantic store — facts and preferences distilled into durable form — with the frozen snapshot making its injection cost zero after session start. The session store is episodic memory: raw episodes, paid for only when the model asks, which is why the retrieval path can afford to be an ordinary tool call rather than an embedding pipeline. The provider slot is the escape hatch for deployments needing semantic recall at scale, and the one-provider rule acknowledges that adding it is an architectural commitment, not an additive feature. Working memory (the assembled prompt and in-context history) is chapter 5's subject. Procedural memory is deliberately absent from the table; in Hermes it is not a memory layer at all but a separate artifact class — the subject of the next section.

## 6.3 Skills as Procedural Memory

### 6.3.1 SKILL.md: an interop bet, not a bespoke format

A skill is a directory containing a `SKILL.md` plus optional support directories (`references/`, `templates/`, `scripts/`, `assets/`) that are progressive-disclosure data, never discovery roots ([agent/skill_utils.py:50](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/skill_utils.py#L50)). The front matter is declared "agentskills.io compatible" (`tools/skills_tool.py:28–46`): required `name` (≤64 chars) and `description` (≤1024 chars, truncated to ~60 in the index), the rest optional. Hermes extensions are namespaced under `metadata.hermes.*` — `requires_toolsets` for conditional activation, `config` for non-secret settings, `blueprint` for cron-automation packaging — keeping the top level standard-compliant. The format choice is an interop bet: skills written for Hermes should be readable by any agent honoring the shared standard, and others' skills (the Hub, §6.3.4) install without translation. The trade is that Hermes's richer semantics live in a vendor namespace the standard ignores.

### 6.3.2 Three-tier progressive disclosure

The system prompt embeds only a compact index — per category, `- name: description` lines inside `<available_skills>`, wrapped in a mandatory-load preamble at [agent/prompt_builder.py:1732–1758](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/prompt_builder.py#L1732): "If a skill matches or is even partially relevant to your task, you MUST load it with skill_view(name)… Err on the side of loading." The index sits in the stable prompt tier (`agent/system_prompt.py:316–324`), so it rides the prefix cache for the whole session; nothing is ever fully hidden, since `compact_categories` demotes categories to names-only lines under prompt-size pressure rather than removing them.

| Tier | What is loaded | When | Token cost | Invalidation |
|---|---|---|---|---|
| 0 — index | Category + `name: description` lines, ~60-char descriptions | Every session, stable prompt tier | ~3k tokens for ~120 skills | LRU + disk snapshot, mtime/size manifest, 30 s scan signature |
| 1 — body | Full `SKILL.md` body + metadata, injected as a tool result | On `skill_view(name)` — mandatory per preamble on partial relevance | Body size; house style ~100–200 lines | None mid-session; preamble governs re-loads |
| 2 — support files | One file from `references/ templates/ scripts/ assets/` | On `skill_view(name, file_path=…)` | Single file only | None; loaded fresh per call |

The economics mirror the memory triad's logic at one remove. Tier 0 is the standing cost: a few thousand tokens, paid once per session and cache-amortized, buying just enough signal to route — the description *is* the trigger, there is no separate triggers field, which is why the house style's 60-character limit is enforced as "NOT cosmetic." Tiers 1 and 2 make procedure pay-per-use: a two-hundred-line workflow enters context only when a task plausibly needs it, and support files — the long tail of reference material — never enter unless explicitly fetched. The forfeiture is that routing quality rests entirely on short descriptions and a prompt-level mandate; there is no embedding retrieval over skill bodies, so a badly described skill is a skill that silently never loads.

### 6.3.3 skill_manage: writes with provenance

The agent edits its procedural memory through `skill_manage` — `create`, `patch` (old/new string, preferred), `edit`, `delete`, `write_file`, `remove_file` — with guards on every write: front-matter and size validation, a 100 KB cap on agent-authored content (`tools/skill_manager_tool.py:488`), read-before-write inside the background review, and an optional `skills.write_approval` staging gate. The load-bearing guard is provenance. A ContextVar records the write origin — tools/skill_provenance.py:37–45 (eliding comments):

```python
_write_origin: contextvars.ContextVar[str] = contextvars.ContextVar(
    "skill_write_origin",
    default="foreground",
)
BACKGROUND_REVIEW = "background_review"
```

Only skills created inside the background reflection fork are marked `agent_created`; foreground, user-directed creates belong to the user and are curator-exempt ([tools/skill_manager_tool.py:1421–1427](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/skill_manager_tool.py#L1421)). Provenance answers the question every self-modifying system must answer first: who wrote this procedure — a human, the hub, or the agent itself? Without the flag, autonomous maintenance would eventually prune the user's own skills; with it, the decay lifecycle (chapter 7's Curator) applies only to agent sediment. The write-trigger cadences — a skill nudge every ten tool iterations firing a whitelisted background fork — and the Curator's active → stale(30 d) → archive(90 d) transitions are chapter 7's subject, beyond one note here: archive-never-delete is a debugging affordance, not sentimentality — a rotted procedure can be diffed against the version that worked.

### 6.3.4 Skills Hub: untrusted text through a scanning funnel

The Hub is nine `SkillSource` adapters — GitHub taps (openai/skills, anthropics/skills, and others), optional-skills, skills.sh, well-known indexes, URL, ClawHub, Claude Marketplace, LobeHub, Browse.sh — behind one ABC ([tools/skills_hub.py:474–507](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/skills_hub.py#L474); adapter count is a single-source code read). Installs follow one pipeline: fetch → quarantine under `~/.hermes/skills/.hub/quarantine/` → regex scan for exfiltration, prompt injection, destructive commands, and persistence (`tools/skills_guard.py`) → trust-tier policy (`builtin` never scanned; `community` blocks any findings unless forced; `dangerous` never overridable) → install with lockfile and audit log.

> **Insight 5 (the outer ring).** Hermes's extensibility is concentric — registry → toolsets → plugins → MCP → skills — capability, cost, and trust decreasing together moving outward. Skills are the outermost ring and the only layer the agent itself writes: untrusted-but-scanned text in, provenance-tracked text out. The learning loop therefore requires only the cheapest ring to exist.

### 6.3.5 Clone notes

> **Clone notes.** Memory is milestone six of the staged roadmap (~200 LOC: two Markdown files, §-delimited entries, char budgets, one substring-targeted `memory` tool with atomic writes, frozen-snapshot injection at session start). Skills are milestone seven (~300 LOC: a ~150-line front-matter parser, one discovery root, `skills_list`/`skill_view`/`skill_manage`, index injection into the stable tier). Both are individually omittable — the loop runs a full turn without either — and both depend only on the context pipeline, not on each other. Carry the provenance flag from day one; retrofit it after autonomous writes exist and you can no longer tell agent sediment from user property. Defer the trigram/CJK indexes, rebuild markers, Hub, and Curator until scale or untrusted intake actually arrives.
