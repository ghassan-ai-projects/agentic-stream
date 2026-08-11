# 3. The Core Runtime: The Conversation Loop

Chapter 1 stated the thesis — Hermes Agent is a loop, not a graph — and chapter 2 named it as the report's central architectural bet (insight 2): behavior is encoded as data the loop reads, not as orchestration topology. This chapter supplies the primary evidence: a dissection of the one function that executes every user turn, reproducible against the pinned snapshot (HEAD `4c9628e`, July 2026).

## 3.1 Where the Loop Actually Lives

The core loop is not a method on the agent class. It is the module-level function `run_conversation(agent, ...)` spanning agent/conversation_loop.py:669–6166 — roughly 3,900 lines in a 6,170-line module ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/conversation_loop.py#L669)). The `AIAgent` class in `run_agent.py` reaches it through a thin forwarder, `AIAgent.run_conversation` (run_agent.py:6613–6668), which publishes two ContextVars — a conversation tag and session accounting handles — so that every auxiliary LLM call inside the turn inherits cost attribution, and then delegates. `AIAgent` itself is now largely a facade: its `__init__` forwards to `agent.agent_init.init_agent` (run_agent.py:498–500, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/run_agent.py#L498)), and dozens of methods are one-line forwarders into `agent/*` modules. The loop module's own docstring names the refactor: "the biggest single chunk pulled out of ``run_agent.py``."

