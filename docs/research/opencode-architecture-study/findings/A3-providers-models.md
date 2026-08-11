# A3 — Provider/Model Abstraction, Prompt Assembly & Agent Definitions

Repo: `anomalyco/opencode` dev @ a19b52e85bf2. All paths relative to repo root.
Key structural fact: **two provider stacks coexist**:

1. **Production stack** — Vercel AI SDK v6 (`LanguageModelV3`), driven by the models.dev catalog, in `packages/opencode/src/provider/`.
2. **New native stack** — `@opencode-ai/llm` (Effect-TS), a four-axis Route composition in `packages/llm/`, currently opt-in (`flags.experimentalNativeLlm`) with automatic fallback to the AI-SDK runtime (`packages/opencode/src/session/llm.ts:225-275`).

---

## 1. Provider registry — unifying ~26 providers

### 1a. AI-SDK stack (`packages/opencode/src/provider/provider.ts`)

`BUNDLED_PROVIDERS` (provider.ts:107-134) is a registry of ~26 lazily-`import()`ed AI SDK provider packages: `@ai-sdk/anthropic`, `@ai-sdk/openai`, `@ai-sdk/azure`, `@ai-sdk/google`, `@ai-sdk/google-vertex(/anthropic)`, `@ai-sdk/amazon-bedrock(/mantle)`, `@openrouter/ai-sdk-provider`, `@ai-sdk/xai|mistral|groq|deepinfra|cerebras|cohere|gateway|togetherai|perplexity|vercel|alibaba`, `@ai-sdk/openai-compatible`, `gitlab-ai-provider`, `venice-ai-sdk-provider`, and `@ai-sdk/github-copilot` (mapped to an internal opencode-compatible provider, line 131-133). Unknown npm packages are installed at runtime via `Npm.add()` (provider.ts:1776-1796).

On top of the bundle map, `custom(dep)` (provider.ts:168+) registers per-provider `CustomLoader`s returning `{autoload, getModel, vars, options, discoverModels}` — this is where provider quirks live:

- **anthropic** (170-178): injects beta headers `interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14`.
- **opencode (zen)** (179-201): if no key/auth configured, strips all paid models (`cost.input !== 0`) and falls back to `apiKey: "public"` — i.e. free tier gating.
- **openai / meta / xai** (202-224): force `sdk.responses(modelID)` (Responses API instead of Chat Completions); openai gets `headerTimeout: 300_000`.
- **github-copilot** (225-239): picks `responses` vs `chat` endpoint from `model.api.endpoint` or regex `/^gpt-(\d+)/` ≥ 5 (except `gpt-5-mini`).
- **azure / azure-cognitive-services** (240-293): resolves `AZURE_RESOURCE_NAME` from config → auth metadata → env; builds cognitive-services baseURL.
- **amazon-bedrock** (294-455): full cross-region inference-profile prefixing (`us.`, `eu.`, `apac.`, `au.`, `jp.`, `global.`) by region × model family; AWS credential chain (`fromNodeProviderChain`) vs `AWS_BEARER_TOKEN_BEDROCK` precedence; region precedence config > env > `us-east-1`.
- **google-vertex** (498-549): project/location resolution from 6 env vars, ADC token via `google-auth-library` custom `fetch`; **google-vertex-anthropic** (550-569) builds `aiplatform.{location}.rep.googleapis.com` base URLs (94-99).
- **openrouter / llmgateway / vercel / nvidia** (456-497): attribution headers (`HTTP-Referer: https://opencode.ai/`, `X-Title`).
- **gitlab** uses `discoverModels` for deployment discovery (1592-1604).

### 1b. Native stack (`packages/llm`)

The canonical constructor `Route.make` (`packages/llm/src/route/client.ts:303-339`) composes **four orthogonal axes** — `Protocol` (API contract: body schema + builder + stream state machine), `Endpoint` (where), `Auth` (how), `Framing` (byte→frame cutting) — documented at client.ts:306-320. `route.with(patch)` (client.ts:257-268) is an immutable decorator merging defaults (`mergeRouteDefaults`, 105-120). `route.model({id})` produces a `Model` bound to that route (93-103).

