# A6 — Extensibility & Infrastructure Subsystems

Repo: `anomalyco/opencode` dev @ a19b52e85bf2 (2026-07-20). All paths relative to repo root. Abbreviations: `OP = packages/opencode/src`, `PL = packages/plugin/src`.

The runtime is built on Effect-TS: every subsystem is a `Context.Service` constructed by a `Layer.effect`, wired into a DAG via `LayerNode.make({ service, layer, deps })` (e.g. `OP/mcp/index.ts:998-1002`). Per-project state is created lazily through `InstanceState.make`, keyed on `{directory, worktree, project}` (`OP/project/instance-context.ts:5-9`), with `Effect.addFinalizer` used for teardown (MCP kill, plugin dispose, LSP shutdown).

---

## 1. Plugin architecture

### 1.1 Hook surface (exact)

The contract is the `Hooks` interface in `PL/index.ts:222-335`. A plugin is `type Plugin = (input: PluginInput, options?) => Promise<Hooks>` (`PL/index.ts:74`). `PluginInput` (`PL/index.ts:56-66`) supplies an SDK client, `project`, `directory`, `worktree`, `serverUrl`, a `BunShell` (`$`), and `experimental_workspace.register()` for workspace adapters (see `PL/example-workspace.ts`).

Hook inventory, with trigger sites:

| Hook | Trigger site | Mutates |
|---|---|---|
| `config(cfg)` | `OP/plugin/index.ts:241-249` (after load) | config object in place |
| `event({event})` | `OP/plugin/index.ts:251-258` (bus subscription, per-directory filter) | — |
| `dispose()` | finalizer, `OP/plugin/index.ts:261-274` | — |
| `tool` map | `OP/tool/registry.ts:194-199` | adds custom tools |
| `auth`, `provider` | provider layer (e.g. built-ins `OP/plugin/index.ts:64-82`) | OAuth methods, model catalogs |
| `chat.message` | `OP/session/prompt.ts:1000` | user message + parts |
| `chat.params` | `OP/session/llm/request.ts:114-132` | temperature/topP/topK/maxOutputTokens/options |
| `chat.headers` | `OP/session/llm/request.ts:134-146` | HTTP headers |
| `shell.env` | `OP/session/prompt.ts:554`, `OP/server/routes/instance/httpapi/handlers/pty.ts:71` | env for shell/pty |
| `tool.execute.before` | `OP/session/tools.ts:106-110` (and MCP path `:402`) | tool args |
| `tool.execute.after` | `OP/session/tools.ts:121-125` | title/output/metadata |
| `command.execute.before` | `OP/session/prompt.ts:1460-1461` | command parts |
| `tool.definition` | `OP/tool/registry.ts:313` | description/parameters sent to LLM |
| `experimental.chat.system.transform` | `OP/session/llm/request.ts:69-73`, `OP/agent/agent.ts:381` | system prompt array |
| `experimental.chat.messages.transform` | `OP/session/prompt.ts:1255`, `OP/session/compaction.ts:350` | full message history |
| `experimental.session.compacting` / `experimental.compaction.autocontinue` | `OP/session/compaction.ts:343`, `:454` | compaction prompt / continue turn |
| `experimental.text.complete` | `OP/session/processor.ts:516` | generated text |
| `experimental.provider.small_model` | `OP/provider/provider.ts:1887` | small-model pick |

Dispatch is `Plugin.trigger(name, input, output)` (`OP/plugin/index.ts:280-293`): hooks run **sequentially in registration order**, each receiving the same mutable `output` bag, and the (possibly mutated) output is returned. Typing restricts `trigger` to hooks matching the `(input, output) => Promise<void>` shape (`TriggerName`, `OP/plugin/index.ts:40-42`).

### 1.2 Loading mechanism

Two classes of plugins (`OP/plugin/index.ts:64-82, 166-238`):

