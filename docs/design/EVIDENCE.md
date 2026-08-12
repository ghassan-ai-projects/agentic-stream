# Research and Source Evidence

## 1. Evidence base

This design is grounded in:

| Artifact | Role |
|---|---|
| [Streaming Native Agent Runtime Architecture Report](../research/Streaming_Native_Agent_Runtime_Architecture_Report.docx) | Product thesis, temporal semantics, Situation model, scheduler, action plane, replay, deployment |
| [OpenClaw Architecture and Agent Patterns Study](../research/OpenClaw_Architecture_and_Agent_Patterns_Study.docx) | Run/session separation, tool policy, context, concurrency, resilience, clone boundary |
| [Hermes Agent architecture study](<../research/Kimi_Agent_Hermes Agent Architecture Study/hermes-study.agent.final.md>) | Minimal loop, iteration budget, capability subtraction, skills/memory, provider and environment seams |
| [OpenCode architecture study](../research/opencode-study/OpenCode-Architecture-Study.md) | Hand-owned loop, typed events, fail-closed permissions, client/server boundary, clone scope |

Reference source trees inspected on 2026-07-29:

| Project | Local checkout | Commit |
|---|---|---|
| Hermes Agent | `/Users/ghassan/external-projects/hermes-agent` | `d2d56951a409a3cef73eb95f8f7a93137e1a76ea` |
| LangGraph | `/Users/ghassan/external-projects/langgraph` | `d15bdf7f3ec699a0bc821e4260cd62493ed335e1` |
| LangChain | `/Users/ghassan/external-projects/langchain` | `5f00ec677da0477e39be36272e42102a5b0b0007` |
| OpenClaw | `/Users/ghassan/external-projects/openclaw` | `daa32a0675f97fd4c2312545a391dc9a28695601` |

The checkouts use squashed analysis commits. File paths and behavior are more
meaningful evidence than commit history in these copies.

## 2. Findings that determine the product boundary

### 2.1 “Streaming” is overloaded

LangGraph's `StreamMode` is explicitly:

- full graph values;
- node/task updates;
- model messages;
- checkpoints;
- task lifecycle;
- custom/debug output.

Source: `libs/langgraph/langgraph/types.py:120`.

OpenClaw's `AgentEvent` vocabulary is:

- agent, turn, and message lifecycle;
- incremental assistant updates;
- tool execution lifecycle.

Source: `packages/agent-core/src/types.ts:543`.

Hermes' gateway stream events similarly describe assistant chunks, commentary,
tool-call progress, and delivery lifecycle.

Source: `gateway/stream_events.py`.

These are valuable **execution streams**. They do not define event-time
watermarks, late-data policy, temporal windows, missing-event timers, or
unbounded keyed state. The new runtime must preserve this distinction in names,
contracts, and APIs.

### 2.2 Agent frameworks are episode engines

LangGraph's `Pregel` runtime executes actors in bulk-synchronous steps:
plan, execute selected actors, then update channels. It persists workflow state
through `BaseCheckpointSaver` keyed by a `thread_id`.

Sources:

- `libs/langgraph/langgraph/pregel/main.py:450`;
- `libs/langgraph/langgraph/pregel/_loop.py:158`;
- `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176`.

LangChain's `create_agent` constructs a tool-calling graph with middleware,
structured-output strategies, checkpointer, store, interrupts, and cache.

Sources:

- `libs/langchain_v1/langchain/agents/factory.py:808`;
- `libs/langchain_v1/langchain/agents/middleware/types.py:383`;
- `libs/langchain_v1/langchain/agents/structured_output.py:196`.

These are strong choices inside one EpisodeExecutor. Making either the
continuous event core would import thread/graph semantics where event-time and
Situation semantics belong.

### 2.3 Mature agents own their loop

Hermes' `run_conversation` is a hand-owned loop with explicit per-turn state,
recovery, compression, retry, interruption, and finalization.

Sources:

- `agent/conversation_loop.py:1087`;
- `agent/iteration_budget.py:17`.

OpenClaw's reusable agent core owns nested loops for tool continuation,
steering, follow-ups, abort handling, typed events, and preparation of the next
turn.

