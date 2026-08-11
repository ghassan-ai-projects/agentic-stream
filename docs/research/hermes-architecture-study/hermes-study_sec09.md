# 9. The Platform Layer: Environments, Security, Providers, and Research Seams

The preceding chapters treated the agent as a loop plus the data the loop reads. This chapter descends one level, to the platform the loop stands on: the execution environments that run its shell commands, the security model that decides which commands may run, the provider stack that decides which model answers, and — as the closing case study — the research seams that once connected this runtime to an RL training stack and were severed in a single pull request without destabilizing anything. The through-line is insight 6: the parts of this platform that survived contact with removal are the ones built as narrow, named seams.

## 9.1 The Trust Model, Quoted and Taken Seriously

### 9.1.1 One boundary, and it is not in the process

Hermes Agent's security policy makes a claim most agent frameworks avoid making about themselves, and it makes it in writing. From `SECURITY.md:60–65` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/SECURITY.md#L60)):

> **The only security boundary against an adversarial LLM is the
> operating system.** Nothing inside the agent process constitutes
> containment — not the approval gate, not output redaction, not any
> pattern scanner, not any tool allowlist. Any in-process component
> that screens LLM output is a heuristic operating on an
> attacker-influenced string, and this policy treats it as such.

The architectural consequence is total delegation: containment lives in the environment layer (section 9.2) or in whole-process wrapping chosen by the operator, and everything in-process — the approval gate, the redactor, the danger-pattern scanner — is classified as a heuristic over attacker-influenced strings. This is not the policy conceding defeat; it is the policy assigning each component an honest role. The in-process layer is built to catch cooperative-mode mistakes and to raise the cost of injection, and the code says so itself: the denylist "is structurally incomplete… catches cooperative-mode mistakes, not adversarial output" (`SECURITY.md:142–146`), and environment scrubbing "reduces casual exfiltration. It is not containment" (`SECURITY.md:121–131`). What real-world exploit resistance this buys is out of scope here; what is verified is that the code's defaults behave the way the policy says they do.

### 9.1.2 The approval gate: chain-of-responsibility with a floor below yolo