> **Stale-source note (C1).** CONTRIBUTING.md still describes the architecture as "User message → AIAgent._run_agent_loop()" (CONTRIBUTING.md:304, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/CONTRIBUTING.md#L304)). No symbol named `_run_agent_loop` exists anywhere in the tree at this snapshot (verified by recursive grep over `*.py`). The CONTRIBUTING diagram predates the god-file decomposition that extracted the loop into `agent/conversation_loop.py`. Resolution: code at HEAD, snapshot July 2026, is authoritative; any documentation or third-party article that locates the loop in `run_agent.py` is temporally stale.

This matters beyond citation hygiene. A reader cloning "the loop" from the documented location would extract a facade; the actual control flow, recovery logic, and budget enforcement all live in the extracted module.

## 3.2 Turn Lifecycle

### 3.2.1 Prologue seam, two nested loops, epilogue seam

A single user turn passes through four regions (Figure 2). First, the **prologue seam**: `build_turn_context(...)` (agent/turn_context.py, invoked at conversation_loop.py:729–748) performs — per its module docstring — stdio guarding, retry-counter resets, message sanitization, system-prompt restore-or-build, session-row creation, preflight compression, the `pre_llm_call` plugin hook, and crash-resilience persistence. It also binds the execution thread for interrupt targeting (`agent._execution_thread_id`, turn_context.py:1069). The system-prompt step reuses the byte-identical prompt stored in the session row when present — cache-stability machinery owned by chapter 5.

![Figure 2: Turn control flow — prologue seam, outer budget loop, inner retry loop, epilogue seam](/mnt/agents/output/diagrams/02_turn_control_flow.png)

Second, the **outer budget loop** (conversation_loop.py:822), where one iteration equals one model API call. Its skeleton at conversation_loop.py:822–857 shows the two gates every iteration must pass:

```python
while (api_call_count < agent.max_iterations and agent.iteration_budget.remaining > 0) or agent._budget_grace_call:
    ...
    if agent._interrupt_requested:
        _turn_exit_reason = "interrupted_by_user"
        break
    ...
    elif not agent.iteration_budget.consume():
```

Each iteration rebuilds `api_messages` (system-prompt prepend, MoA advisory injection, Anthropic `cache_control` markers), runs preflight context-pressure checks, then enters the **inner retry loop** (`while retry_count < max_retries`, conversation_loop.py:1453; default 3, config `agent.api_max_retries`). The API call passes through a middleware seam (`run_llm_execution_middleware`, conversation_loop.py:1687–1714) into an interruptible wrapper. A response carrying `tool_calls` is dispatched through `agent._execute_tool_calls(...)` (conversation_loop.py:5394 → run_agent.py:6449–6491), whose segment planner splits the batch into parallel-safe runs separated by sequential barriers — the dispatch pipeline is chapter 4's subject; what matters here is that after tools execute, the loop `continue`s, and only a tool-call-free response ends the iteration with a `final_response`.

Fourth, the **epilogue seam**: every exit path funnels into `finalize_turn(...)` (conversation_loop.py:6146–6166 → agent/turn_finalizer.py:69), which handles the budget-exhaustion fallback (§3.3.1), trajectory save, session persistence, and transcript-tail invariants. The two seams are the system's primary extension points: both were extracted behavior-neutrally (their docstrings say so), so a cloner can replace prologue or epilogue policy without touching the loop body.

### 3.2.2 The implicit state machine

There is no state enum. Control state is carried by two artifacts: the diagnostic string `_turn_exit_reason`, initialized to `"unknown"` at conversation_loop.py:784 and overwritten at every exit path, and `TurnRetryState` (agent/turn_retry_state.py:32–79), a per-iteration dataclass collapsing roughly sixteen one-shot recovery guards plus four `restart_with_*` signals (compressed messages, length continuation, rebuilt messages, redirected messages) that the loop reads after each attempt to decide whether to rebuild the request and re-enter. Transitions are `break`/`continue` plus these flags; the comments frame the flags as "restart signals … read by the loop after the attempt," which reads as deliberate design [INFERRED: deliberate — no explicit state enum exists].

The exit reasons observed at this snapshot:

| Exit reason | Set at | Meaning | Restart behavior |
|---|---|---|---|
| `text_response(finish_reason=…)` | conversation_loop.py:6046 | Normal completion; model answered without tool calls | None; loop breaks, finalizer assembles result |
| `budget_exhausted` | conversation_loop.py:854 | `IterationBudget.consume()` returned False at loop top | No in-loop restart; finalizer may issue one tool-less summary call |
| `max_iterations_reached(n/max)` | turn_finalizer.py:124–131 | Finalizer's relabeling of budget exits after the summary fallback | None; result carries the summary or a preserved verification answer |
| `interrupted_by_user` | conversation_loop.py:839 | Interrupt flag seen between iterations | Cooperative break; budget fallback explicitly ineligible |
| `interrupted_during_api_call` | conversation_loop.py:4708 | Interrupt landed mid-request; dangling tool_calls patched closed | Cooperative break after transcript repair |
| `guardrail_halt` | conversation_loop.py:5398 | Tool-guardrail / repeated-invalid-call circuit breaker tripped | Hard break; no retry |
| `all_retries_exhausted_no_response` | conversation_loop.py:4763 | Inner retry loop spent without a usable response | Break; failure result dict |
| `partial_stream_recovery` / `fallback_prior_turn_content` | conversation_loop.py:5547, 5578 | Empty final response recovered from partial stream or prior turn | Recovery supplies `final_response`; loop exits normally |
| `empty_response_exhausted` | conversation_loop.py:5760 | Empty-response nudges hit their attempt cap | Break with failure semantics |
| `ollama_runtime_context_too_small` / runtime-context error | conversation_loop.py:1240 | Local-runtime context floor violation | Iteration refunded (L1243–1246) before exit |
| `local_processing_error(…)` / `error_near_max_iterations(…)` | conversation_loop.py:6136–6139 | Outer safety-net classifier judged the exception a deterministic local bug | Deliberately **not** retried — "they will fail identically on every iteration and only burn the iteration budget" (L6051–6073) |

The table's analytical content is in the last column. The loop distinguishes three classes of ending: terminal success (no restart machinery involved), terminal failure after recovery is exhausted (retries, nudges, and fallbacks already spent), and non-retryable determinism (the outer `except` classifier refusing to re-enter). Note that restart *decisions* live mostly outside this table, in the `TurnRetryState` flags: a compressed-messages restart refunds the iteration and re-enters the same logical step, so it never becomes an exit reason at all. Exit reasons are therefore the residue — what remains after every restart channel has either succeeded silently or hit its one-shot guard. The one-shot convention is load-bearing: a new recovery branch that forgets to set its guard loops forever, and a new restart path that forgets to refund burns budget on recovery rather than progress.

## 3.3 Budget, Concurrency, Interrupt

### 3.3.1 IterationBudget: the runaway-spend brake

Two caps are checked in the outer-loop condition: a per-turn counter `api_call_count < agent.max_iterations` (default 90, run_agent.py:434) and a thread-safe `IterationBudget` instance (agent/iteration_budget.py:17–59). The failure this prevents is unbounded tool-loop churn translating directly into API spend. The core of the counter, agent/iteration_budget.py:37–45:

```python
def consume(self) -> bool:
    """Try to consume one iteration.  Returns True if allowed."""
    with self._lock:
        if self._used >= self.max_total:
            return False
        self._used += 1
        return True
```

Budgets are per-agent, not global: the parent is capped at `max_iterations` (90) and each subagent gets an independent budget capped at `delegation.max_iterations` (default 50), so a delegation tree's total can exceed the parent's cap (iteration_budget.py:20–26). A shared budget can be injected via the constructor so subagent trees draw from one pool [INFERRED: exact call site not read]. The lock exists because the reflection fork and tool workers share the process.

Two subtleties define the correctness surface. First, **refunds**: `refund()` returns one iteration for execute_code-only turns ("cheap RPC-style calls that shouldn't eat the budget", conversation_loop.py:5433–5437), compression restarts (L4711–4713), redirect/rebuild restarts (L4702–4704, L4733–4738), and runtime-context errors (L1243–1246); each refund also decrements `api_call_count`. Refunds are a manual convention — nothing forces a new restart path to refund, and forgetting one makes recovery consume the budget it exists to protect, terminating the turn prematurely. Second, the **exhaustion path**: when the loop breaks with `budget_exhausted` and no `final_response`, the finalizer makes one extra API call with tools stripped, asking the model to summarize (turn_finalizer.py:127–142, [permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/turn_finalizer.py#L127)) — budget exhaustion degrades to a summary, not silent truncation; if a verification gate withheld a composed answer, that exact answer is reused instead. A `_budget_grace_call` flag sits in the loop condition but is only ever set to False in this snapshot — a vestigial/test seam [INFERRED from exhaustive grep]; a cloner should not replicate it.

### 3.3.2 Synchronous loop, threaded tool batches

The loop is plain blocking Python — no `async`/`await` anywhere in the turn path; `finalize_turn`'s docstring states it ("no awaits, no early returns", turn_finalizer.py:14–15). Parallelism is confined to tool batches, which run on a custom pool whose constants sit at agent/tool_executor.py:95–98 ([permalink](https://github.com/NousResearch/hermes-agent/blob/4c9628e/agent/tool_executor.py#L95)):

```python
_MAX_TOOL_WORKERS = 8
# Keep this above the stock auxiliary.web_extract timeout (360s) so the batch
# guard does not preempt a slow-but-valid summarization attempt.
_DEFAULT_CONCURRENT_TOOL_TIMEOUT_S = 420.0
```

The 420 s batch wall-clock deadline exists so a slow-but-valid tool cannot wedge a turn indefinitely. The pool is daemon by deliberate construction (tool_executor.py:700–707): an abandoned batch is shut down with `wait=False`, but stdlib `ThreadPoolExecutor` workers are non-daemon and joined unconditionally by the `concurrent.futures` atexit hook — "so one wedged tool thread would block interpreter exit forever (multi-minute CLI exits)." The failure prevented is a wedged CLI on every interrupted batch; the accepted cost is that a detached tool thread may outlive its turn, which is why request-local cancellation flags exist. Results are appended in original tool-call order; the concurrency verdict (sync loop, daemon pool, 8 workers, 420 s) is corroborated by three independent code reads in the underlying research corpus.

### 3.3.3 Cooperative three-layer interrupt

Cancellation is cooperative, built from three mechanisms. (1) An agent-level flag, `agent._interrupt_requested`, set by `AIAgent.interrupt()` (run_agent.py:2839–2940), which also aborts the active HTTP socket, fans interrupt bits out to tool-worker thread IDs, and recursively interrupts child subagents. (2) Per-thread interrupt bits in tools/interrupt.py:34–70 — a process-global set keyed by thread ID, so interrupting one session "does not kill tools running in other sessions … critical in the gateway where multiple agents run concurrently in the same process." (3) The interruptible API call itself (agent/chat_completion_helpers.py:506–518), which runs the HTTP request on a daemon worker while the main thread polls; on interrupt only the worker's sockets are shut down and the resulting transport error is swallowed via a request-local flag. A `/stop` therefore unwinds deterministically: socket abort → swallowed worker error → `close_interrupted_tool_sequence` patches dangling tool_calls → break with `interrupted_during_api_call` → finalizer closes the transcript tail. The streaming side is protected by a single-writer fence: each attempt claims a monotonic token before consuming its stream (run_agent.py:5341–5358), and superseded writers' deltas are dropped — the failure prevented is two overlapping attempts interleaving text into the consumer. The fence degrades to "no fence" rather than raising, after a real cron crash on a missing attribute (agent/stream_single_writer.py:14–21).

## 3.4 Errors and Honest Assessment

### 3.4.1 Error classification and the provider seam

Every API failure in the inner loop passes through `classify_api_error(...)` (agent/error_classifier.py:24–98; call site conversation_loop.py:3032–3044), which returns a `FailoverReason` plus recovery hints — `retryable`, `should_compress`, `should_rotate_credential`, `should_fallback`. The loop consumes the hints: jittered decorrelated backoff to avoid thundering-herd retries against a shared rate-limited provider; `Retry-After` honored with a 600 s cap; context-overflow errors converted into compression restarts rather than blind retries; `should_fallback` errors routed to `_try_activate_fallback`, which swaps providers mid-conversation and resets the retry counters. This chapter owns the seam, not the taxonomy: the ~25-reason `FailoverReason` enum and the five-layer provider architecture it drives are chapter 9's subject.

### 3.4.2 The 3,900-line loop as design risk

The god-file decomposition moved the pressure; it did not eliminate it. `run_conversation` alone is ~3,900 lines, its inner retry region on the order of 2,400 — a museum of incident IDs (#26293, #29507, #32421, #65991, #66267), each a real provider failure converted into a guarded recovery branch. The load-bearing regions are small and extractable: the outer while-condition, the consume/refund discipline, the tool-dispatch branch, the finalizer's summary fallback. The incidental regions — provider-specific recoveries, MoA injection, redirect/steer plumbing — are additive robustness a clone can defer. The blast radius of a careless edit inside the retry region remains the runtime's largest single design risk: a branch that violates the one-shot guard or refund convention degrades every turn silently rather than failing loudly.

### 3.4.3 Clone notes

> **Clone notes.** Milestone one of a clone is the loop plus the provider path, in that order of risk: a minimal `run_conversation` equivalent — outer budget while, inner retry while, `IterationBudget` with the summary-call fallback, sequential tool dispatch — plus one streaming-capable `chat_completions` transport. That core is the largest single slice of the 10–15% minimal core described in Chapter 10, Part B. Defer the streaming fence, compression restarts, failover chain, and redirect machinery; each is additive and each has a named failure it prevents, so the deferral cost is knowable. Preserve two conventions verbatim from day one: the OpenAI-shaped internal message representation (chapter 9's invariant) and the refund-on-restart discipline — the second is the one a fresh implementation is most likely to get wrong, because nothing in the type system enforces it.