Source: `packages/agent-core/src/agent-loop.ts:270`.

The reusable lesson is not to clone their product surfaces. It is to keep the
native episode loop explicit enough to enforce budgets, cancellation, tool
policy, terminal outcomes, and event ordering.

### 2.4 Typed event vocabularies decouple execution and presentation

Hermes documents that the agent emits structured facts and the gateway decides
how each platform renders them. OpenClaw's low-level loop emits stable agent,
turn, message, and tool events to a caller-owned sink.

This supports the design choice to:

- emit one typed internal event vocabulary;
- stream it through SSE or worker RPC;
- keep UI/channel formatting outside the core;
- make durable and ephemeral event classes explicit.

### 2.5 Policy must exist before and during execution

OpenClaw prepares and validates tool arguments, invokes `beforeToolCall`, checks
cancellation, executes the tool, and records a final outcome that also covers
pre-execution failure.

Sources:

- `packages/agent-core/src/agent-loop.ts:940`;
- `packages/agent-core/src/types.ts:49`;
- `src/agents/agent-tools.before-tool-call.policy.ts`.

Hermes removes unavailable capabilities at registry/schema-emission time and
uses a fail-closed approval chain before shell execution.

Sources:

- `tools/registry.py:217`;
- `tools/approval.py:3032`.

The new runtime adopts both principles:

- workers see only capability-scoped evidence tools and allowed Intent schemas;
- every call/Intent is revalidated when executed;
- policy order is code, not prompt prose.

### 2.6 Boundedness is part of correctness

Hermes uses a thread-safe `IterationBudget` with explicit consumption and
refund semantics. OpenClaw threads cancellation through model and tool
execution and normalizes aborted terminal events. LangChain exposes model- and
tool-call limit middleware.

Sources:

- Hermes: `agent/iteration_budget.py`;
- OpenClaw: `packages/agent-core/src/agent-loop.ts`;
- LangChain:
  `libs/langchain_v1/langchain/agents/middleware/model_call_limit.py` and
  `tool_call_limit.py`.

Agentic Stream extends boundedness beyond model iterations:

- event queues;
- window state;
- timer paging;
- cognitive queue;
- episode time/tokens/tools/cost;
- tool result bytes;
- action retries;
- artifact and data retention.

### 2.7 Checkpointed workflow state is not stream recovery

LangGraph checkpoints support state persistence, interrupts, resume, and
time-travel within a workflow thread. They are a good executor capability.
Stream recovery must additionally restore:

- source offsets;
- event-time watermarks;
- window/operator state;
- durable timers;
- keyed Situation order;
- late correction;
- outbox and external-effect reconciliation.

The design therefore keeps framework checkpoints private to an executor and
uses the runtime's event/state ledgers as the product-level recovery boundary.

## 3. Patterns adopted

| Pattern | Source | Application in Agentic Stream |
|---|---|---|
| Hand-owned bounded loop | Hermes, OpenClaw, OpenCode | Small native episode loop |
| Run/attempt separation | OpenClaw study | Episode identity separate from execution attempt |
| Typed lifecycle stream | Hermes, OpenClaw | Episode/Situation/action event vocabulary |
| Single logical writer | OpenClaw study, LangGraph state steps | One writer per virtual partition and one active episode per Situation |
| Typed tool registry | Hermes, OpenClaw, LangChain | Evidence tool catalog with schema and capability |
| Pre-inference capability filtering | Hermes/OpenClaw studies | Worker receives only allowed tools/intents |
| Execution-time policy | OpenClaw source | Tool and Intent revalidation |
| Fail-closed default | Hermes/OpenCode studies | Unknown tool/action/policy outcome denies |
| Explicit budgets | Hermes, LangChain | Episode and resource budgets |
| Cooperative cancellation | OpenClaw | Supersession and deadlines through context/RPC |
| Append-only transcript/ledger | OpenClaw study, OpenCode study | Immutable Situation, episode, Decision, and action records |
| Durable checkpoints | LangGraph | Inspired executor adapter contract, not stream semantics |
| Middleware/adapter seams | LangChain | Optional executor/provider wrappers |
| Progressive disclosure | Hermes/OpenClaw studies | Bounded evidence tools instead of context dumps |
| Outbox/idempotency | architecture report | Cross-boundary state/effect correctness |
| Recorded cognition | architecture report | Faithful incident replay despite model nondeterminism |

