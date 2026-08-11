# A2 — OpenCode Tool System: Deep Architectural Analysis

Repo: `anomalyco/opencode` dev branch @ a19b52e85bf2 (2026-07-20). All paths below are relative to `/mnt/agents/output/opencode-study/opencode-dev`. Primary sources: `packages/opencode/src/tool/`, `packages/opencode/src/session/tools.ts`, `packages/llm/src/tool*.ts`, `packages/plugin/src/tool.ts`.

## 1. The Tool contract (`tool/tool.ts`)

The internal contract is **not** the AI SDK's `tool()` and not zod — it is an **Effect-TS** abstraction. Every built-in tool is defined via `Tool.define(id, Effect.gen(...))` (`tool/tool.ts:151-169`), producing an `Info`:

```ts
// tool/tool.ts:55-77
export interface Def<Parameters extends Schema.Decoder<unknown>, M extends Metadata = Metadata> {
  id: string
  description: string
  parameters: Parameters              // Effect Schema.Struct, e.g. read.ts:28
  jsonSchema?: JSONSchema7            // optional pre-computed schema (plugin tools)
  execute(args: Schema.Schema.Type<Parameters>, ctx: Context): Effect.Effect<ExecuteResult<M>>
  formatValidationError?(error: unknown): string
}
export interface Info<...> { id: string; init: () => Effect.Effect<DefWithoutID<Parameters, M>> }
```

The **execution context** (`tool/tool.ts:36-46`) carries: `sessionID`, `messageID`, `agent` (name), `abort: AbortSignal`, optional `callID`, an `extra` bag (used to smuggle `model`, `bypassAgentCheck`, `promptOps` into task.ts — see `session/tools.ts:64`), the prior `messages`, and two callbacks: `metadata({title?, metadata?})` which streams UI-progress updates into the tool-call part, and `ask(...)` which raises a permission request (`PermissionV1.Request`) and blocks until approved/denied. Results are `{title, metadata, output: string, attachments?}` (`tool/tool.ts:48-53`); attachments are base64 file parts (images/PDFs from read.ts:317-324, webfetch.ts:113-122, MCP resources).

`Tool.define` **wraps** every execute (`tool/tool.ts:99-149`) with: (a) schema decoding via `Schema.decodeUnknownEffect` — hoisted once per init for performance (comment at line 109-110); failures become `InvalidArgumentsError` (tool.ts:24-34) whose message is model-facing prose ("Please rewrite the input so it satisfies the expected schema"); (b) OTEL span `Tool.execute` with `tool.name/session.id/message.id/tool.call_id` attributes; (c) **automatic output truncation** unless the tool already set `metadata.truncated` (tool.ts:131-144). So truncation is a cross-cutting decorator, not per-tool code.

The newer layer `packages/llm/src/tool.ts` is a parallel, provider-agnostic contract: `Tool.make({description, parameters, success, execute, toModelOutput, toStructuredOutput})` in **typed** mode (Effect Schema in/out) or **dynamic** mode (raw `jsonSchema`, params typed `unknown`, `_decode: Effect.succeed` — llm/src/tool.ts:159-186). Errors must be `ToolFailure`. `packages/llm/src/tool-runtime.ts:23-36` exposes `ToolRuntime.dispatch(tools, call)` — decode → execute → encode → project to `ToolOutput`, emitting `LLMEvent.toolResult`/`toolError` events. It is wired in only behind the `experimentalNativeLlm` flag (`session/llm.ts` "Runtime seam: native is an opt-in adapter over @opencode-ai/llm", ~line 223); the production path still uses the AI SDK `streamText` with `ai.tool()` wrappers built in `session/tools.ts`.

`packages/plugin/src/tool.ts` is the public custom-tool API — plain zod + promises:

```ts
// plugin/src/tool.ts:45-51
export function tool<Args extends z.ZodRawShape>(input: {
  description: string
  args: Args
  execute(args: z.infer<ZodObject<Args>>, context: ToolContext): Promise<ToolResult>
})
```
with `ToolContext` ≈ the internal Context plus `directory`/`worktree` and promise-based `ask`/`metadata` (plugin/src/tool.ts:3-21).

## 2. Registration & per-agent resolution (`tool/registry.ts`)