- **Protocols** (`packages/llm/src/protocols/`): `anthropic-messages.ts`, `bedrock-converse.ts` (+ `bedrock-event-stream.ts` binary framing), `gemini.ts`, `openai-chat.ts`, `openai-compatible-chat.ts`, `openai-responses.ts` (+ WebSocket route), with shared lowering in `protocols/shared.ts` and `protocols/utils/` (cache, lifecycle, tool-schema, tool-stream). The `Protocol` interface (`route/protocol.ts:38-60`) owns `body.schema`/`body.from` and `stream.initial/step/terminal` — explicitly designed so "DeepSeek, TogetherAI, Cerebras… reuse `OpenAIChat.protocol` without forking 300 lines per provider" (protocol.ts:20-23).
- **Provider facades** (`packages/llm/src/providers/`): anthropic, amazon-bedrock, azure, cloudflare (`CloudflareAIGateway`/`CloudflareWorkersAI`), github-copilot (dual chat+responses routes, `shouldUseResponsesApi` gpt≥5 regex, github-copilot.ts:23-30), google, openai (`model` defaults to Responses; `chat` and `responsesWebSocket` alternates, openai.ts:38-52), openai-compatible (+profiles, e.g. OpenRouter profile), openrouter (reuses `OpenAIChat.bodyFields` and merges `providerOptions.openrouter` into the body, openrouter.ts:36-52), xai.

### 1c. transform.ts — quirk normalization (AI-SDK stack)

`ProviderTransform.message` (transform.ts:442-491) pipeline: `unsupportedParts` → `normalizeMessages` → `applyCaching` (Anthropic-family only) → providerOptions key remap → Responses `itemId` stripping. Normalized quirks:

- **Lone UTF-16 surrogates** sanitized everywhere (25-27).
- **Anthropic/Bedrock**: empty messages/parts dropped; reasoning parts kept only with `signature`/`redactedData` (146-199).
- **Claude tool-call IDs** scrubbed to `[a-zA-Z0-9_-]` (201-228); **Mistral** IDs forced to 9-char alphanumeric, plus synthetic `assistant:"Done."` message inserted between `tool`→`user` (230-278).
- **DeepSeek**: every assistant message must carry a reasoning part (280-296).
- **Interleaved reasoning** (`reasoning_content`/`reasoning_details`) hoisted from content parts into `providerOptions.openaiCompatible[field]` (298-330).
- `sdkKey()` (42-74) maps npm package → providerOptions key (`@ai-sdk/anthropic`→`anthropic`, `@openrouter/ai-sdk-provider`→`openrouter`, …) and stored options are remapped (459-472); Azure gets both `openai` and `azure` keys (1362-1364).
- `store !== true` ⇒ strip Responses `itemId`s to keep signed bodies immutable (474-488).
- `options()` (1107-1275): per-provider defaults — `store:false` for OpenAI-family; `promptCacheKey = sessionID` (Azure/OpenAI/xAI/Venice; OpenRouter `prompt_cache_key`); `include: ["reasoning.encrypted_content"]` for stateless GPT-5 reasoning; `thinkingConfig.includeThoughts` for Google; `enable_thinking` for alibaba-cn; zhipu `thinking:{enabled, clear_thinking:false}`; `toolStreaming:false` for non-Claude models on the anthropic SDK.
- `variants()` (685+): reasoning-effort/budget **option presets per model family and npm package** — GPT-5 version regexes (552-555) with release-date-gated `none` (≥2025-11-13) / `xhigh` (≥2025-12-04) efforts; Anthropic adaptive efforts for opus≥4.7/sonnet≥5 (`["low","medium","high","xhigh","max"]`, 628-640); Google `thinkingLevel`/`thinkingBudget` (646-683); SAP wrapped in `modelParams` (663-665); Kimi adaptive `thinking` with `display:"summarized"` (723-730).
- `schema()` (1462+): OpenAI tool-schema lowering (boolean schemas → `{type:"string"}`, `const`→`enum`, 1379+), plus Gemini and Moonshot sanitizers.
- Per-model `temperature/topP/topK` heuristics (493-529: qwen 0.55, gemini 1.0/topK 64, kimi-k2 0.6/1.0, minimax-m2…).

---

## 2. Model resolution, capabilities & limits

