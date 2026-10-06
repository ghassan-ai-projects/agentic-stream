# Validation and review

Coverage (short): facade 100%, app 88.4%, domain 96.7%, store 78.6%. The five
original behavior tests (bounded one-shot fire, tamoz-style fallback features,
skipped CEL evaluation, expiry without a fire, expiry retry after SQLite busy)
moved to the app layer and pass unchanged in assertions. New tests cover
constructor refusal for each safety dependency, install refusal without runtime
ownership and with a tripped interlock, route and conflict refusals, the final
authorization check, identity requirements, payload precedence, CEL refusals,
fire-once and allowance spending, expiry visibility, and rollback.

Fault injection: a storage failure at each write boundary of an install, fire or record leaves no trace.

Gates proven by injection: facade logic, an exported field on `store.Tx`, a raw
`Exec` and `database/sql` in app, SQL outside the store, and a `watch_conditions`
write from app.

`make ci-check` passes; non-short race tests pass for watch, runtime and the
architecture gates.

| Dimension | Rating / 10 | Remaining limitation |
| --- | --- | --- |
| Layering | 9 | None known |
| Domain rules | 9 | Payload stays `map[string]any` on the command |
| Fail-closed safety | 9 | Per-watch fan-out is not one transaction |
| Ubiquitous language | 9 | Statuses are constants |
| Tests | 9 | Fault injection rolls back every write boundary |
| Encapsulation | 9 | Opaque `store.Tx`, injected ports |
| Type safety | 7 | Untyped command payload and event features |
| Simplicity | 9 | Three internal layers, no wire package |
