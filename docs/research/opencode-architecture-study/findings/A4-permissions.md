# A4 — Permission & Security Model (OpenCode, dev @ a19b52e)

Scope: `packages/opencode/src/permission/`, `tool/`, `agent/`, `session/`, `server/`, `cli/`, plus `packages/core` and `packages/schema`. All line refs verified against the checkout.

---

## 1. The Permission Evaluation Engine

### Rule model
A rule is a triple `{permission, pattern, action}` with `action ∈ allow | ask | deny` (`packages/schema/src/v1/permission.ts:16-25`). A **ruleset is an ordered array**; order is the precedence mechanism.

The evaluator (`packages/opencode/src/permission/index.ts:28-38`):

```ts
rulesets.flat().findLast((rule) =>
  Wildcard.match(permission, rule.permission) && Wildcard.match(pattern, rule.pattern))
  ?? { action: "ask", permission, pattern: "*" }
```

- **Last matching rule wins** (`findLast`) — there is no "explicit deny overrides" logic like AWS IAM; specificity is expressed purely by ordering (catch-all `*` first, specific rules after). This matches the docs (`packages/web/src/content/docs/permissions.mdx:91`).
- **Fail-closed default**: no match → `"ask"`.
- The permission *key* is also wildcarded, so `"*": "deny"` matches every permission.

### Wildcard syntax
`packages/core/src/util/wildcard.ts:3-14`: `*` → `.*`, `?` → `.`, everything else regex-escaped, anchored `^…$`. Two subtleties:
- A pattern ending in `" *"` also matches the bare prefix (`escaped.slice(0,-3) + "( .*)?"`, line 11) — `"git status *"` matches `git status` with no args.
- Case-insensitive on Windows (`si` flags), backslashes normalized to `/`.

### Config schema
`packages/core/src/v1/config/permission.ts`: `Rule = Action | Record<pattern, Action>`; `Info` accepts either a single Action (normalized to `{"*": action}`, lines 40-41) or an object with known keys (`read, edit, glob, grep, list, bash, task, external_directory, todowrite, question, webfetch, websearch, lsp, doom_loop, skill`) plus arbitrary extension keys (line 35, `Schema.Record` rest — this is how MCP tool names become permission keys). The comment at lines 14-16 states parsing uses `propertyOrder: "original"` so **user key order is preserved for precedence** — the JSON object order *is* the rule order.

`Permission.fromConfig` (`permission/index.ts:186-198`) flattens config to rules, expanding `~` / `$HOME` at pattern start to the home dir (lines 178-184). `Permission.merge` (lines 200-202) is plain concatenation — precedence = array position. Additional sources: `OPENCODE_PERMISSION` env JSON (`config/config.ts:545-551`) and the deprecated `tools` boolean map, translated to allow/deny rules with `write|edit|patch → edit` (lines 554-566).

### arity.ts — what "arity" means
`packages/opencode/src/permission/arity.ts` is **not** about function arity in the FP sense; it is a **command-prefix dictionary**: how many whitespace-separated tokens constitute the "human-understandable command". `prefix(tokens)` (lines 1-9) does longest-prefix lookup in `ARITY` (e.g. `git: 2` → `git checkout`; `npm run: 3` → `npm run dev`; unknown → first token). Flags never count. The shell tool uses it to build the *always-approve* suggestion: `BashArity.prefix(tokens).join(" ") + " *"` (`tool/shell.ts:409`), so approving `git status --porcelain` "always" whitelists `git status *`, not the literal command. The dictionary itself is LLM-generated (prompt embedded in comments, lines 11-23).

### Tool hiding (deny → remove from schema)
`Permission.disabled` / `visibleTools` (`permission/index.ts:204-219`): a tool whose permission is denied with pattern `*` is **filtered out of the tool list shown to the model entirely** — a first, proactive defense layer before any runtime check. Aliases: `edit/write/apply_patch → "edit"`, MCP resource reads → `"read"`.

---

## 2. The Approval Flow — request/reply over the event bus

