## Chapter 5 — Model & Provider Abstraction, Prompt Assembly

Chapter 3 traced what the agent loop does with a provider turn; this chapter explains how OpenCode decides *which* provider, *which* model, and *which exact bytes* go on the wire — and how the system prompt the model sees is assembled. Every claim was verified against the dev-branch checkout (commit a19b52e8); the provider layer is one of the most actively migrated areas of the codebase, so gating flags and fallback behavior are cited explicitly wherever they apply.

### 5.1 Two LLM stacks behind one runtime seam

OpenCode ships **two complete provider stacks**. The production stack is built on the Vercel AI SDK v6: every provider is an npm package exposing a `LanguageModelV3` factory, driven from `packages/opencode/src/provider/` and invoked once per turn through `streamText` (Chapter 3). The second, `@opencode-ai/llm` (`packages/llm/`), is an in-house Effect-TS replacement whose central abstraction is a **Route** — an immutable composition of four orthogonal axes, documented at the canonical constructor: `Protocol` ("what is the API I'm speaking"), `Endpoint` ("where do I send the request"), `Auth` ("how do I authenticate it"), and `Framing` ("how do I cut the response stream into protocol frames") (`packages/llm/src/route/client.ts:306-320`).

Selection happens at exactly one seam. When `flags.experimentalNativeLlm` is set, `LLM.stream` offers the turn to the native runtime first (`packages/opencode/src/session/llm.ts:225-275`); the gate in `native-runtime.ts:55-65` accepts only the `openai`, `anthropic`, and `opencode*` providers carrying API-key (not OAuth) credentials, and otherwise returns a concrete `{type:"unsupported", reason}` — after which the session falls back to the AI-SDK path with the reason logged. Both paths converge on the same canonical `LLMEvent` stream, so the processor never learns which stack served the turn.

| Axis | AI SDK v6 path (production) | `@opencode-ai/llm` (native) |
|---|---|---|
| Status | Default runtime for every session | Opt-in via `experimentalNativeLlm`; automatic logged fallback |
| Location | `packages/opencode/src/provider/` | `packages/llm/` |
| Composition | `BUNDLED_PROVIDERS` — 24 lazily imported SDK packages — plus `custom()` quirk loaders (`provider.ts:107-134, 168`) | `Route.make` composing Protocol × Endpoint × Auth × Framing (`client.ts:303-339`) |
| Wire protocols | Whatever each vendor SDK implements | Hand-written protocols: `anthropic-messages`, `openai-chat`, `openai-responses`, `openai-compatible-chat`, `gemini`, `bedrock-converse` (+ event-stream framing), `packages/llm/src/protocols/` |
| Message adaptation | `ProviderTransform.message` middleware per request (`transform.ts:442-491`) | Canonical `LLMRequest` lowered by `Protocol.body.from`; shared lowering in `protocols/shared.ts` |
| Cache policy | `applyCaching`: first 2 system + last 2 messages, Anthropic family only (`transform.ts:335-384`) | `cache-policy.ts`: last tool + last system part + latest user message, at compile time (`:18-22`) |
| Auth | `auth.json` store + per-provider `custom()` loaders | Composable `Credential`/`Auth` combinators with `orElse` chains (`route/auth.ts`) |
| Used when | Always, unless the flag is on and the native gate accepts | Only `openai`/`anthropic`/`opencode*` with API-key auth (`native-runtime.ts:55-65`) |

The coexistence is a textbook strangler-fig migration, and the table shows the maturity gradient plainly. The legacy stack wins on breadth — two dozen vendors for free, because each AI SDK package absorbs a provider's wire format — at the cost of a thick normalization layer (`transform.ts`, §5.4) that exists precisely because OpenCode does not control those implementations. The native stack inverts the trade: it owns seven protocols end to end, which is what makes semantic cache policy (§5.5), composable credential chains (§5.6), and provider-agnostic structured output possible, but it pays for that control in hand-maintained wire code and a deliberately narrow support gate. Keeping both behind one `LLMEvent` algebra is what makes the migration reversible per request rather than per release.

### 5.2 The provider registry: lazy imports and a quirk layer