1. **Internal** — compiled-in auth plugins (Codex, Copilot, GitLab, Poe, Cloudflare, Azure, DigitalOcean, Snowflake, xAI), skipped when `flags.disableDefaultPlugins`.
2. **External** — from merged config `plugin` entries, via `PluginLoader.loadExternal` (`OP/plugin/loader.ts:208-236`). Pipeline stages (`loader.ts:86-145`): **install/resolve** (`resolvePluginTarget`, `OP/plugin/shared.ts:207-213` — path specs resolved to `file://` URLs, npm specs installed on demand via `Npm.add` into a global cache, `packages/core/src/npm.ts:115-137` with flock) → **entrypoint detection** (package.json `exports["./server"]` → `main` → directory `index.{ts,tsx,js,mjs,cjs}`, `shared.ts:54,103-169`; a containment check blocks entries escaping the package dir, `shared.ts:89-97`) → **compatibility** (npm packages may declare `engines.opencode` semver range, `shared.ts:194-205`) → **dynamic `import()`**. File-plugin install failures are retried once after `config.waitForDependencies()` (Bun caches failed imports, so only pre-import failures retry, `loader.ts:222-229`). Deprecated npm packages that became built-ins are silently skipped (`shared.ts:10`).

Two module shapes are accepted (`OP/plugin/index.ts:95-121`, `shared.ts:272-304`): the modern default-export `{ id?, server }` (`PluginModule`), or legacy named function exports. File plugins must export an `id`; npm plugins fall back to package name (`shared.ts:306-323`). Config-side, plugins also come from auto-discovery: `{plugin,plugins}/*.{ts,js}` in every config directory (`OP/config/plugin.ts:18-29`), deduped by identity while keeping provenance (`Origin{spec,source,scope}`, `OP/config/plugin.ts:60-78`).

### 1.3 Example & custom tools

`PL/example.ts:4-18` — minimal plugin returning `tool.mytool` built with the `tool()` helper (`PL/tool.ts:45-54`; `tool.schema = z`, Zod args). Plugin tools are adapted into the runtime `Tool.Def` by `fromPlugin` (`OP/tool/registry.ts:120-176`): Zod→JSON-Schema conversion, Effect↔Promise bridging of `ctx.ask`, output truncation. Standalone tool files are also picked up from `{tool,tools}/*.{js,ts}` in config dirs (`registry.ts:178-192`).

### 1.4 What plugins CANNOT do (observed limits)

- **No permission veto.** `"permission.ask"` is declared (`PL/index.ts:261`) but has **zero trigger sites** in this tree (verified by repo-wide grep; only the type exists). Plugins cannot approve/deny permission requests at runtime.
- **No execution cancellation.** `tool.execute.before`/`after` mutate args/output only; a hook cannot abort, replace, or short-circuit a tool call with its own result (the real `item.execute` always runs, `OP/session/tools.ts:111`).
- **No new hook points / no middleware ordering control.** The hook list is closed and execution order is fixed (registration order, sequential — comment at `OP/plugin/index.ts:218-219`).
- **No sandboxing.** Plugins run in-process with full `Bun.$` and filesystem access; failure isolation is limited to per-plugin try/catch at load and per-hook `Effect.promise` without error propagation guarantees.
- **Server/TUI split.** A module exports `server()` XOR `tui()`, never both (`shared.ts:293-295`); server plugins cannot touch UI.
- A next-gen **v2 API** exists (`PL/v2/promise`, `PL/v2/effect`) with a different model — `define({id, setup})`, domain transform hooks (`ctx.agent/catalog/command/skill/...`), runtime hooks (`ctx.aisdk.sdk/language`), `registration.dispose()` (`PL/v2/promise/README.md`) — currently in parallel with the v1 Hooks system.

---

## 2. MCP integration

**Client manager** (`OP/mcp/index.ts`): an Effect service holding `{config, status, clients, defs, instructions}` per instance (`:142-148`). On init all configured servers connect **concurrently** (`Effect.forEach … concurrency: "unbounded"`, `:505-529`). The SDK `Client` advertises only the `roots` capability (sampling/elicitation commented out with issue links, `:39-50`) and answers `ListRoots` with the instance directory (`:75-81`).

**Transports**:
- `type: "local"` → `StdioClientTransport` with `cwd`, merged env, `BUN_BE_BUN=1` for opencode-spawned servers (`:340-357`).
- `type: "remote"` → tries `StreamableHTTPClientTransport` then falls back to `SSEClientTransport` (`:269-284`); custom headers supported. Connection uses `Effect.acquireUseRelease` so failed transports are closed (`:218-232`). Default timeout 30 s (`:38`).

