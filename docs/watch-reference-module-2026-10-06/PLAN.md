# Watch migration plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Domain, store, app and facade together (the ownership gate would otherwise see two writers); update runtime composition and callers | Domain/store/app/facade tests, runtime tests, lint | Pending |
| 2 | Architecture gates, injection proof, module guide and maps | Injected failures, full CI, race | Pending |

## Behavior that must not change

Payload validation precedence, watch identity, idempotency conflicts, fire-once per event, allowance spending and disable-at-zero, expiry marking, the busy-retry bound (3 attempts, 250 ms) and its cancellation error, clock read points, and the exact transaction each mutation uses.

## Deliberate changes

- Owner, epoch and interlock become constructor requirements; composition without a runtime owner passes an explicit always-pass check.
- `NewEffector`, the setters, nil-receiver guards and the unused authorization parameter are removed; `Fire` becomes private.
- The effector type is renamed `Service`.

## Deferred

Typed payload records parsed once at the boundary; moving `FireEvent`'s per-watch fan-out into one transaction would change delivery semantics and is a separate decision.
