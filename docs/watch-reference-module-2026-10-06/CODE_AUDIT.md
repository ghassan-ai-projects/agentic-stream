# Code and boundary audit

| Candidate | Production evidence | Decision |
| --- | --- | --- |
| `Effector`, `NewEffector`, `NewEffectorWithClock`, `WithRuntimeOwner`, `WithInterlock` | Only runtime composition and tests constructed and configured it | Replace with `Service` and `New(Config)`; ownership and interlock are constructor requirements |
| Exported `Fire` | Called by `FireEvent` and tests only | Private to app; tests use `export_test.go` |
| Unused authorization parameter on `dispatch` | Never read | Removed |
| Nil-receiver and nil-database guards | Unreachable once `New` guarantees a configured service | Removed |
| Duplicated expiry SQL in fire and expire | Same statement twice | One store operation, `ExpireDue` |
| Pure CEL/payload helpers beside SQL | No I/O | Moved to domain |

Watch owns `watch_conditions` and `watch_fires` and reads no foreign table; the
ownership gate points both at `internal/watch/internal/store`. `FireEvent`
still lists matching watches outside a transaction and fires each in its own
transaction, so a crash between watches leaves earlier fires recorded and later
ones to the next event; changing that is a delivery-semantics decision.