The AI-SDK registry, `BUNDLED_PROVIDERS` (`provider.ts:107-134`), maps 24 npm package names to thunks that `import()` the package on first use — startup cost is paid only for providers actually configured — and packages outside the map are installed at runtime from npm (`Npm.add`, `provider.ts:1776-1796`). Behavior is then specialized per provider by `custom()` loaders (`provider.ts:168+`), each returning `{autoload, getModel, vars, options, discoverModels}`. This is where vendor quirks live, and the survey reads like a field guide to API inconsistency:

- **anthropic** injects the beta headers `interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14` (`provider.ts:170-178`).
- **opencode** (the in-house "zen" gateway), when no key is configured, deletes every paid model from the catalog entry and falls back to `apiKey: "public"` — free-tier gating implemented as model-list surgery (`provider.ts:179-201`).
- **openai / xai** force the Responses API over Chat Completions via `sdk.responses(modelID)` (`provider.ts:202-224`); **github-copilot** chooses chat-vs-responses per model from a `gpt-5` regex (`provider.ts:225-239`).
- **azure** resolves the resource name from config → auth metadata → env (`provider.ts:240-293`); **amazon-bedrock** implements cross-region inference-profile prefixing (`us.`, `eu.`, `apac.`…) and the full AWS credential chain (`provider.ts:294-455`); **google-vertex** resolves project/location from six environment variables and authenticates with Application Default Credentials (`provider.ts:498-549`).

(Interpretation.) The architectural value is *concentration*: every workaround is one loader with a comment, not an `if` scattered through session code. A cloner should treat this file as institutional memory — each entry documents a provider bug or convention that will otherwise be rediscovered in production.

### 5.3 The model catalog: models.dev as the source of truth

Model metadata does not come from the providers at all. `ModelsDev.Service` (`packages/core/src/models-dev.ts:154`) fetches `https://models.dev/api.json` (overridable via `OPENCODE_MODELS_URL`), caches it as `models.json` under a file lock with a five-minute TTL, and maps each entry into OpenCode's `Model` record: `api.{id,url,npm}`, `capabilities.{temperature, reasoning, attachment, toolcall, modalities, interleaved}`, `cost` (including cache read/write pricing and context tiers), `limit.{context,input,output}`, `status`, and `release_date` (`provider.ts:1031-1045, 1207-1258`). models.dev `experimental.modes` expand into synthetic `<id>-<mode>` models.

Resolution is a layered merge — catalog → plugin `models` hooks → config `provider.*` overrides → environment API keys → stored auth → plugin auth loaders → the `custom()` loaders above — followed by filtering (`disabled_providers`, per-model blacklists, `alpha` gating on `enableExperimentalModels`, `deprecated` deletion) (`provider.ts:1338-1664`). A user can therefore override a model's npm package, base URL, cost table, limits, or capabilities from config alone. At request time, `parseModel` splits `provider/model` on the first `/` (`provider.ts:1992-1998`), `getModel` resolves with fuzzysort "did you mean" suggestions, and `getLanguage` caches one `LanguageModelV3` per provider/model pair (`provider.ts:1830-1859`). Default selection is its own fallback chain: configured model → most recently used (`state/model.json`) → first available from a hardcoded priority list (`["gpt-5","claude-sonnet-4","big-pickle","gemini-3-pro"]`, `provider.ts:1981-1990`), with a parallel `getSmallModel` family priority (`gemini-flash`, `gpt-nano`, `claude-haiku`) feeding cheap tasks such as title generation (Chapter 3).

### 5.4 Message transforms: the normalization gauntlet

Before any AI-SDK request leaves the process, `wrapLanguageModel` middleware runs `ProviderTransform.message` (`transform.ts:442-491`), a pipeline that rewrites the message list for the target vendor: lone UTF-16 surrogates are sanitized everywhere; Anthropic/Bedrock requests drop empty messages and unsigned reasoning parts (`:146-199`); Claude tool-call IDs are scrubbed to `[a-zA-Z0-9_-]` (`:201-228`); Mistral IDs are forced to 9-character alphanumerics, with a synthetic `assistant:"Done."` message spliced between a tool result and the next user message (`:230-278`); DeepSeek assistants must carry a reasoning part (`:280-296`); `reasoning_content`/`reasoning_details` from interleaved-thinking proxies are hoisted into `providerOptions.openaiCompatible` (`:298-330`); and Responses-API `itemId`s are stripped whenever `store !== true` so signed bodies stay immutable (`:474-488`). A sibling `options()` (`:1107-1275`) applies per-vendor defaults — `store:false` for the OpenAI family, `promptCacheKey = sessionID`, encrypted-reasoning includes for stateless GPT-5, `thinkingConfig.includeThoughts` for Google — and `variants()` (`:685+`) encodes reasoning-effort presets per model family, down to release-date-gated GPT-5 effort levels. This file is the price of the AI-SDK strategy: breadth bought from vendor SDKs must be repaid as a 1 787-line anti-corruption layer.

