# A5 — Server / API / Event-Bus Layer & Client Boundary

Repo: `anomalyco/opencode` dev @ a19b52e85bf2 (2026-07-20). All paths below are relative to the repo root.
**Correction to the brief:** the server is **no longer Hono**. Zero `hono` imports remain in `packages/opencode/src`, `packages/server/src`, `packages/protocol/src` (verified by grep). The HTTP stack is **Effect-TS `HttpApi` / `HttpRouter`** (`effect/unstable/httpapi`) served over Node's `http` via `@effect/platform-node`. Hono survives only as a historical design influence.

---

## 1. Server topology

### 1.1 One typed API tree, three composition layers

The API is declared in three places and merged into a single `HttpApi`:

- `packages/opencode/src/server/routes/instance/httpapi/api.ts:54-94` — builds `RootHttpApi` (control, control-plane, global), `InstanceHttpApi` (15 groups: config, experimental, file, instance, mcp, project, project-copy, pty, question, permission, provider, session, sync, tui, workspace), then `OpenCodeHttpApi` = root + `EventApi` + instance + `ServerApi` (the v2 protocol surface from `@opencode-ai/protocol/api`) + `PtyConnectApi`.
- `packages/protocol/src/api.ts:37-64` — the package-owned **v2 protocol**: `HttpApi.make("server")` with groups Health, Location, Agent, Session, Message, Model, Provider, Integration, Credential, Permission, FileSystem, Command, Skill, Event, Pty, Question, Reference, ProjectCopy. Middleware placement is owned by protocol; concrete service keys are injected by server (`packages/server/src/api.ts:5-8`).
- `packages/server/src/routes.ts:39-63` — `createRoutes(password?)` / `createEmbeddedRoutes()` assemble the v2 `Api` with `HttpApiBuilder.layer(Api, { openapiPath: "/openapi.json" })` plus auth/schema-error middleware and the service layer (Database, EventV2, SessionV2, …).

### 1.2 Listener lifecycle

`packages/opencode/src/server/server.ts` is the listener entry:

- `Server.listen(opts)` (`server.ts:73-98`) → `listenEffect` → `HttpRouter.serve(HttpApiApp.createRoutes(opts), …)` (`server.ts:100-115`) over `NodeHttpServer.layer` wrapping a raw `createServer()` (`server.ts:199-224`).
- **Port selection:** explicit `0` → tries **4096 first, then any free port** (`server.ts:117-122`). CLI `serve` prints `opencode server listening on http://host:port` (`packages/opencode/src/cli/cmd/serve.ts:20`).
- **In-process mode:** `Server.Default` (`server.ts:56-65`) exposes `app.fetch` — the full router as a Web `fetch` handler with **no TCP socket**. Used by `opencode run` and the TUI worker (§4).
- **Graceful shutdown:** `Listener.stop(close?)` unpublishes mDNS, optionally force-closes HTTP sockets and all WebSockets via `WebSocketTracker` (`server.ts:172-197`), with a 1-second graceful timeout.
- **OpenAPI:** `Server.openapi()` = `OpenApi.fromApi(PublicApi)` (`server.ts:67-69`); served at `GET /doc` with a lazily-built, cached serialized body (`routes/instance/httpapi/server.ts:183-192`). The CLI `generate` command (`cli/cmd/generate.ts`) prints this spec to stdout.

### 1.3 Route surface (v1-compat "instance" API)

Representative paths (all declared as typed `HttpApiEndpoint`s with Effect Schema codecs and OpenAPI annotations):

