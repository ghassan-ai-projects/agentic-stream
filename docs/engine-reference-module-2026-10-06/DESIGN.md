# Engine module design

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade (`internal/engine`) | `New(ctx, Config)`, `Service.RunGlobal`, the named replay-ownership value | Logic, SQL, transactions, the mutex |
| App (`internal/app`) | The serialised run, global-run, drain, timer, apply, restore-after-rollback and heartbeat-arming use cases; the in-memory operator, Situation and cognition planes; clock reads and ID order | SQL, database handles, raw transactions, JSON field access |
| Domain (`internal/domain`) | Watermark arithmetic, persisted Situation state integrity and rebuild, version-write derivation, lineage identity, due-timer matching, boot fencing, heartbeat due time and timer identity | I/O, clock reads, `database/sql`, `storage` |
| Store (`internal/store`) | Opaque transactions; owner check on the original transaction; SQL for checkpoints, inbox, operator state, Situations, versions, lineage, timers; restore rows; busy retry; WAL checkpoint; operator-state and timer-payload codecs | Deciding what to apply, fire or restore |

Public API: `New(ctx, Config) (*Service, error)`; `(*Service).RunGlobal(ctx, beforeApply)`; `ReplayOwnership` for runs that have no runtime owner. `Config` requires `DB`, `Log`, `Spec`, `RuntimeOwner` (with `Epoch`); `Clock` defaults to the physical clock, `TenantID` to the default tenant, and `Cognition` selects whether Situation versions reach cognition.

Ordering preserved: construction saves the deployment, configures schema validation, builds operators then Situations over one deterministic ID sequence, restores Situations, then attaches cognition. A record applies in one transaction: owner fence, inbox check, operators, Situation folding, operator-state save and heartbeat re-arming, then checkpoint and inbox insert; any failure rolls back, resets in-memory Situations and restores them from storage. Timers fire per partition under the same fence. The mutex serialises `RunGlobal`.

Cognition receives the caller's transaction through a store-defined interface that `*cognition.Service` satisfies; app never names `*sql.Tx`.

Enforcement: `TestEngineFacadeOnlyDelegates`, `TestEngineStoreKeepsTransactionsOpaque`, `TestEngineApplicationUsesTransactionalPorts`, existing domain/app/SQL gates, and `durableOwners` pointing the seven tables at the store. No schema change.