`ToolRegistry` is an Effect `Context.Service` whose layer eagerly initializes all built-ins (`registry.ts:96-114`) and assembles per-instance state via `InstanceState.make` (registry.ts:116-249):

- **Built-ins**: invalid, question (gated: only `app|cli|desktop` clients or `enableQuestionTool`, registry.ts:202,228), shell (id `"bash"`, see `tool/shell/id.ts:17` — kept as "bash" for compat), read, glob, grep, edit, write, task, webfetch, todowrite, websearch, skill, apply_patch, plus flag-gated: `execute` (code-mode, `experimentalCodeMode`), `lsp` (`experimentalLspTool`), `plan_exit` (`experimentalPlanMode && cli`) — registry.ts:226-244.
- **Custom tools**: scanned from `{tool,tools}/*.{js,ts}` in every config directory (registry.ts:178-192), dynamically imported as `file://` URLs, each export validated by `isPluginTool` (duck-typing on `args`/`description`/`execute`, registry.ts:350-352), plus `p.tool` entries from loaded plugins (registry.ts:194-199). `fromPlugin` (registry.ts:120-176) is the **adapter**: zod args → JSON Schema via `z.toJSONSchema` with metadata normalization (`zodJsonSchema`, registry.ts:369-416), Effect-bridged `ask`, and post-hoc truncation of string results.
- **Per-model/agent filtering** happens in `tools()` (registry.ts:286-335): `websearch` only for opencode-zen provider or exa/parallel flags (`webSearchEnabled`, registry.ts:58-60,288-290); **edit/write vs apply_patch are mutually exclusive** — `usePatch = modelID.includes("gpt-") && !oss && !gpt-4` → apply_patch for gpt-5 family, edit+write otherwise (registry.ts:292-295). Task's description is dynamically suffixed with the available subagent roster filtered by the caller's `task` permission (`describeTask`, registry.ts:260-273). A plugin hook `tool.definition` can rewrite description/schema per model (registry.ts:313).

Agent definitions do **not** list tools; they carry a `permission: PermissionV1.Ruleset` (`agent/agent.ts:35-55`). Built-ins: `build` (full), `plan` (`edit: {"*": "deny", ".opencode/plans/*.md": allow}`, agent.ts:157-178), `explore` (`"*": deny` then allow grep/glob/bash/read/webfetch/websearch, agent.ts:193-213), `general` (todowrite denied), `compaction`/`title`/`summary` (`"*": deny`). At request time `resolveTools` drops denied tools entirely so the model never sees them:

```ts
// session/llm/request.ts:208-213
const disabled = Permission.disabled(Object.keys(input.tools), ...)
return Record.filter(input.tools, (_, k) => input.user.tools?.[k] !== false && !disabled.has(k))
```

`Permission.disabled` (`permission/index.ts:204-214`) maps `edit|write|apply_patch → "edit"` and MCP-resource tools → `"read"` before matching. Denied-at-runtime (not hidden) tools instead hit `ctx.ask` inside execute, which throws `PermissionV1.RejectedError` — a tool-level **fail-closed gate**.

## 3. Result flow & truncation (`truncate.ts`, `truncation-dir.ts`)

Flow: LLM tool call → AI SDK `execute` wrapper (`session/tools.ts:99-133`) → plugin `tool.execute.before` → `item.execute(args, ctx)` → attachments get PartIDs → plugin `tool.execute.after` → on abort, `processor.completeToolCall` persists immediately. The wrapper is built per assistant message in `SessionTools.resolve` (`session/tools.ts:41-49`), which also constructs the `Tool.Context` (session/tools.ts:59-90): `metadata` → `processor.updateToolCall` (live UI), `ask` → `Permission.ask` with the merged agent+session ruleset. Completion writes a `completed` tool part with output/metadata/attachments (`session/processor.ts:160-184`); failures become `error` parts, with special handling for `PermissionV1.RejectedError`/`Question.RejectedError` (processor.ts:185-200).

Truncation is centralized in the `Truncate` service (`tool/truncate.ts`): defaults `MAX_LINES = 2000`, `MAX_BYTES = 50*1024` (truncate.ts:15-16), overridable via config `tool_output.max_lines/max_bytes` (truncate.ts:75-83). When exceeded, the **full output is written to a spill file** and the model gets a head (or tail) preview plus a hint:

```ts
// truncate.ts:129-131
const hint = hasTaskTool(agent)
  ? `... Full output saved to: ${file}\nUse the Task tool to have explore agent process this file ... Do NOT read the full file yourself - delegate to save context.`
  : `... Use Grep to search the full content or Read with offset/limit to view specific sections.`
```

Spill files live in `TRUNCATION_DIR = Global.Path.data/tool-output` (`truncation-dir.ts:4`), named with ascending `ToolID`s (`truncate.ts:68-73`), and are **garbage-collected hourly with 7-day retention** (`RETENTION = Duration.days(7)`, cleanup fiber at truncate.ts:143-148). The hint is **agent-aware**: only agents whose permission ruleset allows `task` are told to delegate (truncate.ts:28-31) — context-management policy injected at the truncation boundary. Individual tools layer their own caps: read.ts caps 2000 lines / 50KB / 2000 chars-per-line with continuation hints (`Use offset=N to continue.`, read.ts:344-350); glob/grep hard-limit 100 results with "be more specific" hints (glob.ts:48, grep.ts:71); shell streams keep `maxBytes*2` in memory and spill to a file mid-stream when full exceeds maxBytes (shell.ts:491-523).

## 4. Edit/write safety (`edit.ts`, `write.ts`, `apply_patch.ts`)

**Edit** is exact-string-replace with a **cascaded strategy chain** (`edit.ts:682-729`, header credits cline/gemini-cli, edit.ts:1-4). Nine `Replacer` generators are tried in order: `SimpleReplacer` (exact), `LineTrimmedReplacer`, `BlockAnchorReplacer` (first/last-line anchors + Levenshtein similarity ≥ 0.65 over middle lines, edit.ts:220,288-425), `WhitespaceNormalizedReplacer`, `IndentationFlexibleReplacer`, `EscapeNormalizedReplacer`, `TrimmedBoundaryReplacer`, `ContextAwareReplacer` (≥50% middle-line match, edit.ts:588-644), `MultiOccurrenceReplacer` (only valid with `replaceAll` or a unique match). Guardrails: identical old/new is rejected (edit.ts:75-77); empty oldString on an existing file is rejected with "use write for an intentional full-file replacement" (edit.ts:90-96); `isDisproportionateMatch` refuses fuzzy matches whose span is ≥ max(oldLines+3, oldLines*2) or 4×/500 chars larger (edit.ts:709-713,731-737) — a blast-radius limiter on fuzzy fallback; non-unique single matches error with "Provide more surrounding context" (edit.ts:728). **Concurrency**: a per-file `Semaphore(1)` keyed by resolved path (`locks` map, edit.ts:35-45,88) serializes read-modify-write per file. CRLF/BOM fidelity: line endings are detected and preserved (`detectLineEnding`/`convertToLineEnding`, edit.ts:26-33), BOM preserved via `Bom.split/join` (edit.ts:126-135). A unified diff (`createTwoFilesPatch` + `trimDiff` indent-normalizer, edit.ts:646-680) is computed **before** the permission ask and attached to the ask metadata (edit.ts:137-153), so the approval UI shows the exact diff; after writing, `format.file` runs and the diff is recomputed (edit.ts:155-171). Post-write LSP diagnostics are appended to the output ("LSP errors detected in this file, please fix:", edit.ts:196-201; write.ts:74-90 also reports up to 5 other files).

**Fresh-read enforcement**: `edit.txt:4` and `write.txt:5` claim "This tool will error if you attempt an edit without reading the file," but at this commit **no code enforces it** — searches for `markRead|hasRead|wasRead|lastRead|readAt|without reading` across `packages/opencode/src` find no mechanism; edit.ts only re-stats the file at execution time. The "enforcement" is prompt-level only (older upstream releases had a FileTime check; it has been removed/never ported to this Effect rewrite). File-time/optimistic-concurrency checking is likewise absent; the per-file semaphore is the only concurrency control.

**Write** (write.ts) is full-file replace: BOM-aware, diff-first permission ask (`permission: "edit"`, write.ts:54-62), `writeWithDirs` (creates parents), then formatter + `FileSystem.Event.Edited`/`Watcher.Event.Updated` publication (write.ts:64-72).

