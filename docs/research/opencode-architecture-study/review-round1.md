# Review Round 1 — OpenCode Architecture Study (sec02–sec10)

**Verdict: PASS** — no fabricated or claim-unsupported citations found in ~40 spot-checks against the dev @ a19b52e85bf2 checkout (package version 1.18.3 confirmed in `packages/opencode/package.json`). The load-bearing claims all hold. The issues below are one internal counting error, one cross-chapter numeric inconsistency, one wrong file count, and a set of trivial line-drift/style items. None changes an architectural conclusion.

Scope: sec02–sec10 (sec01 excluded, being written concurrently). Repo checkout: `/mnt/agents/output/opencode-study/opencode-dev`.

---

## Issues (ordered by severity)

### 1. sec09 §9.8 — wrong count: "guardrails (nine mappings)", its own table has ten
- **File/location:** `opencode-architecture-study_sec09.md`, line 72 (histogram paragraph after the master table).
- **Problem:** The master table (lines 34–70) contains **10** rows mapped to Guardrails: Doom-loop guard, Strategy (replacer cascade), Decorator, Runner FSM, Abort cascade, Memento (shadow-git), Retry schedule, LSP broken-set, Fail-closed defaults, Capability-based filtering. The prose says nine. Verified by counting `Guardrails` occurrences in the table (10). The companion count "memory/context management (six)" is correct.
- **Fix:** Change "nine mappings" → "ten mappings".

### 2. Cross-chapter contradiction: bundled provider count (24 vs ~26)
- **File/location:** `sec05.md` lines 15 & 26 say "**24** lazily imported SDK packages"; `sec09.md` line 29 says "~26 providers"; `sec10.md` line 58 says "~26 vendors". (Concurrently-written sec01 also says ~26.)
- **Problem:** Verified `BUNDLED_PROVIDERS` at `packages/opencode/src/provider/provider.ts:107-134` has exactly **24 keys** (counting the two multi-line entries `@ai-sdk/google-vertex/anthropic` and `@ai-sdk/github-copilot`). sec05 is precise and correct; sec09/sec10 contradict it. Note 24 keys ≈ 21 distinct npm packages (subpath keys for bedrock/mantle and vertex/anthropic share packages), so "~26" is not defensible on either basis.
- **Fix:** Normalize everywhere to "24 bundled provider SDK packages" (or "two dozen"). If the "~26" intends to include runtime-installed providers (`Npm.add`, provider.ts:1776-1796), say so explicitly.

### 3. sec04 §4.7 — wrong file count: "14 files, 235 lines" should be 15 files
- **File/location:** `opencode-architecture-study_sec04.md`, line 166.
- **Problem:** `packages/opencode/src/tool/*.txt` contains **15** files (apply_patch, edit, glob, grep, lsp, plan-enter, plan-exit, question, read, skill, task, todowrite, webfetch, websearch, write) totaling exactly 235 lines. The line count is right; the file count is off by one.
- **Fix:** Change to "15 files, 235 lines" (or state the exclusion, e.g. if plan-enter/plan-exit were not meant to count).

### 4. sec09 §9.8 — quantitative overstatement: "roughly a third of the catalog"
- **File/location:** `opencode-architecture-study_sec09.md`, line 72.
- **Problem:** Enabler rows ("— (enabler)") number **9 of 35 = 26%** — closer to a quarter than a third.
- **Fix:** "roughly a quarter" (and "carries the agent-specific three-quarters" if the reciprocal phrase is kept).

### 5. sec10 — module-count mismatch: Figure 6 shows 12 modules, §10.2 table lists 13 rows
- **File/location:** `sec10.md` lines 7–11 (Figure 6 + caption "twelve numbered modules") vs the §10.2 table (lines 19–33, 13 rows).
- **Problem:** The d6 diagram (inspected) shows 12 numbered boxes, folding the six core tools into box 3 "Tool contract + registry"; the §10.2 table gives "Six core tools" its own row, yielding 13. The caption accurately describes the figure, but a reader cross-referencing figure ↔ table will count differently.
- **Fix:** Add a half-sentence in §10.2 noting the table splits figure-module 3 into contract + tools (13 rows vs 12 boxes), or renumber.