**Lifecycle & status**: `connected | disabled | failed | needs_auth | needs_client_registration` (`:83-107`). `client.onclose` marks failed and publishes `ToolsChanged`; servers push `ToolListChangedNotification` → defs re-listed → `ToolsChanged` event (`:442-472`). Logging notifications are bridged into Effect logs (`:474-490`). Teardown kills the whole stdio process tree (pgrep descendants + SIGTERM) then closes clients (`:531-556`). Runtime `add/connect/disconnect` mutate state (`:627-659`).

**Auth**: full OAuth2 flow for remote servers — dynamic client registration fallback, local callback server (`oauth-callback.ts`), browser open with `BrowserOpenFailed` event, CSRF state check (`:909-913`), `finishAuth` on the pending transport (`:918-942`). Tokens/client-info/code-verifier persist in `~/.local/share/opencode/mcp-auth.json` with file locking (`OP/mcp/auth.ts:34-36`). `oauth: false` disables.

**Tool bridging & namespacing**: tools are cached per server and exposed as `sanitize(clientName) + "_" + sanitize(toolName)` where `sanitize = /[^a-zA-Z0-9_-]→_` (`OP/mcp/catalog.ts:117-119`) — this **is** the collision strategy: server prefix + character sanitization; last-writer-wins if two sanitized keys still collide (plain `result[name] = …`, `mcp/index.ts:684`). `convertTool` adapts each MCP tool to a Vercel-AI-SDK `dynamicTool` (`catalog.ts:42-83`): forces `type:"object"`/`additionalProperties:false`, `callTool` with progress-reset timeouts, `isError` → throw, empty content → serialized `structuredContent`. A tolerant re-list path handles servers whose `outputSchema` breaks validation (`catalog.ts:145-168`). `session/tools.ts:390-424` wraps each with `tool.execute.before/after` plugin hooks and a permission `ask` keyed by the tool name. MCP **resources** are bridged as three synthetic tools `list_mcp_resources`, `list_mcp_resource_templates`, `read_mcp_resource` (`session/tools.ts:27-31, 136-386`) with permission patterns `mcp:<server>:*`; binary blobs → base64 attachments (10 MB cap). Server `instructions` are collected and surfaced for prompt assembly (`mcp/index.ts:615-625`).

**Per-agent enablement**: there is no MCP-specific agent field; MCP tools flow through the generic tool filter — `resolveTools` removes anything denied by the merged agent/session permission ruleset or `user.tools[name] === false` (`OP/session/llm/request.ts:208-214`), and `Permission.visibleTools` is used for code-mode catalogs (`OP/tool/registry.ts:281`).

---

## 3. LSP integration

**Registry** (`OP/lsp/server.ts`): ~39 built-in server descriptors of shape `Info { id, extensions, root, spawn }` (`server.ts:69-74`) — deno, typescript, vue, eslint, oxlint, biome, gopls, rubocop, ty/pyright (mutually exclusive via flag, `lsp.ts:98-108`), elixir-ls, zls, csharp/razor, fsharp, sourcekit, rust-analyzer, clangd, svelte, astro, jdtls, kotlin, yaml, lua, intelephense, prisma, dart, ocaml, bash, terraform, texlab, dockerfile, gleam, clojure, nixd, tinymist, haskell, julia. `root` is computed by `NearestRoot`/`StrictNearestRoot` walking up for marker files (`server.ts:29-65`); many servers auto-install via `Npm`. Config can disable or define custom servers (`{command, env, extensions, initialization}`), merged over built-ins (`lsp.ts:149-189`).

**Lazy spawning** (`lsp.ts:208-297`): nothing spawns at startup. `getClients(file)` maps file extension → candidate servers → root; clients are cached by `root+serverID`, concurrent spawns deduped through a `spawning` promise map, permanent failures recorded in a `broken` set. New clients publish `LSP.Event.Updated`.