- **Session** (`routes/instance/httpapi/groups/session.ts:78-105`): `GET/POST /session`, `GET /session/status`, `GET/PATCH/DELETE /session/:sessionID`, `POST .../fork`, `.../abort`, `.../share`, `.../init`, `.../summarize`, `POST .../message` (prompt), `POST .../prompt_async`, `.../command`, `.../shell`, `.../revert`, `.../unrevert`, message/part delete & update, deprecated `POST .../permissions/:permissionID`.
- **Global** (`groups/global.ts:65-71`): `/global/health`, `/global/event` (SSE), `/global/config`, `/global/dispose`, `/global/upgrade`.
- **Control** (`groups/control.ts:31-34`): `PUT /auth/:providerID`, `POST /log`.
- **Instance** (`groups/instance.ts:43-55`): `/instance/dispose`, `/path`, `/vcs*`, `/command`, `/agent`, `/skill`, `/lsp`, `/formatter`.
- Plus `/question`, `/permission`, `/config`, `/provider`, `/mcp`, `/file` (`find`/`read`/status), `/pty`, `/sync` (`start|replay|steal|history`, `groups/sync.ts:38-43`), `/tui/*` (appendPrompt, openHelp, showToast… `groups/tui.ts:36-46`), `/workspace`, `/experimental`.

The **v2 protocol API** is mounted under `/api/*`: e.g. `GET /api/session`, `POST /api/session/:sessionID/prompt` ("durably admit one session input and schedule agent-loop execution", `packages/protocol/src/groups/session.ts:205-222`), `GET /api/session/:sessionID/event` (`session.events` — replay durable events after a seq then stream new ones, `session.ts:327-341`), `GET /api/event` (whole-server typed SSE, `packages/protocol/src/groups/event.ts:35-44`).

### 1.4 Streaming transports

- **SSE is the primary event transport; WebSocket is used only for PTY.** Three SSE endpoints:
  1. `GET /event` — v1-shape instance stream (`handlers/event.ts:25-87`): emits `server.connected`, filters events by `location.directory`/`workspaceID`, merges a 10 s `server.heartbeat`, ends on `server.instance.disposed`.
  2. `GET /global/event` — unfiltered GlobalBus stream (what the TUI subscribes to, §4).
  3. `GET /api/event` — v2 native payloads, bounded queue of 256 (`packages/server/src/handlers/event.ts:9-49`), 15 s comment heartbeats.
- **WebSocket:** only `PtyConnectApi` — ticket-authenticated upgrade for pseudo-terminals (`groups/pty.ts`, `websocket-tracker.ts`, handler pattern documented in `routes/instance/httpapi/AGENTS.md`).

### 1.5 Auth, CORS, mDNS