### 6. Chapter-title style inconsistency
- **File/location:** H2 of sec02 ("## 2. System Architecture…"), sec05 ("## 5. Model & Provider…"), sec08 ("## 8. Extensibility…") use `## N. Title`; sec03/sec04/sec06/sec07/sec09/sec10 use `## Chapter N — Title`.
- **Problem:** Mixed heading convention across the same report.
- **Fix:** Unify (the `## Chapter N — Title` form is the majority).

### 7. Trivial citation drift (normalization pass; none reverses a claim)
- sec02 §2.5: `EventV2 (packages/core/src/event.ts:150)` — the `Service` class is at **:148** (`:150` is `allBounded`).
- `commitDurableEvent` range cited as `event.ts:205-366` (sec03 §3.1) vs `event.ts:205-352` (sec07 §7.3); the function starts at :205 and ends ≈:367. Pick one range.
- TUI worker bridge cited as `tui.ts:25-48` (sec02 §2.2) vs `tui.ts:24-48` (sec07 §7.7); `createWorkerFetch` starts at :24.
- `Agent.Info` cited as `agent.ts:35-56` (sec03 §3.7) vs `agent.ts:35-55` (sec05 §5.7).
- sec08 hooks table off-by-ones: `shell.env` cited `prompt.ts:554` (actual :555); `experimental.text.complete` cited `processor.ts:516` (actual :517); `experimental.session.compacting`/`autocontinue` cited `compaction.ts:343,454` (actual :344,:455).
- sec10 §10.5 cites `compaction.ts:32-34,83` for the clamp constants; the constants live at :30,:33-34 and the clamp at :82-83 (`:32` is `DEFAULT_TAIL_TURNS`). Harmless but imprecise.

### 8. Informational — length overruns vs outline budgets
- sec03 (~2,830 words vs ~1,900 target), sec04 (~3,030 vs ~1,900), sec10 (~2,700 vs ~2,000), sec05 (~2,140 vs ~1,600). No action required for correctness; flag if the assembled book enforces budgets.

---

## Verified-OK key claims (spot-checked against the repo)

Agent loop (sec03):
- `prompt.ts:1088` — `while (true)` loop at exactly that line; `runLoop` at :1081; entry `SessionPrompt.prompt` at :1052 with `revert.cleanup` at :1056; exit-test comment at :1103-1109; `maxSteps = agent.steps ?? Infinity` at :1178. ✓
- Descending ULID session ids (`packages/schema/src/identifier.ts:10,14,22`; `session-id.ts:8`). ✓
- 12 part variants (`v1/session.ts:357-370`) and 8-class assistant error union (:385-394). ✓
- Doom-loop guard: `DOOM_LOOP_THRESHOLD = 3` at `processor.ts:29`, check at :356-380; identical-input comparison confirmed. ✓
- Retry schedule: `retry.ts:26-66` — 2000·2^(attempt−1), 30 s no-header cap, 2³¹−1 ms header cap; `policy` at :176-199. Transport retry: `executor.ts` MAX_RETRIES=2 (:36), 500·2^a ±20% jitter, 10 s cap (:37-38), statuses 429/503/504/529 (:91). ✓
- Runner FSM four states at `runner.ts:33-37`; abort cascade via AbortController at `llm.ts:361-364`. ✓
- Compaction: `DEFAULT_TAIL_TURNS = 2` (:32), `clamp(usable·0.25, 2_000, 8_000)` (:33-34,:82-83), `PRUNE_MINIMUM = 20_000` (:28), `PRUNE_PROTECT = 40_000` (:29), skill protected (:31), `COMPACTION_BUFFER = 20_000` (`overflow.ts:8`); `select` at :188, `prune` at :243. ✓
- Processor 5-line stream core at `processor.ts:640-646`; `toLLMEvents` at `llm/ai-sdk.ts:76`. ✓