- **ID parsing**: `parseModel` (provider.ts:1992-1998) splits `provider/model` on the first `/`.
- **Catalog**: `ModelsDev.Service` (`packages/core/src/models-dev.ts:154-170`) fetches `https://models.dev/api.json` (override `OPENCODE_MODELS_URL`), cached to `models.json` with flock + scheduled refetch. `fromModelsDevModel` (provider.ts:1207-1258) maps each entry to `Model` (schema at 1031-1045): `api.{id,url,npm}` (default npm `@ai-sdk/openai-compatible`), `capabilities.{temperature, reasoning, attachment, toolcall, input/output modalities, interleaved}`, `cost` (incl. cache read/write, tiers, `experimentalOver200K`, 1174-1205), `limit.{context,input,output}`, `status`, `release_date`, plus derived reasoning `variants`. models.dev `experimental.modes` become synthetic `<id>-<mode>` models (1264-1275).
- **Merge order** (layer init, provider.ts:1338-1664): models.dev catalog → plugin `models` hooks → config `provider.*` entries (deep merge; config can override npm/url/cost/limits/capabilities, 1420-1515) → env API keys (`source:"env"`) → stored auth (`source:"api"`) → plugin auth loaders → `custom()` loaders → config re-apply. Then filtering: `disabled_providers`/`enabled_providers`, per-model `blacklist`/`whitelist`, `alpha` (gated on `runtimeFlags.enableExperimentalModels`) and `deprecated` deletion, invalid gpt-5 chat aliases (1606-1653).
- **Runtime resolution**: `getModel` (1806-1828) with fuzzysort "Did you mean" suggestions (1298-1325); `getLanguage` (1830-1859) caches `LanguageModelV3` per `providerID/modelID`; SDK factory instances cached by `Hash.fast({providerID, npm, options})` (1722-1730). `resolveSDK` (1668-1800) interpolates `${VAR}` in baseURL from vars loaders + env, wraps `fetch` with `chunkTimeout` SSE watchdog (`wrapSSE`, 37-83) and `headerTimeout` (85-92).
- **Default/small/fallback**: `defaultModel()` (1942-1975): `cfg.model` → most-recent from `state/model.json` → first provider sorted by priority `["gpt-5","claude-sonnet-4","big-pickle","gemini-3-pro"]` (1981-1990). `getSmallModel()` (1873-1940): `cfg.small_model` → plugin hook `experimental.provider.small_model` → family priority `["gemini-flash","gpt-nano","claude-haiku"]` (opencode→`gpt-nano`; copilot→`gpt-mini` first; Bedrock prefers `global.` then region-prefixed). `closest()` (1861-1871) substring-matches query terms.
- **Native routing**: `native-request.ts:160-178` maps `model.api.npm` → llm provider facade (`@ai-sdk/openai`→`OpenAI.configure().responses()`, anthropic, azure, google, bedrock, openai-compatible, openrouter); `native-runtime.ts:44-62` gates to openai/anthropic/opencode providers with API-key auth, otherwise returns `{type:"unsupported", reason}` → fallback to ai-sdk (session/llm.ts:253-267). Request pipeline `compile` (client.ts:344-359): 3-level options merge (route→model→request, 167-180) → `applyCachePolicy` → `body.from` → schema validation → `prepareTransport`. Retries in `route/executor.ts`: `MAX_RETRIES=2`, 500 ms base, 10 s cap, ±20 % jitter (345-351), retryable statuses 429/503/504/529 (91), `Retry-After` honored, and aggressive credential redaction in error payloads (46-84).

---

## 3. Auth

- **Credential store** (`packages/opencode/src/auth/index.ts`): `auth.json` under `Global.Path.data`, written `0o600`; `OPENCODE_AUTH_CONTENT` env can inject the whole store. `Info = Oauth{refresh,access,expires,accountId?,enterpriseUrl?} | Api{key,metadata?} | WellKnown{key,token}` (discriminated union).
- **Flow orchestration** (`packages/opencode/src/provider/auth.ts`): `ProviderAuth.Service` exposes `methods/authorize/callback`. Methods come from **plugin `auth` hooks**; `authorize` (163-186) runs prompt validation then stashes the pending `AuthOAuthResult`; `callback` (188-221) completes `"code"` (manual paste) or `"auto"` (device/localhost) flows and persists either `{type:"api"}` or `{type:"oauth"}` tokens.
- **OAuth implementations** (bundled plugins): GitHub Copilot **device-code flow** (`plugin/github-copilot/copilot.ts:222-300` — POST `/login/device/code`, show `verification_uri` + user_code, poll `/login/oauth/access_token`; supports GHE domains). OpenAI Codex **PKCE + localhost callback server** (`plugin/openai/codex.ts:80-230` — `ISSUER/oauth/authorize`, `OAUTH_PORT` server, refresh-token rotation at 126-150/356-369; restricts visible models when OAuth, 281-290). `request.ts:57,99` special-cases OpenAI-OAuth: system prompt moves to `providerOptions.instructions`, `store:false`.
- **Native stack auth** (`packages/llm/src/route/auth.ts`): composable `Credential`/`Auth` combinators — e.g. Anthropic: `Auth.optional(apiKey).orElse(Auth.config("ANTHROPIC_API_KEY")).pipe(Auth.header("x-api-key"))` (providers/anthropic.ts:13-18); OpenAI: `AuthOptions.bearer(options, "OPENAI_API_KEY")`; `Redacted` secrets, `MissingCredentialError` on absent env.

