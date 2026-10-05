# Plan

| Round | Scope | Proof |
| --- | --- | --- |
| R0 | Survey, layer design, this folder | Full CI baseline green on `4ecad37` |
| R1 | Domain layer: vocabulary and pure rules (schema checking, quarantine identity, encoded events, stored-record decode, input validation) with table tests; registered in gates and repository map | Domain tests, unchanged package suite, lint, architecture tests |
| R2 | Store (all SQL and the `Unit` plumbing) and app (use cases); facade reduced to delegation; package suite still unchanged | Package tests unchanged and green, each layer ≥ 60% coverage, SQL and app gates |
| R3 | Injection proofs for the new gates, module README and language guide, AGENTS.md and map updates, rating | Injected violations rejected, `make ci-check`, final report |

## Behavior that must not change

- Error strings and precedence: contract validation before schema validation
  before insert; trace-context validation before the duplicate check; the
  exact wraps (`append events:`, `validate envelope:`, `insert event:`,
  quarantine and gap texts).
- Transaction boundaries: one transaction per append batch (rollback of
  earlier records on later admission failure), per quarantine, per release,
  and one atomic redrive.
- Identity and digests: quarantine `q_` ids, payload digests, `payload:`
  fallback ids, gap ids, event payload SHA-256 columns.
- Duplicate semantics: ignored inserts report -1; quarantine retries bounded
  at 10; hash conflicts reject; release/redrive exactly once.
- Clock reads: one `clk.Now()` per inserted event, formatted RFC3339Nano UTC.
- Read semantics: tenant scope, position filter, optional partition filter,
  default limit 1000, log order, streaming with early stop.

## Deliberate behavior changes

None. The public API keeps every symbol and signature, including
`ReadEntityWindow(ctx, db, ...)` taking the database handle (its callers in
other modules own that handle; changing the signature is a cross-module
follow-up, not part of this migration).

## Deferred follow-ups

- Move `ReadEntityWindow` callers onto a facade method so they stop holding
  the raw database handle.
- Typed payload access on `Record` (the envelope stays the contract).

## Status

| Round | Status |
| --- | --- |
| R0 | Accepted, this commit |
| R1 | Pending |
| R2 | Pending |
| R3 | Pending |
