## 4. The Tool System

The loop-centric thesis of chapter 1 becomes concrete in the tool system: capabilities in hermes-agent are data — registry entries carrying schema, handler, toolset membership, and an availability probe — not branches in the loop's control flow. Everything the model can do passes through one singleton `ToolRegistry` (`tools/registry.py:765`; [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/registry.py#L765)), and everything in this chapter is a consequence of that decision: how tools are discovered, how they are hidden, how they execute, and how the surface extends without touching core code.

### 4.1 Registration and Discovery

#### 4.1.1 Self-registration at import time, AST-gated discovery

Each module in `tools/` calls `registry.register()` at module top level, declaring a `ToolEntry` whose slots include `name, toolset, schema, handler, check_fn, requires_env, is_async, max_result_size_chars, dynamic_schema_overrides` (`tools/registry.py:87–116`). The dependency chain is deliberately acyclic: `tools/registry.py` imports nothing in-repo; tool modules import the registry; `model_tools.py` imports both and triggers discovery at its own import (`model_tools.py:194–217`; [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/model_tools.py#L194-L217)). The force being traded is explicit: import-time registration pays startup cost and import side effects — every tool module's top-level code runs whether or not the session enables it — in exchange for zero-config discoverability, with no import list to maintain.

Discovery is not a hardcoded list. `discover_builtin_tools()` scans `tools/*.py` and imports only modules that provably self-register, using a cheap text prefilter ahead of `ast.parse`. The gate, `tools/registry.py:43–84` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/registry.py#L43-L84)):

```python
if "registry" not in source or "register" not in source:
    return False
tree = ast.parse(source, filename=str(module_path))
...
return any(_is_registry_register_call(stmt) for stmt in tree.body)
```

Three details carry the design intent. Only module-body statements count, so helpers that call `register()` inside a function are skipped by construction (`tools/registry.py:44–48`). `mcp_tool.py` is excluded; MCP registration has its own dynamic path (§4.2.2). Import failures are swallowed with a warning, so a missing optional dependency such as `fal_client` degrades one toolset rather than the process (`tools/registry.py:78–83`). The countervailing force: AST gating is a brittle heuristic — a module registering conditionally or through an alias would be silently skipped — accepted because it avoids heavy imports of modules that cannot register anything. MCP discovery was moved out of module import entirely because it can block for 120 s and the gateway lazy-imports `model_tools` inside its event loop; each entry point calls `discover_mcp_tools()` explicitly at startup (`model_tools.py:199–210`).

#### 4.1.2 Stale-source note (C4): the real count

The README and website describe "40+ tools." A literal `registry.register()` call-site count across 35 `tools/*.py` files at HEAD yields **74 statically registered built-in tools**, **57 static toolsets** in `TOOLSETS`, of which **25 are `hermes-*` platform bundles** — plus an unbounded number of dynamic MCP and plugin tools at runtime. The figure is Medium-confidence in the report's grading: a single counter, but a deterministic method (cross-verification, conflict zone C4). "40+" is conservative marketing that remains literally true; this study's count of record is 74, resolved to code at HEAD, snapshot July 2026.

### 4.2 Schemas, Toolsets, Gating

#### 4.2.1 Schema emission and `check_fn` gating

Schemas are plain OpenAI function-calling dicts defined as module constants; the registry wraps each in the `{"type": "function", "function": ...}` envelope at emission time, after filtering and after merging any `dynamic_schema_overrides` — a zero-arg callable re-evaluated on every pass, so `delegate_task`'s description can reflect live concurrency limits (`tools/registry.py:558–576`; [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/registry.py#L558-L576)). Between name resolution and wrapping sits the `check_fn` probe: an availability predicate behind a 30 s TTL cache with a 60 s "last-good" grace window, so a flaky Docker, Modal, or Playwright probe does not oscillate the schema (`tools/registry.py:143–206`). A representative call site, `tools/file_tools.py:2104–2107` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/file_tools.py#L2104-L2107)):

```python
registry.register(name="read_file", toolset="file", schema=READ_FILE_SCHEMA, handler=_handle_read_file, check_fn=_check_file_reqs, emoji="📖", max_result_size_chars=100_000)
registry.register(name="write_file", toolset="file", schema=WRITE_FILE_SCHEMA, handler=_handle_write_file, check_fn=_check_file_reqs, emoji="✍️", max_result_size_chars=100_000)
registry.register(name="patch", toolset="file", schema=PATCH_SCHEMA, handler=_handle_patch, check_fn=_check_file_reqs, emoji="🔧", max_result_size_chars=100_000)
registry.register(name="search_files", toolset="file", schema=SEARCH_FILES_SCHEMA, handler=_handle_search_files, check_fn=_check_file_reqs, emoji="🔎", max_result_size_chars=100_000)
```

The four file tools share one probe, so the per-callable cache evaluates it once per schema build. The architectural weight sits here: a tool whose check fails is omitted from the emitted schema, and a tool absent from the schema is physically uncallable — the model cannot invoke what it cannot see. This is insight 4 (capability subtraction beats instruction) at its smallest scale: gating enforced in data at emission time, not by prompt-level "do not use" instructions a model can ignore.

#### 4.2.2 Toolset composition, platform bundles, MCP merge, Tool Search

Toolsets are static dicts of shape `{description, tools, includes}`, with `includes` composing under cycle detection (`toolsets.py:689–769`); registry-only toolsets introduced by plugins and MCP coexist with the static table. The 25 `hermes-*` bundles package capability per deployment: `hermes-cli` and `hermes-cron` are exactly the 54-name `_HERMES_CORE_TOOLS` (`toolsets.py:31–81, 432–447`), `hermes-discord` adds the `discord` pair, `hermes-gateway` composes the messaging bundles via `includes`, and `hermes-webhook` is intentionally minimal — four read-only tools — as prompt-injection hardening (`toolsets.py:86–91`). Disabling a platform bundle subtracts only its non-core delta so shared core tools survive (`model_tools.py:416–438`).

MCP servers merge into the same registry under toolset `mcp-<server>`, names prefixed `mcp__{server}__{tool}`, with per-server include/exclude filters and a prompt-injection scan of each description at registration (`tools/mcp_tool.py:5462–5570`). The collision policy is asymmetric by design — MCP may never shadow a built-in, while MCP↔MCP overwrites are permitted for server refresh. The guard, `tools/mcp_tool.py:5508–5515` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/mcp_tool.py#L5508-L5515)):

```python
existing_toolset = registry.get_toolset_for_tool(tool_name_prefixed)
if existing_toolset and not existing_toolset.startswith("mcp-"):
    logger.warning(
        "MCP server '%s': tool '%s' (→ '%s') collides with built-in "
        "tool in toolset '%s' — skipping to preserve built-in",
        name, mcp_tool.name, tool_name_prefixed, existing_toolset,
    )
    continue
```

On `notifications/tools/list_changed` a server deregisters and re-registers its whole surface — "nuke and repave" — and shutdown deregisters, so dead servers leave no phantom schemas (`tools/mcp_tool.py:3452–3467`). Tool Search then applies progressive disclosure to this volatile surface: when MCP/plugin tools exceed roughly 10 % of the context window, non-core tools collapse behind three synthesized bridge tools (`tool_search`, `tool_describe`, `tool_call`); core tools are never deferred (`model_tools.py:551–579`). The prompt-token economics of that threshold are chapter 5's subject.

### 4.3 Execution Pipeline

#### 4.3.1 Planner → executor → guardrails → approval

An assistant batch is first split by `_plan_tool_batch_segments()` into maximal contiguous parallel runs and sequential barriers, preserving emission order exactly (`agent/tool_dispatch_helpers.py:40–205`). The safety rules are declarative: `clarify` is always a barrier; eleven read-only tools are unconditionally parallel-safe; the path-scoped tools (`read_file`, `write_file`, `patch`) join a run only on non-overlapping canonical target paths; MCP tools join only if their server declared `supports_parallel_tool_calls`; runs shorter than two calls are demoted. Strategy selection falls out of the segmentation — single call: sequential; one parallel segment: concurrent; mixed: segmented — and execution lands on the custom `DaemonThreadPoolExecutor` introduced in chapter 3 (≤8 workers, 420 s batch deadline, abandoned rather than joined on timeout so wedged threads cannot stall CLI exit; `agent/tool_executor.py:95–137, 836–845`). All argument parsing and block decisions complete before any worker starts. Around the dispatch sit the guardrails — a per-turn circuit breaker issuing allow/warn/block/halt decisions on repeated failure signatures (`agent/tool_guardrails.py:63–173`) — and the approval chain-of-responsibility, which lives mostly inside the terminal and `execute_code` handlers rather than in the generic dispatch path: container skip → hardline floor → sudo-stdin guard → user deny rules → yolo/allowlist → pattern and content scanning → prompt (`tools/approval.py:3180–3235`; full treatment in chapter 9). A plugin `pre_tool_call` hook can escalate any tool into the same human gate via `request_tool_approval`. One responsibility ambiguity is worth naming: dangerous-command policy is a property of the tool, not the pipeline, so any path bypassing `handle_function_call` — the `execute_code` RPC channel is the existing example — must re-implement its own guard, which it does via whole-script pre-execution approval.

#### 4.3.2 Extension story and toolset taxonomy

Adding a built-in tool is a two-file change, verified against the developer guide and the code: create `tools/your_tool.py` containing a `check_fn` probe, a handler returning a JSON string with errors as `{"error": ...}` (never raised — the contract is enforced by `_normalize_handler_result`, `tools/registry.py:583–612`), an OpenAI-format schema dict, and a module-level `registry.register()`; then add the name to a toolset in `toolsets.py`. No discovery step exists — the AST gate picks the module up automatically (`website/docs/developer-guide/adding-tools.md:29–211`). The taxonomy below condenses the 74 static tools into the fourteen categories of the `tools/` inventory.

| Category | Representative tools | Count | Bundle membership |
|---|---|---|---|
| File & patch | `read_file`, `write_file`, `patch`, `search_files` | 4 | Core → all `hermes-*` bundles except `hermes-webhook` |
| Terminal & code execution | `terminal`, `process`, `execute_code` | 3 | Core, except `hermes-webhook` |
| Browser automation | `browser_navigate` … `browser_dialog` | 12 | Core |
| Web, search, media & vision | `web_search`, `web_extract`, `x_search`, `image_generate`, `video_generate` | 9 | Mixed: four in core (three also in `hermes-webhook`); `x_search`/video opt-in |
| Audio & voice | `text_to_speech` | 1 | Core |
| Skills system | `skills_list`, `skill_view`, `skill_manage` | 3 | Core |
| Planning, memory & multi-agent | `todo`, `memory`, `clarify`, `delegate_task`, `kanban_*` | 16 | Core; kanban `check_fn`-gated on worker env |
| Messaging & platform | `discord`, `feishu_drive_*`, `yb_*`, `cronjob`, `session_search` | 14 | Mixed: `discord` pair only in `hermes-discord`; `cronjob`/`session_search` core |
| Smart home | `ha_get_state`, `ha_call_service` | 4 | Core-listed, default-off on cron, `HASS_TOKEN`-gated |
| Desktop GUI & computer use | `read_terminal`, `project_*`, `computer_use` | 8 | Mixed: pane tools core (`HERMES_DESKTOP`-gated); `project_*` GUI-gateway only |
| Registry & orchestration core | `tool_search`, `tool_describe`, `tool_call` (synthesized) | — | Infrastructure; emitted by Tool Search, not `TOOLSETS` |
| Approval & security | — (gates, not tools) | — | Cross-cutting guard layer |
| MCP client infrastructure | `mcp__<server>__<tool>` (runtime) | dynamic | Registry-only `mcp-*` toolsets with server-name aliases |
| Misc/support | — | — | Internal helpers |

Two facts stand out. First, the surface is heavy at the edges: browser automation (12) and planning/multi-agent (16, of which kanban alone contributes 12) are the largest clusters, while the classic agent primitives — files, terminal, web — are comparatively small; the system's ambition shows in where the tool mass sits. Second, bundle membership is mostly a function of one list: ten of the fourteen categories feed `_HERMES_CORE_TOOLS`, so platforms differentiate by subtraction (`hermes-webhook` drops to four tools) and by `check_fn` gating (home assistant, kanban, desktop, computer use are core-listed but hidden until their probe passes) rather than by maintaining divergent per-platform lists. The single-source caveat applies to the counts: they derive from the literal call-site count of §4.1.2, and the infrastructure rows carry no static registrations — the Tool Search bridge tools are synthesized at schema-emission time and MCP tools exist only at runtime.

#### 4.3.3 Extensibility rings

The registration, gating, and merge mechanisms above compose into the report's fifth cross-cutting insight, whose primary home is this chapter: extensibility in hermes-agent is concentric.

![Figure 4: Concentric extensibility rings — registry → toolsets → plugins → MCP → skills; capability, integration cost, and trust all decrease moving outward, and the agent's own writes are confined to the outermost ring.](/mnt/agents/output/diagrams/04_extensibility_rings.png)

Moving outward through Figure 4: **native tools** are trusted in-repo code admitted by the AST gate; **toolsets** package that code into per-platform capability surfaces; **plugins** are external code admitted with trust flags — overriding an existing tool requires explicit operator opt-in (`plugins.entries.<id>.allow_tool_override`), bound to the handler's defining module, with a matching gate on `deregister()` to prevent bypass-by-delete (`tools/registry.py:316–347, 459–524`); **MCP servers** are external processes whose descriptions are scanned for injection, whose collisions are skipped, and whose parallelism is opt-in; **skills** are untrusted-but-scanned text, admitted through quarantine and trust tiers — and agent-authored skills, the only ring the agent itself writes, carry provenance tracking (chapter 6). Capability, cost, and trust decrease together moving outward. The design consequence: self-modification is confined to the cheapest, least trusted ring — the loop never gains a new native tool at runtime, but it can gain a new procedure. A clone can adopt the rings incrementally (registry and toolsets first, skills last), yet the learning loop of chapter 7 requires only the skills ring.

#### 4.3.4 Clone notes

> **Clone notes.** The registry plus a minimal toolset is milestone three of the build roadmap, immediately after the provider path and the core loop: roughly 600 lines for the registry and 1,500 for ten to fifteen core tools. A static import list is an acceptable v1 simplification; the AST gate buys discoverability only once third-party modules exist. Two pieces should not be simplified away. First, implement `check_fn`-style gating at schema-emission time early — about 50 lines eliminates the entire class of runaway-capability bugs that prompt-level instructions merely discourage. Second, keep the handler contract exact: JSON string out, errors as `{"error": ...}` and never raised, thread-safe if the tool can appear in parallel batches. The 64-line daemon pool (`tools/daemon_pool.py`) is worth copying verbatim — it exists because wedged tool threads otherwise produce multi-minute CLI exits.
