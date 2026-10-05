# Engine migration plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Domain: watermark, state integrity, version write, lineage, timers, heartbeat | Domain table tests incl. the lineage collision test | Complete |
| 2 | Store and app and facade together (ownership gate); update runtime composition, replay and test callers | Engine, runtime, replay, admission tests; golden replay | Complete |
| 3 | Architecture gates, injection proof, module guide, maps, validation | Injected failures, full CI, race | Complete |

Layer numbers moved: engine is 10 (domain 5, store 6, app 9), so `runtime/internal/app` is 11, `runtime/internal/composition` 12, `runtime` 13, `replay/internal/store` 11, `replay/internal/app` 12, `replay` 13 and `cmd/agentic-stream` 14. These are reviewed levels, not behavior.

## Behavior that must not change

Operator, Situation and cognition construction order and the shared deterministic ID sequence; the per-record transaction contents and order; every `clock.Now()` read point; the inbox-based idempotency; watermark monotonicity; the rebuild-after-rollback of in-memory Situations; timer matching, boot fencing and acknowledgement; heartbeat arming and idempotent re-arming; the SQLite-busy retry on apply; the WAL checkpoint tolerance of busy.

## Deliberate changes

- Owner check and epoch become constructor requirements; replay passes `engine.ReplayOwnership`.
- `NewEngine`/`NewStreamEngine`/`WithRuntimeOwner` become `New(ctx, Config)` with a `Cognition` flag; the type becomes `Service`.
- `Run` and `RunDueTimers` become private (tests only).
- Error text drift without behavior change where one wrapper moves between layers.

## Deferred

Typed operator-state and timer-payload records shared with `operators`; one transaction per global batch; replacing the deterministic ID coupling between planes.