Tools (sec04):
- `Tool.define` at `tool/tool.ts:151-169`; `Def` interface at :55-65; nine replacers in stated order at `edit.ts:694-704`; `SINGLE_CANDIDATE_SIMILARITY_THRESHOLD = 0.65` at :220; block-anchor quote matches :307-321; per-file semaphore map at :35-45; cline/gemini-cli credit header at :1-4. ✓
- Truncation: `MAX_LINES = 2000`, `MAX_BYTES = 50*1024` at `truncate.ts:15-16`; read limits at `read.ts:13-17` (2000 lines / 50 KB / 2000 chars-per-line); glob/grep 100-result caps (`glob.ts:48`, `grep.ts:67`). ✓
- Built-in registry list + 3 flag-gated tools at `registry.ts:226-243`; edit/apply_patch mutex `usePatch` at :292-295; websearch gating at :288-290. ✓
- Negative claim verified: no `markRead|hasRead|lastRead` enforcement in code; `edit.txt:4` / `write.txt:5` do promise it (docs-vs-code gap claim is correct). ✓

Providers/prompt (sec05):
- `BUNDLED_PROVIDERS` = 24 keys at `provider.ts:107-134`; default-model priority `["gpt-5","claude-sonnet-4","big-pickle","gemini-3-pro"]` at :1981; small-model priority matches. ✓
- Cache policy: `AUTO = {tools, system, messages:"latest-user-message"}` at `cache-policy.ts:18-22`; `RESPECTS_INLINE_HINTS` at :42; 1.25×/0.1× economics in header comment. Legacy `applyCaching` (first 2 system + last 2 non-system) at `transform.ts:335-384`, gated to the Anthropic family at :445-456; transform.ts is exactly 1,787 lines. ✓
- Native gate at `native-runtime.ts:55-65` (openai/anthropic/opencode*, package check, OAuth-with-fetch exception, API key required); flag at `runtime-flags.ts:54`. Seven protocol files under `packages/llm/src/protocols/` (6 API protocols + bedrock-event-stream framing). ✓
- System-prompt assembly quote matches `request.ts:58-66`; header-unchanged rejoin at :74-78; family prompts at `system.ts:27-42`; 14 prompt txt files in `session/prompt/`; remote-instruction fetch with 5 s timeout at `instruction.ts:95-103` (`Effect.timeout(5000)`). ✓
- Agent defaults (`agent.ts:119-136`): `"*": allow`, `doom_loop/question/plan_enter/plan_exit` gating, `*.env` ask + `*.env.example` allow — exact match. ✓

Permissions (sec06):
- 13-line `evaluate` with `findLast` + default `ask` at `permission/index.ts:28-38`; `merge` = flat at :200-202; `disabled` aliasing edit|write|apply_patch→"edit" at :204-219; teardown rejects pending at :54-61; Deferred ask with no timeout at :97-106. ✓
- Wildcard dialect incl. `" *"` → `( .*)?` at `wildcard.ts:3-14`; arity `git: 2` (:83), `"npm run": 3` (:114), generation-prompt comment present. ✓
- Headless behavior: auto→reply "once", else auto-reject at `run.ts:796-816`. ✓
- Negative claims verified: no seccomp/seatbelt/sandbox-exec/bwrap/landlock anywhere in `packages/`; docs-drift claim is real — `permissions.mdx:174-183` says `.env` "denied" while `agent.ts:131-136` says `ask`. ✓

