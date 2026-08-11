## Chapter 10 — Clone Blueprint

Every preceding chapter ends with a takeaway; this chapter assembles them into a build plan. The goal is a *headless agent core* — prompt in, streamed events out — not a terminal UI, not a desktop app, not a gateway business. What follows is the boundary (what to build), the modules (what to copy from where), the sequencing (what to build first), and the traps (what OpenCode itself gets wrong). Effort figures assume one senior engineer fluent with LLM APIs and are marked as interpretation.

### 10.1 The clone boundary

Chapters 2 and 7 established that OpenCode's agent is a client–server core: everything the user perceives as "the agent" is a set of services behind one HTTP/SSE API, and every interface — TUI, desktop, web, IDE, ACP — is a replaceable channel. The clone boundary follows directly. **The agent is**: the session and message model, the step loop, the tool contract with its registry, the permission engine, the provider abstraction, the system-prompt assembly, compaction, and a thin server with one event stream. **The agent is not**: the TUI (OpenTUI/Solid), desktop and web apps, Slack/console/enterprise packages, the ACP adapter, the PTY endpoint, mDNS discovery, session sharing and multi-node sync, and the Zen gateway. Figure 6 draws the line.

![Clone blueprint: twelve core modules to reimplement, channels and extras excluded](diagrams/d6-clone-blueprint.png)

*Figure 6: The minimal agent-only subset. The twelve numbered modules on the left are the clone; the right column is everything OpenCode ships that a headless clone never needs. The suggested build order (provider → prompt → loop → tools → session model → server → events → permissions → compaction → subagents/truncation) reflects dependency order, not importance.*

The exclusion list is as valuable as the inclusion list: OpenCode's monorepo spends most of its packages on channels, and a clone that starts from "a coding agent is a TUI" drowns in rendering code before the first tool call lands. The server-first topology (Chapter 7) means the clone can live as a library with a CLI harness for weeks and gain an HTTP surface in an afternoon.

### 10.2 Minimal viable architecture, module by module

The table maps each clone module to its OpenCode reference implementation — the files worth reading before writing — with a complexity rating (1 = an evening, 5 = a month), the phase needing it (Section 10.7), and whether it can be deferred past the first working version. It lists thirteen rows: the six core tools, drawn as a single box in Figure 6, are broken out individually here because they are built — and read — one at a time.

| Module | Reference implementation in OpenCode | Complexity (1–5) | Clone priority | Can defer? |
|---|---|---|---|---|
| Provider abstraction (single provider first) | `packages/opencode/src/provider/provider.ts`, `session/llm.ts` | 4 | P0 | No |
| System-prompt builder (`<env>` + AGENTS.md) | `session/system.ts:27-66`, `session/instruction.ts` | 2 | P0 | No |
| Step loop (`while (true)` + abort) | `session/prompt.ts:1081-1341` | 4 | P0 | No |
| Session + message/parts model | `packages/schema/src/v1/session.ts`, `session/message-v2.ts` | 2 | P0 | No |
| Tool contract + registry | `tool/tool.ts`, `tool/registry.ts` | 2 | P0 | No |
| Six core tools (read/write/edit/bash/glob/grep) | `tool/{read,write,edit,shell,glob,grep}.ts` | 3 | P0–P1 | Partially (start with 3) |
| Config loader (single file) | `config/config.ts` (clone: 5% of it) | 1 | P0 | No |
| Permission engine (evaluate + ask) | `permission/index.ts:28-38`, `session/tools.ts` | 3 | P1 | No (defer only in toy demos) |
| Output truncation + edit replacer cascade | `tool/truncate.ts`, `tool/edit.ts:682-729` | 3 | P1 | No |
| Compaction + overflow triggers | `session/compaction.ts`, `session/overflow.ts` | 3 | P2 | Yes |
| Task tool (depth-limited subagents) | `tool/task.ts` | 3 | P2 | Yes |
| Thin server + SSE (4–6 endpoints) | `server/routes/instance/httpapi/groups/session.ts`, `handlers/event.ts` | 2 | P3 | Yes (library/CLI first) |
| Event bus (pub/sub; sourcing optional) | `packages/core/src/event.ts` (clone: 10% of it) | 2 | P3 | Partially |

