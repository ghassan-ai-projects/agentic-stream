# Mermaid diagram sources (for ghassan-alhamoud.com reuse)

## M1 — Container map

```mermaid
flowchart LR
    subgraph Clients["Clients (channels)"]
        TUI["TUI (OpenTUI/Solid)"]
        CLI["CLI (opencode run)"]
        ACP["IDE / ACP (Zed, VS Code)"]
        SDKU["SDK consumers"]
    end
    subgraph Server["Server (single Bun process, Effect-TS HttpApi)"]
        API["HTTP API (REST + OpenAPI /doc)"]
        SSE["SSE stream (GET /event)"]
        BUS["Event bus (EventV2 + GlobalBus)"]
        CORE["AGENT CORE<br/>session loop · tools · permissions<br/>providers · prompts · compaction"]
    end
    DB[("SQLite: event store + projections")]
    LLM["LLM providers (~26 SDK packages, models.dev catalog)"]
    EXT["Extensibility: plugins · MCP · LSP · skills · commands"]

    Clients -- "REST / control" --> API
    SSE -- "events" --> Clients
    API --> CORE
    CORE --> BUS
    BUS --> SSE
    CORE --> DB
    CORE --> LLM
    EXT --> CORE
```

## M2 — Agent loop sequence (one prompt turn)

```mermaid
sequenceDiagram
    autonumber
    participant C as Client (TUI/SDK/CLI)
    participant L as SessionPrompt loop
    participant P as Permission engine
    participant T as Tool registry
    participant M as LLM provider
    participant E as Event store + bus

    C->>L: POST /session/:id/prompt_async
    loop while(true) — hand-rolled ReAct loop
        L->>L: status=busy; load messages (filterCompacted)
        L->>M: streamText(system prompt + messages + tools)
        M-->>L: LLMEvent stream: text / reasoning / tool-call
        L->>E: commit part.updated (durable) + part.delta (transient)
        alt tool call requires approval
            L->>P: evaluate(permission, pattern)
            P-->>C: permission.asked event
            C-->>P: POST /permission/:id/reply (once/always/reject)
        end
        L->>T: execute tool (abort-aware, truncated output)
        T-->>L: result + <diagnostics> → tool part
        L->>L: guards: doom-loop, overflow→compact
    end
    L->>E: final message + token/cost summary
    E-->>C: SSE /event fan-out
```

## M3 — Permission evaluation

```mermaid
flowchart TD
    A["Tool invoked by LLM"] --> B{"Tool enabled for this agent?"}
    B -- no --> H["Hidden from LLM (registry filter)"]
    B -- yes --> C["Build match pattern<br/>bash → tree-sitter parse · edit/write → file path"]
    C --> D{"evaluate(): ordered rules<br/>config → agent → session 'always'<br/>last match wins"}
    D -- allow --> E["Execute tool"]
    D -- ask --> F["Deferred rendezvous<br/>permission.asked event (loop suspends)"]
    D -- deny --> G["Error returned to model"]
    F --> R["Client replies: once · always · reject(+message)"]
    R -- once/always --> E
    R -- reject --> G
    E --> O["Truncate output → tool-result part → next step"]
```

## M4 — Event-sourced core

```mermaid
flowchart LR
    RT["Session runtime (V1 loop / V2 runner)"] -- commit --> EV["EventV2 (typed Effect PubSub)"]
    EV -- "durable tx (per-aggregate seq)" --> DB[("SQLite EventTable<br/>replay · owner claims")]
    EV -- fold --> PRJ["SessionProjector → relational read models (same tx)"]
    EV -- "v2 → legacy {type, properties}" --> BR["EventV2Bridge → GlobalBus"]
    BR --> SSE["SSE /event → TUI · SDK · CLI · ACP · web"]
    RT -- "transient deltas (not persisted)" --> BR
```

## M5 — Clone blueprint (minimal subset)

```mermaid
flowchart TD
    subgraph Keep["Reimplement (agent-only subset)"]
        S1["Session + message/parts model"]
        S2["while(true) step loop + Runner FSM + abort"]
        S3["Tool contract + registry (6 core tools first)"]
        S4["Permission engine (evaluate + ask reply)"]
        S5["Provider abstraction"]
        S6["System-prompt builder + env + AGENTS.md"]
        S7["Compaction + overflow"]
        S8["Event bus (pub/sub; sourcing optional)"]
        S9["Thin server: 4 endpoints + SSE"]
    end
    subgraph Drop["Exclude"]
        X1["TUI / desktop / web / slack"]
        X2["ACP · PTY · mDNS · share/sync · zen"]
        X3["LSP · MCP · plugins (defer)"]
        X4["skills · commands · worktrees (defer)"]
    end
    S9 --> S2 --> S3
    S2 --> S1
    S2 --> S4
    S2 --> S5
    S6 --> S2
    S7 --> S2
    S2 --> S8 --> S9
```