`check_dangerous_command` (`tools/approval.py:2877–2944`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/approval.py#L2877)) evaluates a command through an ordered chain: (1) container-guard skip — isolated backends bypass the gate entirely, but local and ssh *never* skip (`approval.py:2863–2874`); (2) a hardline floor — `rm -rf /`, `mkfs`, `dd`-to-device, shutdown, fork bombs — blocked unconditionally, evaluated *before* yolo mode so no operator flag can lower it (`approval.py:2898–2906`); (3) user deny rules, also before yolo; (4) the yolo bypass; (5) a persistent shell-operator-aware allowlist; (6) multi-variant deobfuscated pattern detection; (7) the interactive/smart/gateway decision core. The defaults fail closed and that is verified behavior, not aspiration: an approval timeout denies (default 300 s), an absent human denies, cron sessions deny unless explicitly configured otherwise, and an unscoped secret read under profile multiplexing raises `UnscopedSecretError` rather than risk reading another profile's key (`agent/secret_scope.py:123–177`).

The gate's most instructive detail is a four-line constant at import time, `tools/approval.py:32–35` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/approval.py#L32)):

```python
# Freeze YOLO mode at module import time. Reading os.environ on every call
# would allow any skill running inside the process to set this variable and
# instantly bypass all approval checks — a prompt-injection escalation path.
_YOLO_MODE_FROZEN: bool = is_truthy_value(os.getenv("HERMES_YOLO_MODE", ""))
```

The same treatment protects redaction: `_REDACT_ENABLED` is snapshotted at import so an LLM-issued `export HERMES_REDACT_SECRETS=false` cannot disable it mid-session (`agent/redact.py:61–69`). This is the third canonical instance of the report's capability-subtraction pattern (insight 4): alongside the delegation blocklist and the cron toolset strip, the system defends itself not by instructing the model to behave but by removing the lever the model could pull. Session identity is likewise held in a contextvar, not an environment variable, because concurrent executor threads once raced on `os.environ` and dropped a session onto the non-interactive auto-approve path (GHSA-96vc-wcxf-jjff; `approval.py:54–66`).

## 9.2 Six Backends, Two Methods

### 9.2.1 A two-method contract carrying the whole system

Every terminal backend in Hermes is a `BaseEnvironment` subclass, and the contract each must honor is two methods. `tools/environments/base.py:390–396` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/environments/base.py#L390)):

```python
class BaseEnvironment(ABC):
    """Common interface and unified execution flow for all Hermes backends.

    Subclasses implement ``_run_bash()`` and ``cleanup()``.  The base class
    provides ``execute()`` with session snapshot sourcing, CWD tracking,
    interrupt handling, and timeout enforcement.
    """
```

Everything else — the spawn-per-call `bash -c` flow, the atomically written session snapshot that lets `cd` and exported variables survive across separate processes, the interrupt/timeout/drain loop, bounded output capture — is inherited template method (`base.py:1045–1105`). File tools have no backends of their own: `ShellFileOperations` is a facade over the same shell contract, "implemented on top of the shell contract — they cannot reach paths the backend doesn't expose" (`tools/file_operations.py:793–799`; the security consequence is stated at `SECURITY.md:72–76`). The result is that a command's containment properties are entirely a function of which backend the factory (`tools/terminal_tool.py:1484–1633`, keyed off `TERMINAL_ENV`) instantiates:

| Backend | Execution mechanism | Containment boundary | Approval gate | Persistence model |
|---|---|---|---|---|
| local | host `bash -c`, own process group | none — host trust envelope | always on | host filesystem |
| docker | `docker exec` into long-lived container | the container (cap-drop ALL, no-new-privileges, optional `--network=none`) | skipped only when no host paths bind-mounted | bind mounts or tmpfs per task |
| ssh | `ssh … bash -c` over ControlMaster | network boundary only — someone else's machine | always on | remote FS + two-way `.hermes` sync |
| singularity | `apptainer exec instance://…` | `--containall --no-home` container | always skipped | per-task overlay or writable tmpfs |
| modal (direct/managed) | `sandbox.exec` / gateway HTTPS | cloud VM sandbox | always skipped | filesystem snapshot → restore as image |
| daytona | `sandbox.process.exec` | cloud VM sandbox | always skipped | stop/resume of named sandbox |

Table: the six terminal backends and their trust/containment characteristics (`tools/environments/`; docker hardening at `docker.py:336–344`; approval skip rules at `approval.py:2863–2874`).

Two properties of this table deserve emphasis. First, the asymmetry in the approval column is deliberate and load-bearing: the gate exists to protect the *host*, so it is skipped exactly where an OS-level boundary already stands between the command and the operator's machine — and docker only earns the skip when no host path is bind-mounted into the container, a check (`_docker_has_host_access`) that keeps the convenience of bind-mounted workspaces from silently disabling scrutiny. Second, containment quality is purchased per backend, not per system: local offers none by design, ssh offers someone else's machine, and the cloud backends offer a VM the agent can burn. An operator choosing `TERMINAL_ENV` is choosing the security posture; the code makes no attempt to blur that choice. The concentration of trust is the corresponding risk: because the contract is two methods, every containment guarantee reduces to the correctness of one backend's `_run_bash()` and its platform configuration, and the policy in 9.1.1 is what keeps that concentration honest rather than hidden.

## 9.3 The Five-Layer Provider Architecture

### 9.3.1 api_mode as the central discriminator

Provider support is split across five cooperating layers, and the separation is the single most important thing to understand before cloning:

| Layer | Location | Responsibility | Plugin surface |
|---|---|---|---|
| Auth registry | `hermes_cli/auth.py` (`PROVIDER_REGISTRY`) | provider identity, auth type, env-var priority | static dataclass entries |
| Declarative profiles | `providers/base.py`, `plugins/model-providers/` | per-provider behavior: base_url, headers, quirks via hooks | 33 bundled plugins, lazily discovered |
| Runtime resolver | `hermes_cli/runtime_provider.py`, `hermes_cli/providers.py` | (provider, config, env, auth store) → concrete client parameters | models.dev overlay + user config |
| Transports | `agent/transports/` | OpenAI-shaped state ↔ provider wire format, per `api_mode` | `register_transport()` registry |
| Native adapters | `agent/anthropic_adapter.py` et al. | client construction, token lifecycle, message/tool translation | per-mode adapter modules |

Table: the five provider layers with responsibility and plugin surface. The count of 33 bundled profile plugins is a directory-listing count of `plugins/model-providers/` from a single dimension pass (Medium tier; deterministic method, one counter).

The design's keystone is that only two of these layers touch the wire, and they are keyed by one string: `api_mode` — `chat_completions` (default), `anthropic_messages`, `codex_responses`, `bedrock_converse`, or `codex_app_server` (`agent/agent_init.py:581–612`). The internal representation is OpenAI-shaped end to end — messages, tools, response objects — and each transport's job is confined to converting that invariant shape to and from one provider's wire format (`build_kwargs`, `normalize_response`; `agent/transports/base.py:16–60`). The profile layer is where provider quirks live declaratively: hooks such as `prepare_messages`, `build_extra_body`, and `get_max_tokens` replaced what the module docstring describes as "20+ boolean flags" (`providers/base.py:1–10`), and profiles explicitly "do NOT own client construction, credential rotation, or streaming." Trust in this ring is fail-closed in insight 5's sense: plugin LLM access requires per-plugin opt-in flags before a plugin may override provider or model, and unscoped plugins never see credentials (`agent/plugin_llm.py`). The drift-prone surface is the reasoning-translation shims — reasoning-mandatory models that reject any `reasoning` field, signed thinking blocks that must replay verbatim and in order, encrypted reasoning stamped per issuer — each a 400-error mine encoded in adapter code.

### 9.3.2 Resolution priority: explicit intent wins, OAuth is the last resort

`resolve_provider()` (`hermes_cli/auth.py:1847`, chain documented at `auth.py:1857–1869`, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/hermes_cli/auth.py#L1857)) resolves "which provider?" through an eight-step chain: explicit CLI key → config.yaml → OpenRouter env vars → credential pool → provider-specific keys → `auth.json` OAuth login → AWS credential chain → error. The ordering decision that matters operably is step 6: a logged-in OAuth provider is *demoted to last resort* (issue #29285), because explicit user intent should win over a stale login — a stale OAuth entry warns rather than silently hijacking the session. API keys are additionally scoped to hosts so an aggregator key is never sent to a custom endpoint (`hermes_cli/runtime_provider.py:1132–1156`). Alongside the main lane runs the auxiliary client lane (`agent/auxiliary_client.py`), which serves side tasks — titles, compression, vision, plugin LLM calls — with its own provider chain, per-task config, and ContextVar-based cost accounting. Resilience across both lanes is driven by the ~25-member `FailoverReason` taxonomy (`agent/error_classifier.py:24–72`), which classifies each failure into retry / compress / rotate-credential / fallback actions; the loop-level consumption of that seam is chapter 3's subject and is not re-derived here.

## 9.4 Research Seams and the Amputation

### 9.4.1 The layer that is no longer there

**Stale-source note (C2).** External articles and the still-live documentation page (`hermes-agent.nousresearch.com/docs/developer-guide/environments`) describe an `environments/` directory — `HermesAgentBaseEnv`, `HermesAgentLoop`, `ToolContext`, tool-call parsers, a two-phase GRPO pipeline. Those sources are stale. The directory is absent at HEAD, and this is the one claim in this study verified against live history rather than the snapshot: the GitHub commits API for path `environments/hermes_base_env.py` returns removal commit `5af672c753`, dated 2026-05-15, PR #26106, "chore: remove Atropos RL environments and tinker-atropos integration"; the path 404s both at clone HEAD `4c9628e` and on live `main`.

Historically, the removed layer was a gym-style stack: an Atropos `BaseEnv` subclass that set `TERMINAL_ENV`, resolved Hermes tool schemas, and ran rollouts through an agent loop mirroring `run_agent.py`, with reward functions reaching back into the rollout's own sandbox via a task-scoped `ToolContext` to verify real filesystem state. That is the full historical treatment this chapter gives it — what matters architecturally is not what was deleted but what the deletion did not touch: the loop, the tool dispatcher, and the environments layer all survived unchanged, because the coupling had been built as seams.

### 9.4.2 The seams that survived

| Seam | Location | What it decouples |
|---|---|---|
| `register_task_env_overrides` | `tools/terminal_tool.py:1125–1142` | per-rollout sandbox config (image, cwd) from environment construction |
| per-`task_id` sandbox isolation | `tools/terminal_tool.py:1191`; `batch_runner.py:347` | concurrent rollouts/tasks from shared container state |
| `_run_async` bridge | `model_tools.py:97–122` | sync tool handlers from foreign event loops (gateway, Atropos) |
| ShareGPT trajectory writer | `agent/trajectory.py:30–53` | training-data production from runtime control flow |
| three-layer tool-result budgeting | `tools/tool_result_storage.py:1–23` | context-window safety from any single tool's output |

Table: surviving research seams, with location and what each decouples. The first three are the integration points the RL layer plugged into; the last two are durable assets that serve production independent of any training stack.

The first seam is the most explicit — its docstring still names the consumer that no longer exists, `tools/terminal_tool.py:1125–1132` ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/terminal_tool.py#L1125)):

```python
def register_task_env_overrides(task_id: str, overrides: Dict[str, Any]):
    """
    Register environment overrides for a specific task/rollout.

    Called by Atropos environments before the agent loop to configure
    per-task sandbox settings (e.g., a custom Dockerfile for the Modal image).
    …  # docstring continues: supported override keys and args
    """
```

Each row earns its place differently. The override registry and `task_id` isolation let an external driver configure and fence a sandbox per rollout without the runtime knowing what a rollout is; both now serve the batch runner, ACP sessions, and multi-task isolation generally. The `_run_async` bridge keeps sync tool handlers callable from inside someone else's event loop by running the coroutine on a disposable worker thread — written for Atropos, still required by the gateway. The trajectory writer and budgeting layers are production assets that were always dual-use: budgeting keeps any model's context window safe from unbounded tool output, and the ShareGPT writer turns any session into an inspectable artifact. Nothing in the table required modification when PR #26106 landed; that is the evidence that the seams were real.

### 9.4.3 The lesson, with confidence marked

The architectural lesson is insight 6, now with its proof on the table: design training/eval coupling as injectable seams — narrow, named, callable from either side — so a research layer can be added *or removed* without forking the runtime. Hermes's research-readiness turned out to be seams, not subsystems, and the durable assets are precisely the ones that serve production too. [INFERRED] Nous moved the RL stack out of the OSS repo (to Atropos/Tinker-side or private); the agent repo kept only the generic datagen machinery. The removal itself is verified against the GitHub API; the motive and its impact remain [INFERRED] and are asserted as nothing more.

### 9.4.4 Clone notes

For the builder cloning the agent core: the provider path is stage 1 of the build order — a single `chat_completions` transport with the OpenAI-shaped invariant proves the architecture before any plugin machinery is earned — and the environment-plus-approval pair is stage 4: one `BaseEnvironment` (local) honoring the two-method contract, the file-tools facade over it, and a fail-closed approval gate with the hardline floor and the frozen yolo toggle, roughly 500 lines by the corpus's convergent estimate. The seam-design lesson of 9.4.3 is carried forward into chapter 10: the tool-result budgeting seam reappears there as a named pattern (entry 10) with its own build guidance, and the surviving seams generally anchor Part B's core-interface expectations and re-verify list.
