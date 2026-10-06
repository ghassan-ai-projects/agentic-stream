# Validation and review

Coverage (short): facade 91.7%, app 78.1%, domain 93.3%, store 81.1%. The
original engine behavior suite (checkpoint advance, busy retry on apply, restart
restore, exactly-once durable timers, retiring a previous boot's timer, global-run
failure preserving progress and inbox deduplication, cognition trigger and
scheduler item) moved to the app layer and passes unchanged in assertions, as do
the runtime, replay (golden), admission and episodes suites that construct the
engine. New tests cover constructor refusal for each dependency, an engine
without ownership applying nothing, watermark monotonicity and parse order,
persisted-state integrity refusals, version-write derivation, lineage collision,
timer matching with boot fencing, heartbeat identity, entity-scoped operator
state, lineage and runtime-state guards, timer replacement and acknowledgement,
and rollback.

Fault injection: a storage failure at each write boundary of an install, fire or record leaves no trace and the event applies exactly once after the fault clears.

Gates proven by injection: facade logic, an exported field on `store.Tx`, a raw
`Exec` and `database/sql` in app, SQL outside the store, and a `timers` write
from app.

`make ci-check` passes; non-short race tests pass for engine, runtime, replay,
admission, episodes and the architecture gates.

| Dimension | Rating / 10 | Remaining limitation |
| --- | --- | --- |
| Layering | 9 | Cognition still receives the raw transaction through an interface |
| Domain rules | 9 | Operator state and features are shared types from `operators` |
| Fail-closed safety | 9 | Replay opts out of ownership by an explicit named value |
| Ubiquitous language | 9 | Statuses and codec versions are literals in SQL |
| Tests | 9 | Fault injection rolls back every write boundary |
| Encapsulation | 9 | Opaque `store.Tx`, injected owner port |
| Type safety | 7 | Facts and timer metadata are untyped maps |
| Simplicity | 8 | The deterministic ID coupling between planes remains |