**Diagnostics → LLM**: clients keep merged push (`textDocument/publishDiagnostics`, `client.ts:160-172`) and pull (`textDocument/diagnostic`, `workspace/diagnostic`, `client.ts:294-348`) diagnostic maps, deduped (`client.ts:91-105`), with 150 ms debounce and 5 s/10 s wait budgets (`client.ts:13-16`); typescript's first push seeds the cache (`client.ts:116-121`). After every mutating tool, the tool itself pulls diagnostics into its LLM-visible output: `edit.ts:197-201`, `write.ts:75-76`, `apply_patch.ts:269-271` all run `lsp.touchFile(path, "document")` (didOpen/didChange + `waitForDiagnostics`, `lsp.ts:344-362`) then append `LSP.Diagnostic.report(...)` — severity-1 errors only, max 20/file (`diagnostic.ts:20-27`) — as `"\n\nLSP errors detected in this file, please fix:\n<diagnostics …>"`. `read.ts:119` forks a `touchFile` to warm servers in the background. Full diagnostics also land in tool `metadata.diagnostics`. Separately, an experimental `LspTool` (`OP/tool/lsp.ts`, gated by `flags.experimentalLspTool`, `registry.ts:242`) exposes hover/definition/references/symbols/call-hierarchy (`lsp.ts:377-478`) directly to the model.

---

## 4. Config system

**Schema**: Effect Schema (migrated off zod), `ConfigV1.Info` in `packages/core/src/v1/config/config.ts`. Parsing (`OP/config/parse.ts`) is JSONC (comments, trailing commas) + `Schema.decodeUnknownExit` with `errors:"all"` and **strict top-level unknown-key rejection** (`unrecognized_keys`). Parse includes a legacy strip of `theme/keybinds/tui` (`config.ts:53-62`) and auto-injects `"$schema": "https://opencode.ai/config.json"` into files lacking it (`config.ts:231-235`).

**Interpolation** (`OP/config/variable.ts:33-91`): `{env:VAR}` and `{file:path}` substitution before parsing; `~/` expansion; missing files raise `InvalidError` unless `missing:"empty"`; `{file:…}` inside `//` comments is skipped.

**Layering** (`loadInstanceState`, `config.ts:314-596`), lowest→highest precedence, merged with `mergeDeep` (remeda) except `instructions` which **concatenates + dedupes** (`config.ts:41-51`):
1. `https://<auth-host>/.well-known/opencode` remote config (per stored "wellknown" auth, with token env injection, `:356-396`)
2. Global `~/.config/opencode/{config.json,opencode.json,opencode.jsonc}` (+ legacy TOML `config` auto-migrated and deleted, `:258-276`)
3. `OPENCODE_CONFIG` file
4. Project `opencode.json[c]` walking **up** from directory to worktree, root-first (`ConfigPaths.files`, `paths.ts:10-21`)
5. `.opencode` directories (global config dir, ancestors up to worktree, `~/.opencode`, `OPENCODE_CONFIG_DIR`; `paths.ts:23-41`) — their `opencode.json[c]`, plus markdown agents/modes/commands and file plugins (`config.ts:424-466`); each dir gets a `.gitignore` and a background `npm install @opencode-ai/plugin` so local plugins/tools resolve (`config.ts:436-457`, awaited via `waitForDependencies`)
6. `OPENCODE_CONFIG_CONTENT` inline JSON
7. Console org remote config (`:478-514`)
8. Managed config dir + macOS MDM preferences (enterprise override, `:516-534`)

Post-merge derivations: legacy `mode`→`agent` (`:536-543`), `OPENCODE_PERMISSION` JSON (`:545-551`), deprecated `tools` map → permission rules (`:553-564`). Plugin specs are normalized relative to their declaring file and carry provenance (`plugin_origins`, `config.ts:101-115, 330-349`).

**Hot reload**: none for project config — instance config is loaded once into `InstanceState`. Only the *global* file is cached with `Effect.cachedInvalidateWithTTL(…, Duration.infinity)` and re-read on explicit `invalidate()` (called by `updateGlobal` writes, `config.ts:281-289, 633-660`). No file watchers exist in the server tree (grep-verified).

**AGENTS.md aggregation** (`OP/session/instruction.ts`): global candidates `~/.config/opencode/AGENTS.md`, `~/.claude/CLAUDE.md` (first existing wins); project walk-up tries `AGENTS.md` → `CLAUDE.md` → `CONTEXT.md` and **stops at the first filename that matches anywhere** (`findUp … break`, `instruction.ts:121-131`); `config.instructions` entries are globbed relative to every ancestor dir (`globUp`) and `http(s)` URLs are fetched with a 5 s timeout. Each block is prefixed `Instructions from: <path>` (`instruction.ts:149-160`). Additionally, when the agent reads a file, nearest-dir instruction files are attached once per message (`resolve`, `instruction.ts:163+`).