Server/events/storage (sec02, sec07):
- No Hono: case-insensitive repo-wide search over `packages/opencode/src`, `packages/server/src`, `packages/protocol/src`, `packages/core/src` finds zero Hono imports; server is `HttpRouter.serve(HttpApiApp…)` at `server.ts:100-115`; port 4096-then-free at :117-122; `Server.Default` in-memory fetch handler at :56-65. ✓
- Event sourcing: `commitDurableEvent` at `event.ts:205+` with `immediate` transaction, projector loop quote matching :316-323, `InvalidDurableEventError`; durable declaration `durable: { aggregate: "sessionID", version: 1 }` at `v1/session.ts:502-507`; `message.part.delta` non-durable at :632-641; EventV2 interface at :126-148. ✓
- API shape: 15 instance groups (`httpapi/api.ts:54-94`: Config…Workspace), 18 protocol groups (`protocol/src/api.ts:37-64`); `projectors.ts` is an empty stub (`export function initProjectors() {}`); storage root at `storage.ts:224`; `workspace-routing.ts:22-27` directory/workspace query fields; `server.connected` first frame (`handlers/event.ts:70`); SDK build outputs to `src/v2/gen` — `sdk.gen.ts` = 7,219 lines, `types.gen.ts` = 13,618 lines (exact match to sec07's claim). ✓
- Multi-project: `InstanceStore` Deferred-join boot at `instance-store.ts:108-123`; headless synthetic origin `http://opencode.internal` at `run.ts:952`; `containsPath` incl. worktree-`/` skip at `instance-context.ts:18-26`. ✓
- `CONTEXT.md` = exactly 225 lines. ✓

Extensibility (sec08):
- Dead `permission.ask` plugin hook: declared at `packages/plugin/src/index.ts:261`, zero trigger sites found. ✓
- Hook trigger sites spot-checked (chat.message `prompt.ts:1000`; chat.params `request.ts:115`; chat.headers `:135`; system.transform `:70`; tool.execute.before/after `tools.ts:107/122`; tool.definition `registry.ts:313`; command.execute.before `prompt.ts:1461`; messages.transform `:1255`; small_model `provider.ts:1887`) — all accurate within ±1 line. ✓
- MCP namespacing `sanitize(clientName)+"_"+sanitize(toolName)` at `catalog.ts:117-119`; shadow-git `--git-dir/--work-tree` at `snapshot/index.ts:71-75` (807 lines total, "~800" ✓); snapshot capture at `processor.ts:102,425,436`. ✓
- LSP diagnostics-in-edit-output at `edit.ts:196-201` region. ✓

Cross-reference & structure:
- Figures 1–6 all present, sequential, with existing PNGs (d1–d6 verified on disk; d6 inspected — 12 modules, "~40% of the core's value is the loop + tools + prompt" present as sec10 §10.7 claims). Figure 8.1 (mermaid) is an extra chapter-local figure; acceptable but note the two numbering schemes coexist.
- "Architect's take" section present in all 9 chapters (sec02–sec10). ✓
- All chapter cross-references (Chapter 2–10, §2.2–§6.1, Section 7.7, Section 10.7) resolve to existing chapters/sections. ✓
- No reference/bibliography sections at chapter ends; no TODO/TBD/placeholder text in any chapter. ✓
- Tool-parameter schema story consistent: Effect Schema for internal tools (sec04), zod only for plugin tools (sec04/sec09); no chapter claims zod for built-ins. Effect HttpApi (not Hono) stated consistently in sec02 and sec07. ✓

---

## Review gaps / notes
- The checkout has no `.git` directory, so the commit hash a19b52e85bf2 could not be re-verified via git; version 1.18.3 is confirmed and every checked citation matches the tree.
- Diagrams d1–d5 were not visually inspected (existence verified); only d6 was cross-checked against text.
- sec01 was excluded per instructions; note it currently also says "~26 bundled providers" (issue #2 applies there too).
- External quotes (opencode.ai docs, README mirror) were not re-fetched; they are attributed and non-load-bearing.