**apply_patch** coexists with edit/write but is only exposed for gpt-5-class models (registry.ts:292-295). It parses the `*** Begin Patch` envelope (`patch/index.ts`), supports Add/Delete/Update + `Move to`, validates **all** hunks (resolving content, building per-file diffs) **before** a single batched `edit` permission ask (apply_patch.ts:196-207) and before writing anything — an **all-or-nothing validation-then-commit** design. Chunk matching uses exact sequence search with context-line seeking (`seekSequence`, `computeReplacements`, patch/index.ts:339-401) plus Unicode punctuation normalization (patch/index.ts:404+).

## 5. shell.ts — bash execution & tree-sitter permissions

Tool id is `"bash"` (`shell/id.ts:17`). The description and parameters are **rendered per shell/OS** from a template (`shell/prompt.ts` — `ShellPrompt.render(name, platform, limits, defaultTimeoutMs)`, shell.ts:603), injecting OS/shell notes (pwsh vs powershell vs cmd vs bash), truncation limits, and `${tmp}` guidance; shell.txt contains policy ("DO NOT use it for file operations … use the specialized tools"). Shell selection: `Shell.acceptable(cfg.shell)` (shell.ts:600); execution via Effect `ChildProcessSpawner` (`CrossSpawnSpawner` dep in registry.ts:440) — on Windows+PowerShell: `shell -NoLogo -NoProfile -NonInteractive -Command <cmd>`; otherwise `ChildProcess.make(command, [], {shell, detached: posix})` (shell.ts:293-310). **No pty** — stdin ignored, stdout/stderr merged (`handle.all`) into an Effect Stream (shell.ts:487).

**Tree-sitter parsing for permissions**: a lazily-initialized web-tree-sitter with WASM grammars for bash *and* powershell (`parser = lazy(...)`, shell.ts:311-336). Before execution, the command is parsed; every `command` node's name/args are extracted (`parts`, shell.ts:91-117) and scanned (`collect`, shell.ts:378-414): (a) file-mutating verbs (`FILES`: rm/cp/mv/mkdir/cat/chmod…, cmd.exe verbs `CMD_FILES`, PowerShell cmdlets; shell.ts:28-64) have their path arguments resolved (with quoting/`~`/env-var/`${env:}` expansion, `expand` shell.ts:154-160, cygpath translation for git-bash paths on Windows, shell.ts:349-367) and any path outside the instance root triggers an `external_directory` permission ask (shell.ts:263-280); (b) each command's full source becomes a permission pattern, while `BashArity.prefix(tokens)` (`permission/arity.ts:1-9` — a generated dictionary mapping command prefixes to subcommand arity, e.g. `git`→2, `npm run`→3) produces the `"<prefix> *"` **always-allow pattern** (shell.ts:407-410), so approving once can auto-approve semantically equivalent future commands. Permission id is the tool id `"bash"` (shell.ts:283-290).

**Timeouts/streaming**: default 2 min (`flags.bashDefaultTimeoutMs ?? 2*60*1000`, shell.ts:347), per-call `timeout` param; execution races `exitCode` vs abort-signal vs `sleep(timeout+100ms)` (shell.ts:540-546); on timeout/abort the process is killed with `forceKillAfter: "3 seconds"` and a `<shell_metadata>` note tells the model to retry with a larger timeout (shell.ts:561-567,583). Output streams live into `metadata.output` (30KB rolling preview, shell.ts:220-223,498,525-529), in-memory ring of `maxBytes*2`, spilling to a truncation file mid-stream (shell.ts:500-523); final output is tail-biased with UTF-8-safe byte cutting (`tail`, shell.ts:225-255). Plugin hook `shell.env` can inject env vars (shell.ts:416-426).

## 6. task.ts (subagents) and question.ts (elicitation)

**Task** spawns a child session: depth checked against `subagent_depth ?? 1` (task.ts:104-113); `ctx.ask({permission: "task", patterns: [subagent_type]})` unless bypassed (task.ts:115-125); child session created with `parentID`, derived permissions (`deriveSubagentSessionPermission`) plus default denies for `todowrite` and nested `task` unless the agent declares them (task.ts:135-154). It drives the child through injected `promptOps` (`ctx.extra.promptOps`, task.ts:190-204) and returns the child's last text part wrapped in `<task id state><task_result>…` XML (task.ts:63-77). Background mode (`experimentalBackgroundSubagents`) registers a `BackgroundJob`, returns immediately with `BACKGROUND_STARTED` anti-polling instructions, and later **injects the result as a synthetic user message** into the parent (`injectBackgroundResult`, task.ts:206-241); foreground waits on `background.wait` vs `waitForPromotion`, and parent abort cancels the child (task.ts:306-340). The model inherits the parent's model/variant unless the agent pins one (task.ts:175-180).