### 5.5 Prompt caching as a policy decision

Both stacks treat cache breakpoints as a *policy*, not an accident of message order. The native stack's default `"auto"` policy places three ephemeral breakpoints (`packages/llm/src/cache-policy.ts:18-22`):

```ts
// packages/llm/src/cache-policy.ts:18-22
const AUTO: CachePolicyObject = {
  tools: true,                      // breakpoint on the last tool definition
  system: true,                     // breakpoint on the last system part
  messages: "latest-user-message",  // breakpoint on the newest user message
}
```

The header comment states the reasoning (`cache-policy.ts:5-29`): within one turn the assistant/tool round-trips multiply while everything up to the latest user message stays fixed, so a breakpoint there makes every intra-turn call a prefix hit; and the economics need only one reuse — Anthropic's five-minute cache write costs 1.25× base, a read 0.1×. Hints are emitted only for protocols that honor them — `RESPECTS_INLINE_HINTS = {"anthropic-messages","bedrock-converse"}` (`:42`) — because OpenAI and Gemini caching is implicit; the protocols lower hints to vendor blocks (`cache_control:{type:"ephemeral"}`, Bedrock `CachePointBlock`s). The AI-SDK path's equivalent, `applyCaching` (`transform.ts:335-384`), is a coarser positional heuristic: mark the **first two system messages and the last two non-system messages**, Anthropic-family only, with per-SDK option keys (`anthropic.cacheControl`, `bedrock.cachePoint`, `copilot.copilot_cache_control`, …). The contrast is diagnostic of the whole migration: the legacy code encodes *positions*; the new code encodes the *semantics* of a tool-use loop's stable prefix boundary.

### 5.6 Authentication: three credential shapes, OAuth as a plugin

Credentials persist in `auth.json` under the data directory, always written with mode `0o600` (`packages/opencode/src/auth/index.ts:79`), as a discriminated union `Oauth{refresh,access,expires,…} | Api{key,metadata} | WellKnown{key,token}` (`auth/index.ts:14-34`); the `OPENCODE_AUTH_CONTENT` environment variable can inject the entire store for headless use. OAuth itself is not core code: `ProviderAuth.Service` (`provider/auth.ts`) obtains its *methods* from plugin `auth` hooks and orchestrates `authorize`/`callback` over them — GitHub Copilot's device-code flow and OpenAI Codex's PKCE-plus-localhost-callback flow are both bundled plugins (`plugin/github-copilot/copilot.ts:222-300`, `plugin/openai/codex.ts:80-230`; Chapter 8 covers the plugin surface). In the native stack, auth is a composable value instead: Anthropic's route is `Auth.optional(apiKey).orElse(Auth.config("ANTHROPIC_API_KEY")).pipe(Auth.header("x-api-key"))` (`packages/llm/src/providers/anthropic.ts:13-18`) — a chain of responsibility that falls through explicit credential sources and redacts secrets from error payloads.

### 5.7 Agent definitions: personas as data

Agents are configuration, not code. `Agent.Info` (`packages/opencode/src/agent/agent.ts:35-55`) bundles a mode (`primary | subagent | all`), optional `model`/`variant`/`temperature`/`topP`, a `prompt` override, a `steps` cap, and a permission ruleset. Seven agents are built in (`agent.ts:140-265`):