### Outbound (tool → user)
1. Tool code calls `ctx.ask({permission, patterns, always, metadata})` (`tool/tool.ts:45`).
2. The context is built in `session/tools.ts:81-89`:
   ```ts
   ask: (req) => permission.ask({ ...req, sessionID,
     tool: { messageID, callID },
     ruleset: Permission.merge(input.agent.permission, input.session.permission ?? []) })
   ```
   Session rules are appended after agent rules → **session rules have highest precedence**.
3. `Permission.Service.ask` (`permission/index.ts:67-107`): each pattern is evaluated against `ruleset` + the in-memory `approved` list. Any `deny` → `DeniedError` (carries the matching rules back to the model as feedback, `core/src/v1/permission.ts:21-27`). All `allow` → proceed. Otherwise a `Deferred` is created, stored in a per-instance `pending` Map keyed by `per_*` ID, and `permission.asked` is published (line 100). The fiber **blocks on `Deferred.await` with no timeout**; `Effect.ensuring` guarantees pending-map cleanup (lines 101-106).
4. Event plumbing: `EventV2Bridge` (`event-v2-bridge.ts:19-44`) tags events with instance location and re-emits onto `GlobalBus`; the server streams them over SSE (`client.event.subscribe()`).

### Inbound (user → tool)
- **HTTP API**: `POST /permission/:requestID/reply` with `{reply: once|always|reject, message?}` (`server/routes/instance/httpapi/groups/permission.ts:31-40`, handler `handlers/permission.ts:16-37`), behind `Authorization` + instance-context middleware. `GET /permission` lists pending requests.
- **TUI**: `packages/tui/src/context/sync.tsx:190` stores incoming `permission.asked` requests; if the TUI's auto-approve mode is on it immediately replies `"once"` (lines 192-199). The dialog UI is `packages/tui/src/routes/session/permission.tsx` (diff preview for edits via request metadata).
- **ACP (Zed etc.)**: `acp/permission.ts` serializes requests per session through a promise queue and bridges to the ACP `session/request_permission` JSON-RPC (lines 38-88); on transport error or missing capability it auto-rejects (lines 57, 72).
- `Permission.reply` (`permission/index.ts:109-167`): publishes `permission.replied`, then:
  - `reject` → fails the Deferred with `RejectedError`, or `CorrectedError` when the user supplied feedback text (the model sees the feedback as the tool error — a correction loop). **All other pending requests of the same session are cascade-rejected** (lines 129-138).
  - `once` → succeeds the Deferred only.
  - `always` → succeeds, appends `{permission, pattern from request.always, action:"allow"}` to the in-memory `approved` array (lines 145-151), then **auto-resolves any other pending same-session requests** now fully covered (153-166).

So the pattern is: **synchronous-blocking consumer, asynchronous producer, correlated by request ID** — a Deferred/promise-based rendezvous over pub/sub, not a bidirectional RPC.

### Non-interactive / headless behavior
There is **no timeout and no server-side default** — the Deferred waits forever unless replied or the instance is torn down (the state finalizer rejects all pending, `index.ts:54-61`). Headless `opencode run` handles this client-side (`cli/cmd/run.ts:796-816`): on `permission.asked`, `--auto`/`--yolo`/`--dangerously-skip-permissions` (flags defined lines 242-274) replies `"once"`; **otherwise it auto-rejects** with a stderr notice. Deny rules are always enforced regardless of auto mode. The `question` tool uses a parallel but separate Deferred-based service (`question/index.ts`), and `plan_exit` gates through it rather than through permissions.

---

## 3. Bash Safety — tree-sitter granular decomposition

`tool/shell.ts` parses every command with **tree-sitter WASM grammars for both bash and PowerShell** (lazy init, lines 311-336) and never regex-splits.