---

## 5. Snapshot / revert

**Shadow-git mechanism** (`OP/snapshot/index.ts`). A dedicated bare-ish git dir per project+worktree: `Global.Path.data/snapshot/<project.id>/<Hash.fast(worktree)>` (`:71`), always invoked as `git --git-dir <shadow> --work-tree <worktree>` (`:75`) — the user's repo is never mutated. Disabled unless `project.vcs === "git"` and `config.snapshot !== false` (`:167-170`).

- **Init** (`track`, `:318-347`): `git init`, tuned for huge trees (`feature.manyFiles`, `index.version 4`, `untrackedCache`), then `seed()` (`:198-233`) writes `objects/info/alternates` pointing at the real repo's object store and copies its index — content already hashed by the user's repo is reused instead of re-hashed.
- **Capture** (`add`, `:235-298`): `diff-files --name-only` + `ls-files --others` enumerate changes; candidates are filtered through the *source repo's* `check-ignore`; untracked files >2 MB are excluded via the shadow `info/exclude`; survivors are staged (`git add --sparse`) and `write-tree` yields the snapshot hash. A semaphore per gitdir serializes all ops (`:55-64, 165`).
- **When**: `processor.ts:102` snapshots before each LLM stream, `:425` at `step-start`, `:436` at `step-finish` — every assistant step records a tree hash on its parts; tool parts carry `patch` parts computed by `patch(hash)` (`:349-380`).
- **Revert** (`OP/session/revert.ts:38-88`): collect `patch` parts after the target message, `track()` the current state (for `unrevert`), then `Snapshot.revert(patches)` — per file `git checkout <hash> -- <file>` (batched ≤100 same-hash non-overlapping paths, `:447-521`); files absent from the tree are deleted. `restore()` is `read-tree` + `checkout-index -a -f` (`:382-406`). The revert diff (`snap.diff`) is stored on the session for the UI.
- **GC**: hourly `git gc --prune=7.days` (`:300-316, 761-766`). `diffFull` renders full before/after patches via `git cat-file --batch` (`:546-759`).

`OP/git/index.ts` is a separate read-only git façade (status/diff/log helpers); `OP/worktree/index.ts` manages `git worktree add` sandbox copies with generated slugs and per-project start commands.

---

## 6. Skills & commands

**Skills** (`OP/skill/index.ts`, `OP/tool/skill.ts`) — discovery sources (`skill/index.ts:21-25, 173-233`):
- `.claude/skills/**/SKILL.md` and `.agents/skills/**/SKILL.md` in `$HOME` and ancestors (compat with Claude Code / agents ecosystems; flag-disableable)
- `{skill,skills}/**/SKILL.md` under every config directory
- `config.skills.paths[]` extra dirs; `config.skills.urls[]` remote bundles — `Discovery.pull` fetches `<url>/index.json` (name/files/version manifest), downloads files into `~/.cache/opencode/skills/<name>`, and refreshes atomically via staging dir + rename + `.opencode-version` stamp (`discovery.ts:49-132`).

Frontmatter: YAML parsed by `ConfigMarkdown.parse` with a permissive fallback sanitizer (`OP/config/markdown.ts:16-34`); only `name` (required) and `description` (optional) are read (`skill/index.ts:53-59`); body = prompt content. Duplicate names warn and overwrite; a built-in `customize-opencode` skill is registered first so disk skills can override it (`:276-283`).

Invocation: the LLM calls the `skill` tool with `{name}` (`tool/skill.ts:12-69`) → permission ask `"skill"` → returns `<skill_content>` with the body, the base directory convention ("relative paths … are relative to this base directory"), and a sampled (≤10) file listing. `Skill.available(agent)` filters by agent permission rules (`skill/index.ts:310-315`).

**Commands** (`OP/command/index.ts`, `OP/config/command.ts`): markdown files in `{command,commands}/**/*.md` with frontmatter `{description, agent, model, subtask}` and body template; name derives from path (`config/command.ts:13-39`). The registry unifies three sources (`command/index.ts:65-157`): built-ins `init`/`review` (text templates), config/markdown commands, **MCP prompts** (lazy `getPrompt` with positional `$1…$n` argument mapping), and **skills** (every skill is also a slash command). `hints()` extracts `$ARGUMENTS`/`$N` placeholders (`:36-44`).