- **Auth = optional HTTP Basic.** `OPENCODE_SERVER_PASSWORD` (+ `OPENCODE_SERVER_USERNAME`, default `opencode`) — `server/auth.ts:17-34`. If unset, everything is open and `serve` prints a warning (`cli/cmd/serve.ts:16-18`). Middleware accepts the `Authorization: Basic` header **or** `?auth_token=` query (for SSE/WS clients that can't set headers) — `middleware/authorization.ts:73-83`. PTY connect has a ticket-bypass variant (`authorization.ts:134-150`); public UI assets bypass auth (`isPublicUIPath`, `authorization.ts:110`).
- **CORS:** global `HttpMiddleware.cors` with configurable origins (`routes/instance/httpapi/server.ts:121-128`, `@opencode-ai/server/cors`).
- **mDNS:** `bonjour-service` publishes `_http._tcp` name `opencode-<port>` at `opencode.local` when `--mdns` and hostname is non-loopback (`server/mdns.ts:6-34`, `server.ts:155-170`).
- Other middleware: compression, cors-vary fix, error mapping, `fence`, schema-error, instance-context, workspace-routing (`routes/instance/httpapi/middleware/*`).

---

## 2. Event bus

### 2.1 Three-tier design

1. **`EventV2`** (`packages/core/src/event.ts`) — the real bus: an Effect service (`Service` tag `@opencode/Event`, line 150) combining **typed pub/sub** (one `PubSub` per event type + one `all` PubSub, lines 174-178) with **durable event sourcing** into SQLite. Interface (lines 126-148): `publish`, `subscribe(definition)`, `all()`, `durable({aggregateID, after})`, `listen` (deprecated), `project(definition, projector)`, `replay/replayAll`, `remove`, `claim`.
2. **`GlobalBus`** (`packages/opencode/src/bus/global.ts:11-22`) — a single process-wide Node `EventEmitter` carrying `{directory, project, workspace, payload}` envelopes; auto-assigns `evt_*` ids. This is the **v1-compat fan-out** the SSE handler and TUI worker listen on.
3. **`EventV2Bridge`** (`packages/opencode/src/event-v2-bridge.ts:14-67`) — wraps `EventV2.publish` to attach instance location (directory/workspace/project from `InstanceRef`), and re-emits every EventV2 event onto `GlobalBus` as `{id, type, properties}`. Durable events are additionally re-shaped into legacy **`sync` events** (`{type:"sync", syncEvent:{id,type:versionedType,seq,aggregateID,data}}`, lines 45-60) — the v1→v2 bridge.

### 2.2 Event types and manifest

All event contracts live in `packages/schema/src`. `Event.define({type, durable?: {version, aggregate}, schema})` (`schema/src/event.ts:42-70`) produces a Schema-Struct definition; `Event.latest()` keeps the highest durable version per type (`event.ts:76-92`). The manifest (`schema/src/event-manifest.ts:40-84`) inventories ~30 domains: session (v1 + `session.next.*` v2), permission (v1+v2), question (v1+v2), pty, lsp, mcp, vcs, workspace, worktree, tui, server, legacy…

- **v1 durable session events** (`schema/src/v1/session.ts:502-648`): `session.created/updated/deleted`, `message.updated/removed`, `message.part.updated/removed` — all `durable: {aggregate:"sessionID", version:1}`. Live-only: `message.part.delta`, `session.diff`, `session.error`.
- **v2 granular agent-loop events** (`schema/src/session-event.ts`): `session.next.prompted`, `...step.started/ended/failed`, `...text.started/delta/ended`, `...reasoning.*`, `...tool.called/progress/success/failed`, `...tool.input.*`, `...compaction.*`, `...revert.*`, `...agent.switched`, `...retried`.
- **Interaction events:** `permission.asked/replied` (v1, `schema/src/v1/permission.ts:61-63`), `permission.v2.asked/replied` (`schema/src/permission.ts:43-45`), `question.asked/replied` + `question.v2.asked/replied/rejected` (`schema/src/v1/question.ts:58-59`, `schema/src/question.ts:70-80`).

### 2.3 Event sourcing vs pub-sub — it is both

- **Non-durable events** are pure pub-sub: `publishEvent` → `notify` → listeners + typed/all PubSubs (`core/src/event.ts:369-417`).
- **Durable events** are event-sourced: inside an `immediate` SQLite transaction the writer (a) reads the aggregate's `event_sequence` row, (b) validates seq continuity / replay idempotence / owner, (c) runs **projectors inline** (`for (const projector of list) yield* projector(committed)`, lines 320-322), (d) runs an optional atomic `commit(seq)` hook, (e) upserts `event_sequence`, (f) inserts into `event` (lines 239-352). Only after commit do subscribers get notified (lines 354-360). Replay paths (`replay`, `replayAll`, lines 441-512) support multi-node sync with divergence detection (`Replay diverged at aggregate …`), ownership claims (`claim`), and idempotent re-delivery.
- The design rationale (single-writer total ordering, sync-before-mutation, backwards compat with the old `Bus`) is documented in `packages/opencode/src/sync/README.md` — note the legacy `Bus`/`SyncEvent` modules described there have since been deleted; EventV2 is their successor.

### 2.4 Projectors (read models)

`packages/opencode/src/server/projectors.ts` is now an **empty stub** (`initProjectors(){}`) — historical leftover still imported by `server.ts:1`. The live projector is **`SessionProjector`** (`packages/core/src/session/projector.ts`, 458 lines), registered via `EventV2.project`. It folds v1 durable session events into SQLite read-model tables: `SessionTable` upsert from `SessionInfo` (`projector.ts:44-76`), `MessageTable`/`PartTable` from `message.updated`/`message.part.updated`, and incremental usage accounting (`applyUsage`, lines 90-110, adds cost/token deltas onto the session row from `step-finish` parts). This is textbook **CQRS**: commands mutate via events; queries read projected tables.

---

## 3. Storage

Dual persistence, both rooted at XDG dirs (`packages/core/src/global.ts:10-29`: `~/.local/share/opencode` data, `~/.config/opencode`, `~/.local/state/opencode`, `~/.cache/opencode`):

1. **Legacy JSON file store** (`packages/opencode/src/storage/storage.ts`): key-path addressed JSON documents under `~/.local/share/opencode/storage/**.json` (`file()` at line 63-65), with per-file `TxReentrantLock` read/write locking (lines 218, 266-299), `NotFoundError` mapping, and numbered **migrations** (lines 81-211; migration 1 moves per-project `storage/session/{info,message,part}` trees into `session/<projectID>/…`, `message/<sessionID>/…`, `part/<messageID>/…` and derives project IDs from the git root commit; migration 2 splits summary diffs). Used by v1 session/project/share/todo code.
2. **SQLite via drizzle** (`packages/core/src/database/database.ts`): `EffectDrizzleSqlite` at `~/.local/share/opencode/opencode.db` (channel-suffixed for non-prod installs, `path()` lines 43-55), WAL mode, `busy_timeout=5000`, foreign keys on (lines 27-32). Schema includes:
   - **Event store:** `event` (`id` PK, `aggregate_id` FK→`event_sequence`, `seq`, `type` = `versionedType(type,version)` e.g. `session.updated.1`, JSON `data`) + `event_sequence` (`aggregate_id` PK, `seq`, `owner_id`) — `packages/core/src/event/sql.ts:4-25`, with unique `(aggregate_id, seq)` and `(aggregate_id, type, seq)` indexes.
   - **Projections:** `SessionTable`, `MessageTable`, `PartTable`, `TodoTable`, `SessionMessageTable`, `SessionInputTable`, `SessionContextEpochTable` (`packages/core/src/session/sql.ts:22-168`), plus `WorkspaceTable` etc.
3. **Sync:** `/sync/history|replay|steal|start` endpoints (`groups/sync.ts`) let another node pull durable events after known seqs, replay a full history with owner validation (`handlers/sync.ts:29-60`), or steal ownership — the multi-device/workspace replication built directly on the event store (`EventV2.durable()` streams historical rows then live commits, `core/src/event.ts:585-604`).

---

## 4. Client boundary — one server, many channels

Every channel talks to the **same** `HttpApiApp` router — over TCP, over an in-memory `fetch`, or over RPC:

| Channel | Package | Connection mechanism |
|---|---|---|
| TUI (opentui/solid) | `packages/tui` | `createOpencodeClient({baseUrl, directory, fetch, headers})` from `@opencode-ai/sdk/v2` (`packages/tui/src/context/sdk.tsx:23-31`); SSE via `sdk.global.event()` with manual reconnect + exponential backoff, events batched into Solid `batch()` every 16 ms (`sdk.tsx:82-117`). In the default local setup the TUI runs in a worker thread; its `fetch` is an **RPC bridge** to the main process, which calls `Server.Default().app.fetch` in-process and forwards `GlobalBus` events as RPC `global.event` messages (`packages/opencode/src/cli/cmd/tui.ts:25-48`, `cli/tui/worker.ts:24-56`). Remote `--attach` uses plain HTTP+SSE. |
| CLI headless (`opencode run`) | `packages/opencode/src/cli/cmd/run.ts` | Same SDK client. Default: in-process server — `fetchFn = (req) => Server.Default().app.fetch(...)` with `baseUrl: "http://opencode.internal"` (`run.ts:911-953`). `--attach <url>`: remote (`run.ts:350-351`). Streams events to stdout; `--format json` dumps raw events. |
| `opencode serve` / `web` | `cli/cmd/serve.ts` | Real TCP listener; instances loaded lazily per request via directory header (`serve.ts:10-12`). |
| JS SDK users | `packages/sdk/js` | `createOpencodeServer()` spawns `opencode serve` as a child process and parses the "listening on" line (`sdk/js/src/server.ts:24-60`); `createOpencodeClient` is the hey-api generated client with directory/workspace header injection (`sdk/js/src/v2/client.ts:46-80`). |
| Effect-native / embedded | `packages/sdk-next` | `OpenCode.create()` builds `createEmbeddedRoutes()` from `@opencode-ai/server/routes`, wraps with `HttpRouter.toWebHandler`, and drives it through an Effect `FetchHttpClient` over a fake `fetch` — **same router/handlers/codecs in memory, zero network** (`packages/sdk-next/src/opencode.ts:11-42`). |
| ACP (Zed etc.) | `packages/opencode/src/acp` | Thin adapter: `Agent implements ACPAgent` delegating every ACP method to an `OpencodeClient` (`acp/agent.ts:24-93`, `acp/service.ts`) — ACP is a **protocol channel**, not a separate agent. |
| Desktop/web/console/slack | `packages/desktop`, `packages/web`, `packages/console`, `packages/slack`, `packages/app` | Separate channel packages consuming the same HTTP/SSE API (out of scope here). |

**Agent vs channels:** the "agent" is `packages/opencode` (session/prompt/tool/permission/… services + server) built on `packages/core` + `packages/schema`; `packages/protocol` + `packages/server` define and serve the v2 API. Everything else (`tui`, `desktop`, `web`, `app`, `console`, `slack`, `acp`, SDK consumers) is a channel.

---

## 5. SDK

- **Generated from OpenAPI**, pipeline in `packages/sdk/js/script/build.ts`: (1) run `bun dev generate > openapi.json` in `packages/opencode` (spec from `OpenApi.fromApi(PublicApi)`, post-processed in `routes/instance/httpapi/public.ts` to rewrite query params for caller ergonomics); (2) prune unreachable `SessionNext*1` schemas; (3) run `@hey-api/openapi-ts` (`@hey-api/typescript` + `@hey-api/sdk` plugins) into `src/v2/gen/`.
- **Surface:** `sdk/js/src/v2/gen/sdk.gen.ts` (~7.2k lines) — a single `OpencodeClient` class with namespaced resources (`session.*`, `global.*`, `sync.*`, `tui.*`, `pty.*`, …) matching every operationId; `types.gen.ts` ~13.6k lines; SSE endpoints return async-iterable streams. Legacy `./client` (pre-v2) still exported. `packages/client` additionally holds an **Effect-native generated client** (`generated-effect/`) used by `sdk-next`.
- `packages/sdk/openapi.json` (~37k lines) is the checked-in artifact.

---

## 6. Multi-project / lifecycle

- **Single process, many project instances.** `InstanceStore` (`packages/opencode/src/project/instance-store.ts:37-203`) caches `InstanceContext`s keyed by absolute directory; `load()` boots an instance exactly once (Deferred-guarded, lines 108-124) running `InstanceBootstrap` under `InstanceRef`.
- **Per-request routing:** every instance request carries `?directory=` or `x-opencode-directory` header (default `process.cwd()`) and optional `?workspace=` (`middleware/workspace-routing.ts:22-27, 86-88`); `WorkspaceRoutingMiddleware` can also **proxy** to remote workspace targets, and `InstanceContextMiddleware` loads the instance and provides `InstanceRef`/`WorkspaceRef` to the handler (`middleware/instance-context.ts:22-43`). The SDK injects these headers automatically (`v2/client.ts:46-80`).
- **Disposal:** `POST /instance/dispose` per directory; `POST /global/dispose` disposes all instances then emits `server.disposed` on GlobalBus (`server/global-lifecycle.ts:16-26`); disposal emits `server.instance.disposed`, which terminates each `/event` SSE stream (`handlers/event.ts:59-62`). The listener scope finalizer disposes all instances (`instance-store.ts:192`).
- **Single-instance model:** flock on the state dir (`global.ts:33`); channel-scoped DB filenames avoid cross-channel corruption.

---

## 7. Clone boundary — minimal "headless agent" reimplementation

To clone **only** the agent (prompt in → streamed events out), excluding TUI/desktop/web/slack/console/ACP channels, reimplement:

1. **HTTP+SSE server** with 4 endpoints: `POST /session` (create), `POST /session/:id/message` or `/prompt_async`, `GET /session/:id/message` (history), `GET /event` (SSE: `server.connected`, heartbeat, `{id,type,properties}` frames). Basic-auth optional; `?directory=` routing optional for single-project clones.
2. **Event bus**: typed pub/sub + (if resumability/multi-client sync matters) the durable store — two tables (`event`, `event_sequence`) with per-aggregate monotonic seq, committed atomically with projections (§2.3). The v1→v2 bridge (`EventV2Bridge`) and `GlobalBus` are **not** needed if you emit one canonical event shape.
3. **Session read model**: session/message/part projections (the `SessionProjector` subset) OR the simpler JSON-file store layout (`storage.ts` key-path model) — one of the two, not both.
4. **Agent core**: session services (`Session`, `SessionPrompt` loop, `SessionProcessor`, `SessionStatus`), tool registry/execution, permission + question ask/reply endpoints and their `*.asked/replied` events, provider/LLM layer (Vercel AI SDK), config loading.
5. **Event vocabulary**: the `session.*`/`message.*`/`message.part.*` (+`session.next.*` if cloning v2) and `permission.asked`/`question.asked` contracts from `packages/schema`.
6. **Optional SDK**: regenerate a client from your OpenAPI doc (hey-api) — the in-process `fetch` trick (`Server.Default().app.fetch`) is the cheap way to make CLI and library consumers share one code path.

Explicitly excludable: `packages/tui`, `packages/app`, `packages/desktop`, `packages/web`, `packages/console`, `packages/slack`, `packages/opencode/src/acp`, `/tui/*` + `/pty` + `/sync` route groups, mDNS, workspace proxying, `packages/enterprise`, share-next.

---

## 8. Design patterns → code map

| Pattern | Canonical name | Where |
|---|---|---|
| Event-driven architecture | domain events as sole state-change mechanism | `packages/core/src/event.ts`, `packages/schema/src/event.ts` |
| Event sourcing | durable events, per-aggregate sequences, replay, idempotent re-delivery, owner claims | `core/src/event.ts:205-532`, `core/src/event/sql.ts` |
| CQRS / projections | commands via events; read models built by projectors in the same tx | `core/src/session/projector.ts`, `EventV2.project` (`event.ts:615-620`) |
| Pub-sub | typed `PubSub` per event type + global `PubSub` + deprecated listener list | `core/src/event.ts:174-178, 534-539` |
| Adapter / bridge (v1→v2) | anti-corruption layer reshaping v2 events into legacy `{type, properties}` + `sync` envelopes | `packages/opencode/src/event-v2-bridge.ts` |
| OpenAPI-first SDK | spec from typed API (`OpenApi.fromApi`), client codegen (hey-api) | `server/server.ts:67-69`, `sdk/js/script/build.ts` |
| Hexagonal architecture (ports & adapters) | `protocol` owns contracts; `server` injects services; channels (HTTP, in-memory fetch, RPC, ACP, sdk-next) are interchangeable adapters of the same router | `packages/protocol/src/api.ts:25-26`, `packages/server/src/routes.ts`, `sdk-next/src/opencode.ts` |
| Dependency injection / layered services | Effect `Context.Service` tags + `Layer` composition; per-listener fresh `ConfigProvider` | `routes/instance/httpapi/server.ts:205-313`, `server/server.ts:108-114` |
| Middleware pipeline | typed `HttpApiMiddleware` (auth, instance-context, workspace-routing, schema-error) + router middleware for transport policy | `routes/instance/httpapi/middleware/*` |
| Single-writer total order | one writer per aggregate; seq increments; `steal` transfers ownership | `packages/opencode/src/sync/README.md:29-33`, `handlers/sync.ts` |
| Circuit/liveness | SSE heartbeats (10 s/15 s), bounded queues (`allBounded`, 256) | `handlers/event.ts:63-66`, `packages/server/src/handlers/event.ts:9` |

**Key negative results:** no Hono anywhere in the three server packages; `server/projectors.ts` is an empty stub (real projector in core); WebSocket used only for PTY; legacy `Bus`/`SyncEvent` modules deleted (README is historical).
