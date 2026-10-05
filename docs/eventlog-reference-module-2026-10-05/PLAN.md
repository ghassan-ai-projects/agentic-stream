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

None in rounds R0-R3. A later type-safety polish round changed three public
signatures without behavior changes: `RecordGap(ctx, gap Gap)` replaces eight
positional arguments (no production caller existed); the app `Record` type and
row rebuilding moved from the facade to keep `log.go` thin; see the plan
status. The public API keeps every symbol and signature, including
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
| R1 | Accepted: domain layer with pure rules (schema checking, quarantine identity, encoding, stored-time decoding, input validation) at layer 1; eventlog re-leveled 2→4 with ingress 3→5 and evidence 4→5; suite unchanged and green, domain 81% |
| R2 | Accepted: all SQL and transactions in store behind domain-named methods and a caller-owned Unit; use cases in app; facade keeps the public method set and Record. Durable ownership moved to the store layer (authority/policy precedent). Coverage: facade 79%, app 75%, domain 81%, store 76% |
| Polish | Accepted: `RecordGap` takes the typed `Gap` record; the facade's row-to-`Record` rebuilding lives in `internal/app` (`read.go`) so `log.go` stays aliases and delegation |
| R3 | Accepted: injected violations (storage in app, SQL in app, `os` in domain, `actions` in store) each rejected; module guide and language guide added; AGENTS.md and repository map updated. Full `make ci-check` recorded in the final report |