- `commands(node)` = `node.descendantsOfType("command")` (line 123-125) — this finds every simple command **anywhere in the AST**: pipelines (`a | b`), lists (`a && b; a || b`), subshells, command substitutions, and redirected statements. Each is checked individually.
- For every command, `parts()` (91-117) extracts name + word/string/concatenation tokens, **skipping redirections and separators**.
- **File-touching commands** (`FILES` set lines 29-50: `rm cp mv mkdir touch chmod chown cat` + cd-family; `CMD_FILES` for cmd.exe; PowerShell verbs) have their path arguments resolved (`argPath`: unquote, `~`/`$HOME`/PowerShell `$env:` expansion, glob-prefix truncation via `prefix()`, `provider()` for `C:`/`Filesystem::` drives, `cygpath` for posix shells on Windows). Anything resolving outside the instance lands in `scan.dirs` → an `external_directory` ask with `<dir>/*` globs (263-280).
- Every non-`cd` command contributes its full source text (including redirections, via `source()` lines 119-121) to `scan.patterns`, and its arity prefix to `scan.always` (407-410). The `bash` permission ask therefore carries **each sub-command as a separate pattern** — `git status && rm -rf x` requires both to be allowed (the engine evaluates every pattern; one ask → deny aborts, `permission/index.ts:72-82`). A working directory outside the project also triggers `external_directory` (line 626).
- **No dangerous-command blocklist exists.** Grep for banned/forbidden/rm -rf across `src/` finds only CLI flag descriptions. Safety is entirely user-config (`"rm *": "deny"`) plus the ask flow. Known evasion surface (static analysis only): `dynamic()` (174-179) refuses to resolve args containing `$(`, backticks, or `$VAR`, and redirect *targets* are excluded from path scanning (line 99) — `echo x > /outside/file` is gated only by the whole-command `bash` pattern, not `external_directory`.

---

## 4. External-Directory Protection (workspace boundary)

- Boundary test: `containsPath` (`project/instance-context.ts:18-23`) — a path is inside if under `ctx.directory` **or** `ctx.worktree` (git worktree root); the worktree check is skipped when worktree is `/` (non-git projects) so external-dir prompts still fire.
- Enforcement helper: `tool/external-directory.ts:15-45` — `assertExternalDirectoryEffect(ctx, target, {bypass, kind})`. If outside, asks permission `external_directory` with glob `<parent-dir>/*` (both `patterns` and `always`). `bypass` is used by internal callers (e.g. read with `bypassCwdCheck`).
- Callers: `edit.ts:83`, `write.ts:44`, `apply_patch.ts:74,143`, `read.ts:250`, `glob.ts:44`, `grep.ts:55`, and the shell scanner (§3). Default policy (`agent/agent.ts:122-125`): `"*": "ask"` with allow-listed internal dirs (truncation store, tmp, skill dirs, reference dirs). Enforcement is **cooperative** — each tool opts in by calling the helper; there is no filesystem-level interception.
- "Sandbox" elsewhere in the codebase means **git worktree sandboxes** (`project/project.ts:90-99,309`), not OS isolation.

---

## 5. Per-Agent Permission Profiles

All agents share `defaults` (`agent/agent.ts:119-136`): `"*": "allow"`, `doom_loop: ask`, `external_directory: ask` (+ whitelists), `question: deny`, `plan_enter/plan_exit: deny`, and `read` with `*.env`/`*.env.*` → **ask**, `*.env.example` → allow. Then per-agent overlays (later = stronger), then user config, then session rules:

| Agent | Overlay (agent.ts) |
|---|---|
| `build` (primary) | `question: allow`, `plan_enter: allow` (141-155) |
| `plan` (primary) | `edit: deny` except `.opencode/plans/*.md` and the global plans dir; `task: {general: deny}`; `external_directory` allow for plans dir; `plan_exit: allow` (156-181) — **read-only planning with a carve-out for writing plan files** |
| `general` (subagent) | `todowrite: deny` (182-195) |
| `explore` (subagent) | `"*": "deny"` then allow `grep glob list bash webfetch websearch read` (196-218) — note `bash` is allowed, so "explore" is not strictly read-only at the OS level |
| `compaction`/`title`/`summary` (hidden) | `"*": "deny"` (219-264) |

Custom agents start from `merge(defaults, user)` and apply their config `permission` last (agent.ts:273-293). Every agent gets `Truncate.GLOB` external-directory allow unless explicitly denied (296-310).