**Question** (question.ts) blocks the tool call on an Effect `Deferred`: `Question.ask` registers a pending request, publishes `Event.Asked`, and awaits (`question/index.ts:88-114`); the UI/server resolves it via `reply`/`reject` (Deferred completion) — the LLM loop is suspended purely because the tool's promise hasn't settled. Answers come back as `"q"="a1, a2"` formatted text. `RejectedError` ("The user dismissed this question") propagates as a tool error. `plan_exit` reuses the same machinery to ask "switch to build agent?" and then injects a synthetic user message with `agent: "build"` (plan.ts:29-70) — agent switching implemented as a tool + elicitation + synthetic message.

## 7. MCP tool bridging

Two paths. **Full MCP servers**: `mcp/index.ts` manages clients; `MCP.tools()` (mcp/index.ts:666-684) returns defs keyed by `McpCatalog.toolName(client, name) = sanitize(client)_sanitize(name)` (catalog.ts:117-119 — non `[a-zA-Z0-9_-]` → `_`). `McpCatalog.convertTool` (catalog.ts:42-86) adapts each MCP tool into an AI SDK `dynamicTool`: inputSchema forced to `type: "object"` with `additionalProperties: false`, `execute` → `client.callTool` with timeout + `resetTimeoutOnProgress`, `isError` → thrown Error, empty content + `structuredContent` → JSON-stringified text. In `session/tools.ts:390-489` these are re-wrapped: schema run through `ProviderTransform.schema`, permission ask keyed by the full tool id (`ctx.ask({permission: key, patterns: ["*"]})`), content items split into text / image attachments / resource blobs (10MB cap, mime whitelist, omitted-binary notes, tools.ts:436-462), then truncated like any other output. Built-in pseudo-tools `list_mcp_resources`, `list_mcp_resource_templates`, `read_mcp_resource` are injected when any server advertises `resources` (tools.ts:27-31,136-140). Under `experimentalCodeMode`, individual MCP tools are hidden and replaced by a single `execute` tool (code-mode.ts) that runs a confined script against a catalog description (`describeCatalog`, code-mode.ts:64-71; registry.ts:275-284,388).

**Stateless MCP-over-HTTP**: `mcp-websearch.ts` is a minimal hand-rolled client — JSON-RPC `tools/call` POSTs to `mcp.exa.ai` / `search.parallel.ai` with SSE (`data:`) fallback parsing (`parseResponse`, mcp-websearch.ts:30-43); `websearch.ts` picks a provider by config flag, env override, or **deterministic session-checksum A/B split** (`selectWebSearchProvider`, websearch.ts:36-44).

## 8. The .txt description files — prompt-engineering patterns

Descriptions are colocated `.txt` files imported as strings (`read.ts:7`), shell's is a shell-rendered template (`shell/prompt.ts:28-34`). Patterns observed:

- **When-to-use / when-NOT-to-use**: task.txt ("When NOT to use the Task tool: If you want to read a specific file path, use Read or Glob…"), todowrite.txt ("## When to use / ## When NOT to use … When in doubt, use it"), plan-enter/exit.txt ("Do NOT call this tool:").
- **Tool-selection disambiguation**: read.txt steers to grep ("use the grep tool to find specific content in large files") and glob ("use the glob tool to look up filenames"); glob.txt and grep.txt both redirect open-ended searches to the Task tool; grep.txt redirects counting to `rg` via Bash; webfetch.txt defers to better tools ("if another tool is present that offers better web fetching capabilities … prefer using that tool").
- **Negative/constraining instructions**: edit.txt ("ALWAYS prefer editing existing files… NEVER write new files", "Only use emojis if the user explicitly requests"); write.txt ("NEVER proactively create documentation files"); shell.txt ("DO NOT use it for file operations"); git policy embedded in shell.txt ("Only commit … when explicitly requested", "never commit secrets").
- **Error-contract priming**: edit.txt pre-documents the exact failure modes ("The edit will FAIL if oldString is not found… Found multiple matches…") so the model supplies unique context up front.
- **Behavioral nudges for parallelism/context**: read.txt ("Call this tool in parallel…", "Avoid tiny repeated slices (30 line chunks)"); glob.txt ("It is always better to speculatively perform multiple searches as a batch"); task.txt ("Launch multiple agents concurrently…", "The agent's outputs should generally be trusted", instructs writing "highly detailed" prompts because subagents start with fresh context).
- **Format examples**: apply_patch.txt gives a full envelope example; read.txt documents the `1: foo` line-number format (which edit.txt then references for exact-match indentation).
- **Dynamic/temporal grounding**: websearch.txt injects `{{year}}` ("You MUST use this year when searching").
- **Anti-polling/anti-duplication guards** in tool *outputs*, not just descriptions: task.ts BACKGROUND_STARTED ("DO NOT sleep, poll for progress…"), truncation hints.

