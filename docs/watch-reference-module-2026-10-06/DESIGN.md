# Watch module design

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade (`internal/watch`) | `New(Config)`, the effector port methods, `Expire`, `FireEvent` as one-line delegates | Logic, SQL, transactions |
| App (`internal/app`) | Install, fire, fire-event and expire use cases; the SQLite-busy retry; logging a skipped expression | SQL, database handles, raw transactions |
| Domain (`internal/domain`) | Payload validation, watch identity, idempotency comparison, CEL validation and evaluation, scope matching, retry policy constants | I/O, clock reads, `database/sql`, `storage` |
| Store (`internal/store`) | Opaque transactions, the owner and interlock plumbing on the original transaction, every watch SQL statement | Deciding whether a fire or install is allowed |

Public API: `New(Config) (*Service, error)`; `(*Service).Dispatch`, `DispatchAuthorized`, `Expire`, `FireEvent`. `Config` requires `DB`, `RuntimeOwner` (with `Epoch`) and `Interlock`; `Clock` defaults to the physical clock.

Ordering is preserved: install validates the route, then the payload (identity before expression before expiry), then in one transaction loads any earlier install, accepts an identical one or rejects a conflicting one, asserts ownership then the interlock, inserts, and re-verifies. Fire asserts ownership and the interlock (with empty tenant and target, as today), expires due watches, loads the active watch, requires matching Situation and target, evaluates the expression (an evaluation failure is a logged no-fire), records the fire once per event and spends one allowance, disabling the watch at zero.

Enforcement: `TestWatchFacadeOnlyDelegates`, `TestWatchStoreKeepsTransactionsOpaque`, `TestWatchApplicationUsesTransactionalPorts`, existing domain/app/SQL gates, and `durableOwners` pointing `watch_conditions` and `watch_fires` at the store. No schema change.