**Subagent scoping** (`agent/subagent-permissions.ts:14-27`): the child session inherits from the *parent session* only `external_directory` rules and **all deny rules** ("parent agent restrictions only govern that agent; the subagent's own permissions determine its capabilities"), plus default denies for `todowrite` and `task` unless the subagent's own ruleset mentions them. `tool/task.ts:120-128` gates spawning behind `task` permission (pattern = subagent type), `subagent_depth` default 1 (line 111), and `task.ts:139-172` builds the child session's merged ruleset with dedup. Doom-loop guard: 3 identical tool calls (`DOOM_LOOP_THRESHOLD = 3`, `session/processor.ts:29,356-377`) trigger a `doom_loop` ask — an anomaly-detection circuit breaker.

---

## 6. What Is NOT Sandboxed — honest boundary assessment

- **No OS sandbox.** No seatbelt/sandbox-exec, bwrap, landlock, or seccomp anywhere in the repo (grep across `packages/**` returns nothing). Bash runs as the user with the user's full environment (`shellEnv` spreads `process.env`, `shell.ts:422-425`) — secrets in env vars are visible to every approved command.
- **Network is permission-only.** `webfetch` (per-URL pattern, `webfetch.ts:39-47`), `websearch` (per-query, `websearch.ts:119-125`) and MCP tools (`session/tools.ts:408`, pattern `"*"` per tool key) are gated by rules, but any approved bash command has unrestricted network/FS access. There is no egress filtering.
- **Shell path scanning is heuristic** (§3): dynamic args, redirect targets, env indirection, and unknown file-mutating binaries (`dd`, `tee`, `sed -i`, interpreters) bypass `external_directory` static checks; they are caught only by whole-command bash patterns.
- **Approved set is in-memory and per-instance** (`permission/index.ts:25,50`); "always" grants are not persisted and not shared across instances/workspaces.
- **Plugins run in-process** and wrap tool execution (`tool.execute.before/after`, `session/tools.ts:106-125`) outside the permission decision path.
- Doc drift: `permissions.mdx:174-186` claims `.env` reads are **denied** by default; the code says **ask** (`agent.ts:132-133`).
- The permission service trusts its callers: the model-facing tool list is filtered (`disabled`), but a compromised/misbehaving in-process component could invoke `Permission.Service.reply` directly. The boundary is the LLM↔user trust gap, not a process boundary.

---

## 7. Design Patterns (canonical names → code)

| Pattern | Where |
|---|---|
| **Policy-based authorization / ordered rule engine** (firewall-style, last-match-wins) | `permission/index.ts:28-38` (`evaluate`, `findLast`), config order preserved (`core/src/v1/config/permission.ts:14-16`) |
| **Request–reply over an event bus** (async rendezvous, correlation ID, Deferred/promise) | `permission/index.ts:98-106` + `109-167`; events `permission.asked/replied` (`schema/src/v1/permission.ts:61-66`) |
| **Defense in depth / layered policy cascade** | tool hiding (`visibleTools`, `index.ts:216-219`) → config defaults → agent overlay → user config → session rules (`session/tools.ts:87`) → runtime ask → in-memory `approved` grants |
| **Fail-closed / safe default** | unmatched → `ask` (`index.ts:33-36`); headless without `--auto` → reject (`cli/cmd/run.ts:805-815`); instance teardown rejects all pending (`index.ts:54-61`) |
| **Circuit breaker (anomaly detection)** | `doom_loop` after 3 identical calls (`session/processor.ts:29,356-377`) |
| **Strategy pattern** (per-shell parsers & arg extraction) | bash vs PowerShell/cmd branches (`tool/shell.ts:257-261, 188-218`) |
| **Policy inheritance / scoped delegation** | subagent ruleset derivation (`agent/subagent-permissions.ts:14-27`, `tool/task.ts:139-172`) |
| **Feedback loop / corrected rejection** (reject-with-message as model input) | `CorrectedError` (`core/src/v1/permission.ts:13-19`, `index.ts:121-127`) |
| **DI service layer (Effect)** | `Context.Service` + `Layer` (`permission/index.ts:40-42,221`) |
| **Audit via event sourcing (lite)** | all asks/replies published as versioned events on the bus (`event-v2-bridge.ts:35-60`) |

Summary: OpenCode implements a **cooperative, rule-engine-based authorization layer** with an elegant Deferred-over-event-bus approval rendezvous and genuinely granular (per-subcommand) bash permissions via tree-sitter — but it is **not a sandbox**: the boundary is policy + prompting, and approved code executes with full user privileges.