Two judgments embedded in this table deserve emphasis. First, nothing in the clone exceeds complexity 4, and the two 4s — provider abstraction and the step loop — are hard because of streaming edge cases (partial tool-call deltas, mid-stream aborts, provider-specific quirks), not because of architectural depth; a single-provider start converts the first 4 into a 2. Second, the "can defer?" column is intentionally stingy: only compaction, subagents, and the server are truly deferrable, because the remaining modules are what *makes a run safe and useful* — and Sections 10.5 and 10.8 show the failure modes that appear when they are cut. The OpenCode column is also a reading list: roughly 6,000 lines of reference code govern the whole clone, of which a faithful reimplementation needs perhaps 2,000.

### 10.3 The 20% that delivers 80%

Five pieces carry the agent's behavior; get them right and the rest is plumbing.

**The hand-rolled loop.** OpenCode deliberately does *not* use the AI SDK's `maxSteps`/`stopWhen`; it runs an explicit `while (true)` (`packages/opencode/src/session/prompt.ts:1088`) so that it owns the exit test, step accounting, compaction dispatch, and step limits. The exit test is subtle — break only when the last assistant message finished with something other than `tool-calls`, no unfinished tool parts remain, and the last user message precedes the last assistant — because some providers report `stop` while tool calls are pending (comment at `prompt.ts:1103-1109`). A clone that delegates continuation to the SDK inherits none of this control. Copy the boundary: SDK per turn, loop per task.

**The six core tools.** Read, write, edit, bash, glob, grep; everything else OpenCode ships — webfetch, websearch, task, question, skill, todowrite, apply_patch — is additive. The six cover the model's three needs: observe (read/glob/grep), mutate (edit/write/bash), and search the environment it mutates.

**The system-prompt builder.** A per-model family prompt selected by model ID (`session/system.ts:27-42`), an `<env>` block with working directory, git status, platform, and date (`system.ts:60-66`), and AGENTS.md instructions found walking up from the project (`session/instruction.ts:61-66,122`). Cheap to build, disproportionately grounding: the env block alone eliminates whole classes of "wrong path" model errors.

**Permission evaluate + ask.** The engine is thirteen lines: an ordered rule array, last match wins, default `ask` (`permission/index.ts:28-38`). Paired with an ask channel that blocks the tool call until the user replies, it converts an autonomous agent from a liability into a supervised tool.

**Truncation.** Every tool output passes one policy — 2,000 lines / 50 KB caps with a spill file and a pointer to the full content (`tool/truncate.ts:15-16`). Without it, a single `cat` of a log file destroys the context window compaction is supposed to protect.

### 10.4 Safe simplifications

OpenCode carries infrastructure a clone does not need. Five simplifications are safe, with their costs named:

1. **Skip Effect-TS.** The services are plain dependency injection in functional-programming clothing; constructors with explicit dependencies reproduce the architecture. What you lose — structured concurrency (`Effect.ensuring`, fiber interruption) — you replace with disciplined `AbortSignal` threading and `try/finally`, which the loop and permission ask need anyway.
2. **Skip event sourcing.** The durable log with per-aggregate sequences (`packages/core/src/event.ts`) buys replay, multi-node sync, and crash recovery. A single-user clone can persist sessions as JSON files or one SQLite table of messages/parts, plus in-memory pub/sub for streaming. Keep the *event vocabulary* (`session.updated`, `message.part.updated`, `permission.asked`) so the door to durability stays open.
3. **Collapse the config cascade.** OpenCode merges eight layers (`config/config.ts:314-596`); one JSON file with schema validation covers a clone. Add `{env:VAR}` interpolation only when secrets demand it.
4. **Ship one provider first.** The registry abstracts 24 vendors (`provider/provider.ts`); a clone targeting Anthropic *or* an OpenAI-compatible endpoint needs one code path and one message transform. The second provider will force the right abstraction — writing it for the first is speculation.
5. **Skip models.dev.** A static table of the two or three models you support (context limit, output limit, cost, reasoning flag) replaces the fetched catalog; compaction and cost accounting need exactly those fields.

The common shape of these five: they defer *generality*, never *behavior*. The agent acts the same; it just acts in a smaller world.

### 10.5 What NOT to cut

Each of these looks optional until the first long autonomous run. The failure mode each prevents:

- **Output truncation** — without caps, one verbose command (`npm install`, a test suite, a stack trace) floods the context window, and the model hallucinates over a middle it never saw. Failure mode: silent context poisoning.
- **The edit replacer cascade** — exact-string edit fails constantly because models reproduce indentation and whitespace imperfectly. OpenCode runs nine strategies in order (`edit.ts:694-704`); a clone needs at least the first three — exact, line-trimmed, block-anchor with similarity ≥ 0.65 (`edit.ts:220-221,288`). Failure mode: the agent's most important tool errors out on correct intentions, and the model learns to avoid editing.
- **Fail-closed permission default** — unmatched rules resolve to `ask`, never `allow` (`permission/index.ts:33-36`). Failure mode if inverted: every new tool you add is silently pre-approved, including the one that runs shell commands.
- **Compaction trigger logic** — overflow must be detected *before* the provider rejects the request: post-turn checks, mid-stream checks, an error-path trigger (Chapter 3). The tail-preservation budget — last two user turns within `clamp(usable × 0.25, 2k, 8k)` tokens (`compaction.ts:32-34,83`) — is tuned; copy the constants rather than rediscovering them. Failure mode: hard `context_overflow` API errors mid-task, no recovery path.
- **The doom-loop guard** — three identical consecutive tool calls trigger a permission ask (`session/processor.ts:29,356-377`). Failure mode: a stuck model billing you for the same failing call hundreds of times in one unattended run.
- **Abort propagation** — user cancel must interrupt the HTTP stream, the running tool, and the loop, and still finalize the assistant message (Chapter 3's `AbortedError` stamping). Failure mode: zombie runs mutating files after the user walked away, and corrupt half-written message state.

Note what unites these: *cheap guards around expensive failure*. None exceeds a hundred lines; each prevents a run-ending or trust-ending event.

### 10.6 Suggested stacks

| Criterion | TypeScript clone | Python clone |
|---|---|---|
| Runtime | Bun or Node 20+ | CPython 3.12+ with asyncio |
| LLM client | Vercel AI SDK (what OpenCode uses; per-turn `streamText`) | `anthropic` SDK or litellm for multi-provider |
| Schema validation | Effect Schema or Zod — one schema drives validation *and* the LLM-facing JSON Schema | Pydantic v2 (`model_json_schema()` plays the same dual role) |
| Concurrency | `AbortController` + async iterables; closest to upstream's fiber model | `asyncio.Task` cancellation + `async for` streams |
| Streaming events | SSE via any HTTP framework; trivial in Bun | SSE via FastAPI/Starlette |
| Key advantage | Closest to the reference code — chapters 3–7 translate almost line-for-line; the same SDK means the same quirks already solved | Best fit if the clone must embed in a Python application or data stack |
| Key risk | Effect-TS idioms in the source can mislead — read for behavior, not style | Tool-call streaming and provider quirks (tool-call ID rules, reasoning parts) must be re-solved without the AI SDK's normalization layer |

The TypeScript path is lower-risk for a faithful clone: the reference implementation's pivotal mechanisms — the per-turn `streamText` boundary, the `fullStream` event algebra, per-provider message transforms — are AI SDK behaviors, and another ecosystem means re-deriving work the SDK already encodes. Python is the pragmatic choice when the clone's *host environment* matters more than fidelity, e.g. embedding in an existing Python service; litellm then approximates the provider abstraction and Pydantic reproduces the single-schema tool contract. Both stacks can implement everything in this chapter; they differ in how much provider-normalization labor lands on the author.

### 10.7 Phased roadmap

| Phase | Scope | Exit test | Effort (person-weeks) |
|---|---|---|---|
| P0 — Headless core | Single provider; system prompt with `<env>`; hand-rolled loop; read/edit/bash tools; in-memory session model; JSON config | CLI: "fix this failing test" completes a multi-turn run with edits | 2–3 |
| P1 — Safety & fidelity | Write/glob/grep; replacer cascade (3+ strategies); truncation; permission engine with CLI approve/deny; abort; doom-loop guard | Unattended run asks before mutations; giant outputs spill; identical-call loop is caught | 3–4 |
| P2 — Endurance | Compaction with anchored summary + tail preservation; tool-output pruning; task tool with depth 1 | A 50-step refactor survives context pressure; a subagent returns a digest | 2–3 |
| P3 — Server | 4–6 endpoints from Section 7.7's subset; SSE event stream; permission reply endpoint; persisted sessions | Two CLI clients attach to one server and watch the same run live | 1–2 |
| P4 — Extensions (optional) | MCP tools with namespacing; plugin hooks (`tool.execute.before/after`, `chat.params`); LSP diagnostics after edits | An MCP server's tool is callable and permission-gated | 3–6 |

*(Effort is interpretation: calibrated for one senior engineer with LLM-API experience, excluding testing infrastructure.)* The sequencing logic matters more than the numbers. P0 reaches a *behaviorally complete* agent as fast as possible — loop, prompt, three tools — because every later phase is validated by watching real runs. P1 front-loads trust mechanisms before any unattended use, deliberately ahead of endurance features: an agent that runs long but cannot be trusted is worth less than one that runs short and asks. P3 is cheap *because* it comes late — the server is a thin shell over a working core, the lesson of Chapter 7's in-memory `app.fetch`. The path totals roughly 11–18 person-weeks, consistent with OpenCode's own v1 loop, tools, and permission engine each being a few thousand lines, and with Figure 6's estimate that loop + tools + prompt carry ~40% of the core's value.

### 10.8 Pitfalls observed in OpenCode itself

A clone benefits from OpenCode's mistakes as much as its mechanisms. Five, each verified at this commit:

**Dual-stack migration drag.** Two provider stacks (AI SDK vs `@opencode-ai/llm`), two session cores (V1 loop vs the marked-work-in-progress V2 runner), and two plugin APIs coexist behind feature flags (Chapter 5). Lesson: migrate by *replacing* behind a stable interface, not by accreting parallel stacks; every flag is a permanent tax on each reader.

**Documentation drift.** The permissions docs state `.env` files are *denied* by default (`packages/web/src/content/docs/permissions.mdx:174-183`); the code says *ask* (`packages/opencode/src/agent/agent.ts:131-136`). Lesson: generate user-facing default tables from the ruleset source, or assert them in tests — prose rots.

**Dead extension points.** The plugin API declares a `"permission.ask"` hook with zero trigger sites (`packages/plugin/src/index.ts:261`): plugins believe they can veto permissions; nothing calls them. Lesson: a declared-but-unwired hook is worse than none — integrators build against the declaration.

**Prompt-only enforcement.** `edit.txt:4` and `write.txt:5` promise the tool "will error if you attempt an edit without reading the file"; no code enforces it. Lesson: any behavioral claim made to the model should be backed by code or phrased as preference; unbacked promises become silent model-training lies (Chapter 4's counter-lesson).

**Heuristic security boundaries.** Bash path scanning skips redirect targets and refuses to resolve `$(...)`, backticks, and `$VAR` arguments (`tool/shell.ts:99,174-180`), so `echo x > /outside/file` and dynamically built commands escape the `external_directory` check. Lesson: enforce a promised boundary at the syscall level (containers, seatbelt) or document the evasion class honestly — a heuristic presented as a wall invites misplaced trust (Chapter 6's honest assessment).

### 10.9 Licensing and practical notes

OpenCode is MIT-licensed: the repository root carries an MIT `LICENSE` file ("Copyright (c) 2025 opencode"), confirmed by `"license": "MIT"` in both the root and `packages/opencode` manifests. Code may be copied, modified, and redistributed, including commercially, with the license text preserved. Two caveats remain. First, the MIT grant covers copyright in the code, not the *brand*: "opencode", the logo, and the opencode.ai identity should not be reused for a derivative product — clone the architecture, name your own agent. Second, copying is not coupling: model providers' terms of service, and the licenses of data sources such as the models.dev catalog, apply independently. A clean-room reader can instead treat this study plus the OpenAPI document as the specification and copy no lines at all — the mechanisms in Chapters 3–7 are unpatented, well-understood patterns.

### Architect's take

*(Interpretation.)* The honest summary of this study is that a coding agent's core is smaller than its reputation: a loop, six tools, a prompt builder, a permission gate, and a truncator — perhaps 2,000 disciplined lines — reproduce the behavior that makes OpenCode useful, and everything else is scale, channels, or enterprise. Build in the roadmap's order and resist two symmetric temptations: do not import OpenCode's infrastructure (Effect-TS, event sourcing, the cascade) before you feel its pain, and do not cut its guards (truncation, the replacer cascade, fail-closed defaults) because they look small — they are small the way seatbelts are small. The winning clone is not a smaller OpenCode; it is OpenCode's behavioral core with the courage to stay simple.