---

## 4. Prompt caching

`packages/llm/src/cache-policy.ts` — `applyCachePolicy` runs once at compile time (client.ts:345). Default `"auto"` (18-22) places **three ephemeral breakpoints**: last tool definition, last system part, and last text part of the latest user message — rationale (lines 5-14): the latest user message is the stable prefix boundary across a turn's assistant/tool round-trips; economics (27-29): Anthropic 5-min cache write 1.25× vs read 0.1× ⇒ one reuse wins. Only protocols honoring inline hints are marked — `RESPECTS_INLINE_HINTS = {"anthropic-messages","bedrock-converse"}` (42) — OpenAI/Gemini implicit caching is skipped. Manual `CacheHint`s are preserved (gap-filling only, 47-97). Protocols lower hints to `cache_control:{type:"ephemeral", ttl:"5m"|"1h"}` blocks (anthropic-messages.ts:36-40) or Bedrock `CachePointBlock`s (bedrock-converse.ts:81-113,221-237).

AI-SDK stack equivalent: `applyCaching` (transform.ts:335-384) marks the **first two system messages and last two non-system messages** with per-SDK cache options — `anthropic.cacheControl`, `bedrock.cachePoint`, `openrouter.cacheControl`, `openaiCompatible.cache_control`, `copilot.copilot_cache_control`, `alibaba.cacheControl` — applied only for Anthropic/Claude-family models (445-457); message-level vs content-level placement chosen per provider (360-381).

---

## 5. Agent definitions (`packages/opencode/src/agent/agent.ts`)

`Agent.Info` schema (35-55): `mode: "subagent"|"primary"|"all"`, optional `model {providerID, modelID}`, `variant`, `prompt`, `temperature`, `topP`, `color`, `steps` (max agentic iterations), `options`, `permission` ruleset, `native`/`hidden` flags. Native agents (140-265):

| Agent | Mode | Key traits |
|---|---|---|
| `build` | primary | default; `question/plan_enter: allow` |
| `plan` | primary | all edits denied except `.opencode/plans/*.md` + data-dir plans; `task.general: deny`; `plan_exit: allow` |
| `general` | subagent | `todowrite: deny`; parallel-work description |
| `explore` | subagent | `"*": deny` except grep/glob/list/bash/webfetch/websearch/read; own prompt `prompt/explore.txt` |
| `compaction`, `title`, `summary` | primary, **hidden** | `"*": deny`; own prompts; `title` has `temperature: 0.5` |

Base permission defaults (119-136): allow all, but `doom_loop/question/plan_enter/plan_exit` gated, `*.env` reads ask-first. **Config agents** (`cfg.agent`, 267-294) merge/override or `disable` natives; new entries default `mode:"all"`. **Markdown agents** (`config/agent.ts:11-32`): `{agent,agents}/**/*.md` under `.opencode` — frontmatter → config, body → `prompt`; legacy `{mode,modes}/*.md` forced `mode:"primary"` (34-59). **Subagent permission derivation** (`subagent-permissions.ts:14-27`): child session inherits parent's `deny` + `external_directory` rules; `todowrite`/`task` denied unless the subagent's own ruleset permits. **Agent generation** (`agent.ts:368-436` + `generate.txt`): a meta-prompt ("elite AI agent architect") drives `generateObject` with schema `{identifier, whenToUse, systemPrompt}` to author new custom agents; OpenAI-OAuth path uses `streamObject` with `instructions`.

---

## 6. System prompt assembly pipeline

Assembled per step in `session/prompt.ts:1257-1271`, finalized in `session/llm/request.ts:56-112`:

1. **Base prompt** (request.ts:60): `agent.prompt ?? SystemPrompt.provider(model)` — `session/system.ts:27-42` selects a per-model variant by `api.id` substring: `muse-spark`→`meta.txt`; `gpt-4|o1|o3`→`beast.txt`; `codex`→`codex.txt`; `gpt`→`gpt.txt`; `gemini-`→`gemini.txt`; `claude`→`anthropic.txt`; `trinity`→`trinity.txt`; `kimi`→`kimi.txt`; else `default.txt` (14 files, `session/prompt/*.txt`).
2. **Environment block** (system.ts:60-76): powered-by model ID, `<env>` with working directory, workspace root, git-repo flag, platform, date; plus `<available_references>` from configured references (77-94).
3. **Instructions** (`session/instruction.ts`): global `~/.config/opencode/AGENTS.md` or `~/.claude/CLAUDE.md` (unless `disableClaudeCodePrompt`), first matching project `AGENTS.md`/`CLAUDE.md`/`CONTEXT.md` found walking up to worktree (110-133), `config.instructions` globs and **remote http(s) instruction URLs** fetched with 5 s timeout (95-103,155-169); each wrapped as `Instructions from: <path>`. Per-message incremental attachment of nearby instruction files uses a `claims` map keyed by MessageID (179-221).
4. **MCP + skills blocks** (system.ts:98-128): `<mcp_instructions>` filtered by permission; verbose skill listing unless `skill` is permission-disabled.
5. Concatenation `join("\n")`, then plugin hook `experimental.chat.system.transform` may append; if >2 parts and the header is unchanged, tail is re-joined so the base prompt stays the first system message (request.ts:68-78 — cache-prefix stability). `user.system` appended last (62). Structured-output sessions append a mandatory StructuredOutput instruction (prompt.ts:82,1271). OpenAI-OAuth sends system via `instructions` instead of system messages (99-112). Plan-mode reminders inject `plan.txt` (entering plan), `build-switch.txt` (leaving plan), `plan-mode.txt` with `${planInfo}` template substitution (`session/reminders.ts:27-85`).

`generate.txt` is used **only** by `Agent.generate` (agent.ts:380) — the agent-authoring meta prompt — not in the session prompt path.

---

## 7. Design patterns → code map

- **Registry**: `BUNDLED_PROVIDERS` (provider.ts:107), `custom()` loader map (168), models.dev catalog as the model registry (core/models-dev.ts), `providers/index.ts` facade exports.
- **Adapter**: `protocols/*` (canonical `LLMRequest` ↔ provider wire bodies/events; `Protocol.body.from` / `stream.step`); `ProviderTransform.message` adapting `ModelMessage[]` per provider; `native-request.ts` adapting session/AI-SDK shapes into `@opencode-ai/llm` requests.
- **Strategy**: `SystemPrompt.provider` prompt-variant selection; `variants()` reasoning-option strategies keyed by npm package; `temperature/topP/topK` per-model strategies; `selectAzureLanguageModel` / copilot chat-vs-responses selection.
- **Chain-of-responsibility / fallback**: `Credential.orElse` auth chains (route/auth.ts); native→ai-sdk runtime fallback (session/llm.ts:225-275); `defaultModel` chain config→recent→priority-sort; `getSmallModel` family priorities; executor retry chain with exponential backoff + jitter (route/executor.ts:345-360).
- **Template method**: `compile()` fixed pipeline (resolve options → cache policy → body build → validate → transport prepare, client.ts:344-359) with protocol-specific hooks; `Route.make`/`makeFromTransport` skeleton (226-301).
- **Decorator**: `route.with({...})` layered defaults (client.ts:257-268); `wrapSSE` chunk-timeout response wrapper (provider.ts:37-83); `withOpenAIOptions` (providers/openai-options.ts).
- **Capability-based selection**: `Model.capabilities` gates temperature (request.ts:124), attachment modalities (`unsupportedParts` converts disallowed files to `ERROR: Cannot read …` text, transform.ts:386-422), reasoning variants, `toolcall`.
- **Factory + cache**: `resolveSDK` hash-keyed SDK factory cache, `getLanguage` model cache (provider.ts:1722,1830).
- **Dependency injection (Effect)**: every service is `Context.Service` + `Layer.effect` + `LayerNode.make` with declared deps (`@opencode/Provider`, `@opencode/Agent`, `@opencode/LLMClient`, `@opencode/Auth`…).
- **Structured-output uniformity**: `generateObject` implemented as a forced synthetic `generate_object` tool call on every protocol (llm.ts:80-186) — deliberately avoiding provider-native JSON modes.