## 9. Design patterns observed (canonical names → code)

1. **Registry pattern** — `ToolRegistry` service with per-instance state (`InstanceState.make`), `all()/ids()/tools()/named()` queries (registry.ts:72-82,116).
2. **Plugin/Extension-point pattern** — file-system convention scanning (`{tool,tools}/*.ts`), duck-type validation (`isPluginTool`), plus hooks `tool.definition`, `tool.execute.before/after`, `shell.env` (registry.ts:178-199,313; session/tools.ts:106-125).
3. **Adapter pattern** — `fromPlugin` (zod/promise plugin API → Effect Tool.Def, registry.ts:120-176); `McpCatalog.convertTool` (MCP → AI SDK dynamicTool, catalog.ts:42); `session/tools.ts` (internal Def → AI SDK `tool()`); llm layer dynamic mode (raw JSON Schema → typed Tool).
4. **Strategy pattern** — the nine `Replacer` strategies tried in cascade (edit.ts:694-704); shell-specific prompt renderers (shell/prompt.ts); per-provider websearch selection.
5. **Decorator pattern** — `Tool.define`'s `wrap` adds decode-error normalization, tracing, and truncation around every execute (tool.ts:99-149) without tools opting in.
6. **Defensive truncation / spill-over pattern** — central `Truncate` service with head/tail windows, byte+line dual caps, disk spill, retention GC, and agent-aware delegation hints (truncate.ts).
7. **Permission-gate (fail-closed) with capability-style rulesets** — every tool calls `ctx.ask` before side effects; rulesets merged agent+session (`Permission.merge`); hidden-vs-ask split between `Permission.disabled` (request.ts:209) and runtime asks. `BashArity` provides semantic "always allow" prefixes.
8. **Optimistic UI diffing / validate-then-commit** — diffs computed before permission ask and attached as metadata (edit.ts:137-153); apply_patch validates all hunks before any write; write/edit publish watcher events post-commit.
9. **Mutex-per-resource (keyed semaphore)** — per-file edit locks (edit.ts:35-45); the claimed fresh-read optimistic-concurrency check is *not* implemented at this commit (prompt-only).
10. **Deferred-based elicitation (coroutine suspension)** — `Question.ask` Deferred + event bus (question/index.ts:88-114); same mechanism reused by plan_exit and permission asks.
11. **Subagent-as-tool / recursive agent** — TaskTool with depth guard, permission derivation, background-job promotion, synthetic-message result injection (task.ts).
12. **Schema-as-single-source** — Effect `Schema.Struct` annotated with `description` drives both runtime validation and LLM JSON Schema via `ToolJsonSchema.fromSchema` with normalization (null-stripping for optional fields, allOf flattening, $ref inlining, integer min/max clamping; json-schema.ts:8-88), then `ProviderTransform.schema` per-model adaptation (session/tools.ts:98).

### Notable caveats
- Two tool contracts coexist: production `tool/tool.ts` (Effect, AI SDK bridge) vs experimental `packages/llm` (native runtime seam, `experimentalNativeLlm`).
- The `invalid` tool (invalid.ts) exists so malformed model tool-calls can be represented as parts with an error message ("Do not use").
- Fresh-read-before-edit is asserted in edit.txt/write.txt but unenforced in code at a19b52e8.
