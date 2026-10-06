# Watch migration findings

## Mixed responsibilities

- [`effector.go`](../../internal/watch/effector.go) combines construction, route and payload validation, idempotent install orchestration, SQL, and runtime-owner/interlock assertions.
- [`fire.go`](../../internal/watch/fire.go) mixes event fan-out, expiry SQL, expression evaluation with logging, the one-fire-per-event insert and the allowance decrement.
- [`expire.go`](../../internal/watch/expire.go) mixes the SQLite-busy retry policy with a write transaction.
- [`expression.go`](../../internal/watch/expression.go) is pure but lives beside SQL.

## Optional safety dependencies

`NewEffector`/`NewEffectorWithClock` build a working effector with no runtime owner and no interlock; both are attached by `WithRuntimeOwner`/`WithInterlock` and skipped when omitted ("standalone tests may leave the owner unset"). A forgotten setter silently drops the fence on every durable watch mutation. `New(Config)` must require both; composition without a runtime owner supplies an explicit always-pass check, as it does for policy and actions.

## Public surface

| Symbol | Production use | Decision |
| --- | --- | --- |
| `Effector`, `NewEffectorWithClock` | runtime composition, effect routing, pipeline | Replace with `Service` and `New(Config)` |
| `NewEffector(db)` | tests only | Remove; tests use `New` |
| `WithRuntimeOwner`, `WithInterlock` | composition | Remove; constructor requirements |
| `Dispatch`, `DispatchAuthorized` | composite effector | Keep as the effector port |
| `Expire`, `FireEvent` | pipeline | Keep |
| `Fire` | `FireEvent` and tests | Private to app; tests use `export_test.go` |

## Cross-module access

Watch owns `watch_conditions` and `watch_fires` and reads no foreign table. `FireEvent` lists matching watches outside a transaction and then fires each in its own transaction; this stays unchanged.

## Smaller defects

- `dispatch` receives an unused authorization-check parameter.
- Nil-receiver and nil-database guards in every method disappear once `New` guarantees a configured service.
- Expiry SQL is duplicated in `fire.go` and `expire.go`.
- The busy-retry classification calls `storage` from a rules file.
