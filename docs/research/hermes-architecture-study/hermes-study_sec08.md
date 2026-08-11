# 8. Delegation and Proactivity: Subagents, execute_code, and Cron

Hermes gives the agent three ways to make work happen outside the current turn: `delegate_task` fans reasoning-heavy work out to subagents, `execute_code` collapses mechanical tool chains into one scripted call, and `cronjob` schedules the agent's own future runs. The surfaces differ in isolation, delivery, and cost, but share one governing rule, stated once here as this chapter's spine (cross-cutting insight 4): **capability subtraction beats instruction**. Every recursion or runaway risk these surfaces create is mitigated by removing tools from the spawned context's schema in code — a tool absent from the schema is one the model physically cannot call — never by instructing the model to refrain. Prompt-layer hints (the cron schema's "should not recursively schedule more cron jobs," `tools/cronjob_tools.py:988`) are second-tier reinforcement over a hard control, not the control itself.

## 8.1 Subagent Delegation

### 8.1.1 In-process thread-based subagents

A `delegate_task` call builds a brand-new `AIAgent` per task on the calling thread — fresh `messages` list, ephemeral system prompt assembled from `goal` + `context`, no memory or context files — and runs it on daemon `ThreadPoolExecutor` workers inside the same Python process ([tools/delegate_tool.py:1366–1407](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L1366-L1407); [agent/delegation_context.py:4–6](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/delegation_context.py#L4-L6)). Fan-in is summaries only: the parent's context sees the delegation call and a bounded summary string, never the child's intermediate tool calls; over-budget summaries are head/tail-trimmed with the full text spilled to disk ([tools/delegate_tool.py:1644–1791](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L1644-L1791)).

The in-process thread model buys cheap spawns, a shared credential pool, and zero serialization of parent state; it forfeits hard isolation. Python cannot kill a wedged thread (interrupts are cooperative flags), and only convention plus per-`task_id` namespacing of terminal environment and cwd stops a child from mutating shared process state. Process isolation would add hard kills and memory separation at the price of pickling state, slower spawn, and duplicated provider clients. Hermes accepts threads because the isolation it needs is *conversational* — separate message lists, summarized fan-in — not memory safety.

The model does not choose sync versus async. The dispatch intercept ([run_agent.py:6493–6523](https://github.com/NousResearch/hermes-agent/blob/4c9628e/run_agent.py#L6493-L6523)) forces background for top-level agents — the schema's `background` parameter is ignored — and synchronous execution for orchestrator children, which need worker results inside their own turn and do not own the gateway session an async result would route back to. The trade-off is forced-background delegation versus interactive latency: the parent never blocks, but results re-enter as later turns, so latency-sensitive composition must go through the `orchestrator` role. Sessions that cannot receive a detached result (one-shot `hermes -z`, cron runs, stateless HTTP) degrade to synchronous ([tools/delegate_tool.py:2925–2974](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L2925-L2974)).

### 8.1.2 Hard guards: the blocklist and the depth cap

The primary guard is a constant, quoted from [tools/delegate_tool.py:46–54](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L46-L54):

```python
DELEGATE_BLOCKED_TOOLS = frozenset(
    [
        "delegate_task",  # no recursive delegation
        "clarify",  # no user interaction
        "memory",  # no writes to shared MEMORY.md
        "send_message",  # no cross-platform side effects
        "cronjob",  # no scheduling more work in the parent's name
    ]
```

Two further layers sit behind it: a toolset strip intersected with the parent's own toolsets, so a child cannot gain tools the parent lacks, and exact one-tool deny toolsets that survive composite bundles such as `hermes-cli` ([tools/delegate_tool.py:766–805](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L766-L805)). There is no model-facing `toolsets` argument; the sanctioned exception is `role="orchestrator"`, which re-adds the `delegation` toolset only when the kill switch allows and depth permits.

**Risk callout — unbounded depth when raised (severity: HIGH).** `delegation.max_spawn_depth` defaults to 1 (flat tree) with a floor of 1 and *no ceiling* ([tools/delegate_tool.py:467–503](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/delegate_tool.py#L467-L503); entry guard at :2481–2494). Raising it removes the only depth bound in the system: leaves grow as `max_concurrent_children^depth` — the in-repo documentation notes a 3×3×3 tree reaches 27 concurrent leaves. Existing mitigations cap other axes (`max_concurrent_children` rejects rather than queues; a spawn-pause kill switch; per-child iteration budgets), but none caps depth; operators raising this knob should treat it as an explicit cost multiplier.

### 8.1.3 Durable completion and idle-only re-entry

Async results return as a completion event on the shared `process_registry.completion_queue`, forged into a brand-new turn. Per the module header ([tools/async_delegation.py:15–22](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/async_delegation.py#L15-L22)), completions surface as a new turn only when the agent is idle, never spliced between a tool result and an assistant message — the hard invariant "never mutate past context." This is the role-alternation invariant cross-referenced from chapter 5's cache economics: mid-turn injection would corrupt both the message contract and the byte-stable prompt prefix.

Delivery is durable, not best-effort. Completions persist to a `state.db` table behind a claim/ack protocol — 300 s claim lease, complete/release/drop transitions, `_MAX_DELIVERY_ATTEMPTS = 8` before a terminal `dropped` state ([tools/async_delegation.py:323–424](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/async_delegation.py#L323-L424)). On restart, recovery marks children of provably dead owner PIDs `unknown` and re-enqueues undelivered completions: at-least-once with bounded retries.

All of this runs on the shared `DaemonThreadPoolExecutor` ([tools/daemon_pool.py:1–64](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/daemon_pool.py#L1-L64), 64 lines), whose daemon workers escape the stdlib atexit join that once caused multi-minute CLI exits. Four subsystems reuse it — async delegation, the tool executor (chapter 3's concurrency layer), the memory manager, and the skills hub. The reuse is an operability decision: one exit-safety fix propagates everywhere, but it couples those subsystems' lifecycle behavior, and the async executor grows to meet demand yet never shrinks ([tools/async_delegation.py:464–472](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/async_delegation.py#L464-L472)).

## 8.2 Programmatic Tool Calling

### 8.2.1 execute_code: RPC back into the real dispatcher

`execute_code` is the second surface and the opposite trade: instead of spawning an agent, it spawns a script. The LLM writes Python; a generated `hermes_tools.py` stub module serializes each allowed call over a token-authenticated AF_UNIX socket (file-based RPC on remote backends) into *the same dispatcher the agent loop uses* ([tools/code_execution_tool.py:618–670](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/code_execution_tool.py#L618-L670)). The script runs in a subprocess with a scrubbed environment (secret-substring blocklist plus safe-prefix allowlist), deferred to chapter 9's treatment of execution environments.

The boundary conditions are the interesting part (figures are Medium tier, attributed by method: single code read of the module's constants, dimension 07). The stub generator admits a hard 7-tool allowlist, `SANDBOX_ALLOWED_TOOLS` = {web_search, web_extract, read_file, write_file, search_files, patch, terminal}, intersected with session-enabled tools and enforced again server-side ([tools/code_execution_tool.py:62–70](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/code_execution_tool.py#L62-L70)). Caps are `DEFAULT_MAX_TOOL_CALLS = 50` and `MAX_STDOUT_BYTES = 50_000` with head/tail truncation ([tools/code_execution_tool.py:72–76](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/code_execution_tool.py#L72-L76)). Only the capped stdout re-enters the model context, so loops, filtering, and pagination cost zero intermediate tokens.

Since the script is arbitrary Python that never passes through the terminal tool's dangerous-pattern checks, the whole script is gated before spawn, from [tools/code_execution_tool.py:1223–1230](https://github.com/NousResearch/hermes-agent/blob/4c9628e/tools/code_execution_tool.py#L1223-L1230):

```python
# execute_code runs arbitrary Python (subprocess/os.system/...) that never
# passes through terminal()/DANGEROUS_PATTERNS, so guard the whole script
# here before either dispatch path spawns it. Runs synchronously in the
# caller (tool-executor) thread, which holds the session context (#30882).
# A Docker sandbox with host bind mounts is no longer isolated, so its
# script does not get the container fast-path.
from tools.approval import check_execute_code_guard
_guard = check_execute_code_guard(
```

The approval is whole-script and pre-execution: one decision covers every tool call the script will make, the only workable granularity when the call sequence is data-dependent. Honest residue, carried verbatim from the evidence base: [INFERRED] Residual risk: the script runs with the user's local UID (strict mode) or project venv (project mode) — it is *not* a sandbox in the VM sense unless the terminal backend is remote; the guardrails are allowlist+approval+env-scrub, not seccomp.

## 8.3 Cron: Proactivity with Hard Isolation

### 8.3.1 A fresh isolated session per fire

Cron is the third surface: the agent schedules its own future execution. Each fire builds a fresh, isolated `AIAgent` session — id `cron_{job_id}_{timestamp}`, no memory, no conversation history, session contextvars cleared so the run never impersonates a live user turn ([cron/scheduler.py:2966–3030](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/scheduler.py#L2966-L3030)). The recursion guard is this chapter's second canonical capability-subtraction instance, from [cron/scheduler.py:156–176](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/scheduler.py#L156-L176):

```python
def _resolve_cron_disabled_toolsets(cfg: dict) -> list[str]:
    """Toolsets a cron-spawned agent must never receive.
    Three protected toolsets are always disabled in cron context:
      - ``cronjob`` — would let a cron-spawned agent schedule more cron jobs
      - ``messaging`` — interactive, needs a live gateway session
      - ``clarify`` — interactive, blocks waiting for user input
    ...
    disabled = ["cronjob", "messaging", "clarify"]
```

The strip is layered with the user's config denylist so per-job `enabled_toolsets` cannot widen past policy, and is passed to `AIAgent(disabled_toolsets=...)` at [cron/scheduler.py:3417](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/scheduler.py#L3417). A second hard layer reinforces it: the `cronjob` tool's registration `check_fn` requires interactive environment flags that cron sessions never set, so the tool never enters the schema at all.

### 8.3.2 The LLM as schedule parser — an unusual bet, interrogated

There is no natural-language date parser anywhere in the cron layer. The user says "every morning at 9am"; the model emits a structured `schedule` argument, and `parse_schedule` accepts exactly four shapes — `"30m"` (once), `"every 30m"` (interval), a `croniter`-validated cron expression, or an ISO timestamp ([cron/jobs.py:512–609](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/jobs.py#L512-L609)). The bet: schedule translation is a language task for the model already in the loop, not a parsing problem owned by the scheduler. It pays three ways — no parser dependency to maintain, colloquial recurrence phrasing handled for free, and a small closed output space that is cheap to validate. It costs a silent failure mode: a mistranslated schedule is wrong until the first missed or extra fire, and nothing can detect a semantically wrong-but-well-formed expression. The mitigations are structural, not algorithmic: past one-shots are rejected at create time, and the four-shape whitelist bounds the blast radius. This is the one place in the cron layer where correctness rests on the model rather than on code.

### 8.3.3 At-most-once, by design

Each tick takes a cross-process file lock, finds due jobs, and advances `next_run_at` for all of them *before any execution begins* ([cron/scheduler.py:4029–4035](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/scheduler.py#L4029-L4035)); finite one-shots additionally consume a durable `claim_dispatch` before their side effect runs. The consequence is at-most-once: a crash after the advance but before execution is a *missed* run, never a duplicate. Weighed against exactly-once, this is defensible — exactly-once delivery to chat channels would require a durable intent log plus idempotent side effects at every target, a distributed-transaction cost that buys little for scheduled summaries. The executions ledger is an audit state machine, not a recovery mechanism; its docstring says so verbatim ([cron/executions.py:1–6](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/executions.py#L1-L6)): "The ledger records what is known about each attempt; it is not a retry queue." Interrupted attempts become `unknown` only after their owner process is proved gone; a failed recurring job simply fires at its next occurrence.

Proactivity enters only through consent. Suggestions never auto-create jobs; acceptance calls the same `create_job`, dismissals latch on a stable `dedup_key`, and the backlog is capped at five ([cron/suggestions.py:18–22](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/suggestions.py#L18-L22)). Blueprints parameterize only human-friendly slots (time-of-day, weekday set, interval minutes) over fixed recurrence templates, and `fill_blueprint` returns `create_job` kwargs: one schema, no second job engine ([cron/blueprint_catalog.py:661–674](https://github.com/NousResearch/hermes-agent/blob/4c9628e/cron/blueprint_catalog.py#L661-L674)).

### 8.3.4 The three surfaces compared

| Surface | Isolation level | Capability restriction mechanism | Completion / delivery semantics | Guard type |
|---|---|---|---|---|
| Subagent (`delegate_task`) | Thread in same process; fresh `AIAgent` + fresh message list; per-`task_id` env namespacing | `DELEGATE_BLOCKED_TOOLS` frozenset + toolset strip ∩ parent + depth guard + kill switch | Bounded summary string (sync) or idle-only new turn (async); durable claim/ack, 8 attempts then `dropped` | Hard (code-enforced) |
| `execute_code` (RPC) | Subprocess with scrubbed env; token-authenticated AF_UNIX RPC into the real dispatcher | 7-tool allowlist enforced server-side + whole-script pre-execution approval | Only capped stdout (≤50 KB) re-enters context as the tool result | Hard (allowlist + approval) |
| Cron job | Fresh isolated `AIAgent` session per fire; no history; session contextvars cleared | `cronjob`/`messaging`/`clarify` toolsets stripped in code; `check_fn` registration gating | Final response auto-delivered to channels at fire time; at-most-once via advance-before-execute | Hard, plus second-tier prompt hint |

The right-hand column is uniform, and that uniformity is the chapter's argument: across three surfaces with different threat models, the primary guard is always capability removal in code, with prompt-layer text only as backup. The isolation column forms a gradient — thread, then subprocess, then fresh session — but it does not track risk naively: cron, which runs the *fullest* agent of the three, compensates with the strongest pre-spawn strip and the most conservative delivery semantics, while the least-isolated subagent gets the most engineered return path precisely because it couples tightly to the parent's context. Delivery semantics invert with coupling: the closer a surface sits to the parent conversation, the more machinery surrounds the result's re-entry; the further away, the more delivery leaves the context entirely. Note also what no surface offers: exactly-once. The system prefers auditable misses and bounded retries over duplicate side effects, and encodes that preference in constants and docstrings rather than in policy prose.

### 8.3.5 Clone notes

One related surface deserves a paragraph first. `batch_runner.py` is the datagen-facing cousin of `delegate_task`: it parallelizes across *processes* (`multiprocessing.Pool`, not threads), builds a fresh `AIAgent` per prompt with per-`task_id` environment overrides, and resumes interrupted runs by content rather than index. Its coupling points are treated in chapter 9's research-seams section; here it matters only as evidence that the thread-based default was a choice.

**Clone notes.** In the staged build order, delegation is stage 9 and cron is stage 10 — both omittable from a minimal core, which runs complete turns with provider path, loop, registry, environment, and approval alone. If adopted, delegation is roughly 270 lines: a child factory (fresh agent, goal+context prompt, parent toolsets minus blocklist), a pool runner using `wait(FIRST_COMPLETED, timeout=0.5)` so interrupts stay responsive, a summary gate with head/tail trim and disk spill, and a depth attribute with an entry check. Implement the hard blocklist on day one — about 50 lines prevents the entire runaway-recursion class; the durable claim/ack layer can wait unless the deployment is multi-process. Cron is roughly 300 lines: a JSON job store with atomic writes, a tick loop, a fresh-session executor with the toolset strip, and advance-before-execute. Defer `execute_code` RPC scripting — a second dispatch surface with its own authentication, presupposing a stable dispatcher. Invariants to preserve in either case: remove capabilities in code rather than instructing around them; advance or claim before side effects; re-enter async completions only between turns; give every spawned context a fresh message list with summaries-only fan-in.