| Agent | Mode | Visible | Permission profile | Prompt / tuning |
|---|---|---|---|---|
| `build` | primary | yes (default) | base defaults; `question`/`plan_enter` allowed | family prompt via `SystemPrompt.provider` |
| `plan` | primary | yes | all edits denied except plan files; `task.general` denied; `plan_exit` allowed | family prompt + plan-mode reminders |
| `general` | subagent | yes | `todowrite` denied | default prompt |
| `explore` | subagent | yes | all denied except read/grep/glob/list/bash/web | own `agent/prompt/explore.txt` |
| `compaction` | primary | hidden | `"*": deny` | own `compaction.txt` (anchored summary, §3.6) |
| `title` | primary | hidden | `"*": deny` | own `title.txt`, `temperature: 0.5` |
| `summary` | primary | hidden | `"*": deny` | own `summary.txt` |

The hidden trio deserves attention: session maintenance functions — compacting history, titling sessions, summarizing — are expressed as *agents* so they ride the same model resolution, retry, and permission machinery as user-facing work, with all-deny rulesets that make them structurally incapable of touching tools. Base defaults allow everything but gate `doom_loop`/`question`/`plan_enter`/`plan_exit` and ask before reading `*.env` (`agent.ts:119-136`). Two extension paths exist: config `agent.*` entries merge into, override, or `disable` natives (`agent.ts:267-294`), and markdown files under `.opencode/{agent,agents}/**/*.md` become agents whose frontmatter is config and whose body is the system prompt (`config/agent.ts:11-32`). Subagent spawning applies capability attenuation: the child inherits the parent's `deny` and `external_directory` rules, and `todowrite`/`task` are denied unless the subagent's own ruleset permits them (`subagent-permissions.ts:14-27`) — so a child can never be more powerful than its parent.

### 5.8 System prompt assembly: cache-stable by construction

The system prompt is rebuilt per step and finalized in `LLMRequestPrep.prepare` (`session/llm/request.ts:56-78`):

```ts
// packages/opencode/src/session/llm/request.ts:58-66 (trimmed)
const system = [
  [
    ...(input.agent.prompt ? [input.agent.prompt] : SystemPrompt.provider(input.model)),
    ...input.system,
    ...(input.user.system ? [input.user.system] : []),
  ].filter((x) => x).join("\n"),
]
```

The base layer is either the agent's prompt override or a **model-family prompt**: `SystemPrompt.provider` (`session/system.ts:27-42`) substring-matches the model id — `claude`→`anthropic.txt`, `gpt`→`gpt.txt` (with `codex` and `gpt-4`/`o1`/`o3` special cases), `gemini-`, `kimi`, `trinity`, `muse-spark`→`meta.txt`, else `default.txt`. Fourteen prompt text files ship in `session/prompt/`; the remainder are plan-mode and build-switch reminders injected on mode transitions (Chapter 3). On top of the base come, in order: the `<env>` block — working directory, workspace root, git-repo flag, platform, date (`system.ts:60-76`); instruction files — global `AGENTS.md`/`CLAUDE.md`, the first project-level match found walking up to the worktree, `config.instructions` globs, and remote http(s) instruction URLs fetched with a five-second timeout (`session/instruction.ts:95-103, 155-169`); and permission-filtered `<mcp_instructions>` plus the skill catalog (`system.ts:98-128`, detailed in Chapter 8). Two rules protect the cache prefix: after the plugin hook `experimental.chat.system.transform` runs, if the header is unchanged the tail is re-joined so the base prompt remains the first system message verbatim (`request.ts:68-78`); and on the OpenAI-OAuth path the entire system block moves to `providerOptions.instructions` instead of system messages (`request.ts:99-112`), because that API expects instructions as a parameter, not a message.

### Architect's take

*(Interpretation.)* A cloner should copy three decisions wholesale: externalize model metadata into a fetchable catalog with a config-merge override chain instead of hardcoding per-vendor model lists; concentrate every vendor quirk in one transform/loader layer with a comment per workaround, because that file becomes your institutional memory of provider bugs; and place cache breakpoints deliberately at the latest-user-message boundary — the highest-leverage cost optimization in a tool-use loop. The dual-stack seam is worth studying but not cloning on day one: start behind a single canonical event algebra and a `Protocol`-shaped interface, and let a second runtime earn its place. Per-agent model/prompt/permission overrides, finally, cost almost nothing — they are data — and they are exactly what make plan mode, read-only explore agents, and hidden maintenance agents trivial to express.