**Vs. plugins**: skills/commands/agents/modes are *declarative* (markdown + frontmatter → prompt text and routing metadata, no code execution, no hooks); plugins are *imperative* code with lifecycle and interception points. Agents themselves are the fourth markdown extension (`{agent,agents}/**/*.md`, `{mode,modes}/*.md`, `OP/config/agent.ts:11-59`).

---

## 7. Design patterns map

| Pattern (canonical) | Where | Notes |
|---|---|---|
| **Plugin/hook architecture** (Observer + Interceptor/Chain-of-responsibility) | `PL/index.ts:222-335`; `OP/plugin/index.ts:280-293` | closed hook set, sequential mutable-output chain; `event` hook is a pure Observer over the bus |
| **Adapter** | `OP/mcp/catalog.ts:42-83` (MCP→AI-SDK `dynamicTool`); `OP/tool/registry.ts:120-176` (plugin Zod tool→runtime `Tool.Def`); `OP/lsp/client.ts` (vscode-jsonrpc) | isolates third-party protocol shapes from the tool loop |
| **Layered configuration** (cascade with provenance) | `OP/config/config.ts:314-596`; `OP/config/plugin.ts:9-17` | 8 ordered sources, `mergeDeep`, per-key concat for `instructions`, origin tracking for plugins |
| **Memento** | `OP/snapshot/index.ts` + `OP/session/revert.ts` | shadow-git tree hashes as mementos; revert = selective restore, unrevert = restore of pre-revert memento |
| **Frontmatter-driven declarative extensions** | skills, commands, agents, modes (`OP/skill`, `OP/config/command.ts`, `OP/config/agent.ts`) | markdown body = prompt, YAML frontmatter = metadata |
| **Dependency injection / Service locator** | `Context.Service` + `Layer` + `LayerNode.make(deps)` everywhere (e.g. `OP/mcp/index.ts:998`) | explicit DAG; `InstanceState.make` gives per-project scoped singletons with finalizers |
| **Registry** | formatters (`OP/format/index.ts:130-158`), LSP servers (`OP/lsp/server.ts`), tools (`OP/tool/registry.ts`) | config entries merge/override registry members by name |
| **Factory + lazy initialization** | LSP `Info.spawn` per server, spawned on first file touch (`lsp.ts:217-289`); MCP connect-on-demand (`mcp/index.ts:648-651`) | `spawning` map dedupes concurrent creation; `broken` set = circuit breaker |
| **Retry/timeout policy** | `withTimeout` (`mcp/index.ts:22`), plugin single retry for file deps (`loader.ts:215-230`), transient-read HTTP retry | — |
| **Sortable prefixed identifiers (ULID-like)** | `OP/id/id.ts` | `ses_/msg_/prt_/…` + 48-bit (timestamp·0x1000+counter) hex + base62 random; `descending` bitwise-NOT for reverse-chron listing; `timestamp()` extraction (`:73-78`) |
| **Background provisioning** | `config.ts:438-457` (npm install forked per config dir, joined by `waitForDependencies`) | non-blocking startup, deterministic join before plugin/tool load |
| **Installation/upgrade policy** | `OP/installation/index.ts` (`Method`, `getReleaseType`, channels) | minor infra |

**Format pipeline** (`OP/format/index.ts`): registry of 15+ formatters (prettier, biome, oxfmt, gofmt, ruff/uv, clang, zig, ktlint, mix, rubocop, standardrb, htmlbeautifier, dart, rlang — `OP/format/formatter.ts`); `enabled()` probes resolve a command once and cache it; after `write`/`edit`/`apply_patch` the tool calls `format.file(path)` which runs every formatter matching the extension with `$FILE` substituted (`format/index.ts:73-116`), then the tool re-reads content before diffing (e.g. `edit.ts:112`) — formatting is thus part of the mutation→diagnostics feedback loop.

**Net assessment**: extensibility is three-tiered — (1) in-process **code plugins** with a closed, sequential hook chain plus provider/auth strategy plug points; (2) **protocol adapters** (MCP, LSP) that normalize external capabilities into the same tool/permission/diagnostics fabric; (3) **declarative markdown extensions** (agents, commands, skills) resolved through the same layered-config cascade. Revert safety is guaranteed out-of-band by the shadow-git memento store rather than by tool-level undo.
