# Engine module

The engine is the deterministic stream engine. It reads the event log per
partition, runs operators, folds features into Situations, persists versioned
Situations with their lineage, fires durable processing-time timers, and hands
each new Situation version to cognition in the same transaction. It never
invokes models or effects.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `New(ctx, Config)`, `RunGlobal`, and the named `ReplayOwnership` check |
| App | Serialised runs, per-record transactions, timers, heartbeat arming, and rebuild of in-memory Situations after a rollback |
| Domain | Watermarks, persisted Situation state integrity, version-write derivation, collision-free lineage identity, timer matching with boot fencing, heartbeat timer identity |
| Store | Opaque transactions, engine SQL, the owner fence on the original transaction, busy retry, WAL checkpoint, operator-state and timer-payload codecs |

`New` requires the database, event log, compiled spec and runtime ownership
check; a missing one is a constructor error, never a silently skipped check.
Deterministic replay and tests pass `engine.ReplayOwnership` explicitly. The
clock defaults to the physical clock and the tenant to the default tenant;
`Cognition` selects whether Situation versions reach cognition.

A record applies in one transaction: owner fence, inbox check, operators,
Situation folding with cognition, operator-state save and heartbeat re-arming,
then checkpoint and inbox insert. A failure rolls back, resets the in-memory
Situations and restores them from storage. Operators, Situations and cognition
share one deterministic ID sequence in a fixed construction order, so replay is
byte-stable. Timers fire per partition under the same fence; a timer whose
operator state belongs to a fenced boot is acknowledged without firing.

Migration record (`docs/engine-reference-module-2026-10-06/README.md`).
Architecture gates enforce a pure domain, opaque transactions, exact write
ownership of the seven engine tables, and facade delegation.