## 4. Patterns deliberately rejected or deferred

| Pattern/surface | Decision | Reason |
|---|---|---|
| One agent call per event | Reject | cost, backlog, staleness, oscillation |
| Broker as the architecture | Reject | transport does not define situations, triggers, or action safety |
| LangGraph as stream core | Reject | graph invocation/checkpoint semantics do not own event-time truth |
| LangChain as core dependency | Reject | useful adapter ecosystem; unnecessary hot-path coupling |
| Hermes/OpenClaw fork | Reject | most code is channels, sessions, workspaces, tools, and provider/product surface |
| Long-lived agent waiting on events | Reject | hard to budget, cancel, replay, and upgrade |
| Direct model mutation of Situation | Reject | weak replay, validation, and provenance |
| Direct model access to effectors | Reject | policy bypass and duplicate-effect risk |
| Arbitrary scripts in SituationSpec | Reject | nondeterminism and unsafe plugin boundary |
| Vector database as memory | Defer | not needed for temporal state; evaluate curated knowledge recall first |
| Multi-agent orchestration | Defer | added cost/latency without demonstrated utility |
| Self-authored live policy | Reject | unsafe feedback and uncontrolled drift |
| Distributed stream processing | Defer | semantic correctness must be proven on one node |
| Full web UI | Defer | CLI and stable API establish operator workflows first |
| General shell/browser tools | Reject in core | unnecessary capability and containment surface |

## 5. Evidence-to-decision matrix

| Design decision | Evidence |
|---|---|
| Situation-centered product | Primary streaming architecture report |
| Go modular monolith | Report language evaluation plus core workload analysis |
| Go-only workers | Go runtime and protocol boundary selected for one operational toolchain; Python agent/ML ecosystems are not a v1 runtime target |
| No agent framework in core | Framework source boundaries and event-time gap |
| Explicit native episode loop | Hermes/OpenClaw/OpenCode loop evidence |
| Typed worker event stream | Hermes/OpenClaw structured event contracts |
| Capability-scoped read tools | Hermes registry and OpenClaw policy hooks |
| Version-bound cancellation | Architecture report supersession plus OpenClaw abort model |
| SQLite WAL first | Hermes/OpenClaw/OpenCode local durability studies and modular-monolith scope |
| Append-only ledgers | OpenClaw/OpenCode session/event patterns and replay requirements |
| Replay before live autonomy | Architecture report and model nondeterminism |
| No exact-once marketing | Stream/action boundary analysis and reference failure policies |

## 6. Interpretation cautions

1. The architecture studies and source checkouts represent July 2026 snapshots.
   Provider defaults, package names, and file layouts will continue changing.
2. Source projects solve different products. A mechanism's existence is
   evidence that it works in its context, not that it belongs in this runtime.
3. LangGraph's use of “stream” and “checkpoint” is correct for graph execution.
   The design rejects only an incorrect mapping from those terms to event-time
   stream processing.
4. Hermes and OpenClaw have accumulated robustness from many incidents. The new
   runtime should copy contracts and guards, not thousands of provider-specific
   recovery branches before encountering the corresponding need.
5. The design's Go/framework selection is an architectural decision for v1.
   Python agent ecosystems were considered as research evidence but are not a
   worker, SDK, or deployment target.

## 7. Re-verification checklist before adapter implementation

Before implementing a framework adapter:

- pin an exact upstream tag or commit;
- re-read its current runtime and public extension contract;
- confirm cancellation and terminal-outcome semantics;
- confirm tool schema and structured-output support;
- confirm checkpoint/session ownership;
- run the Agentic Stream EpisodeExecutor conformance suite;
- ensure the adapter cannot reach action credentials or mutate Situation state;
- document license obligations and preserve required notices.
