# Cognition survey findings

## Mixed responsibilities

- `internal/cognition/engine.go` evaluates versions, queries prior Situation
  rows, parses persisted time/trace/snapshot fields, updates
  `situations.last_reasoned_version`, and derives trigger identities.
- `engine_trigger.go` and `engine_cel.go` build the CEL input, compile and run
  expressions, select the first failed gate, and form deltas.
- `scheduler.go` combines trigger outcomes, queue policy, timing, SQLite
  upserts, JSON/digest encoding, schedule-ledger calls, and notification writes.
  `scheduler_items.go` and `scheduler_supersede.go` add SQL reads and lifecycle
  coordination.
- `reconsideration.go` and `reconsideration_evidence.go` mix correction and
  invalidated-action rules with snapshot decoding, schema/digest validation,
  foreign-table queries, writes, and notification encoding.
- `admission_audit.go` reads and updates cognition-owned evaluations while
  combining JSON decoding and the cost-refusal explanation.

The whole current package is one public Go package and owns no private
application, domain, or store boundary. All production SQL is in these package
files, but it includes queries of Situation history, scheduler items, accepted
intents/decisions, episodes, commands, and outcomes.

## Public surface and actual consumers

| Current symbol | Production use outside cognition | Decision |
| --- | --- | --- |
| `Engine`, `NewEngine`, `Engine.Process` | `internal/engine` constructs and calls it in event and timer transactions | Replace with `Service`, `New(Config)`, and `Process`; preserve the caller transaction. |
| `RecordCostRejectionReason` | `internal/admission/skip.go` records the refusal before the scheduler ledger coalesces the same item | Keep as a facade operation; join and use the exact transaction supplied by admission. |
| `Evaluation`, `Scheduler`, `NewScheduler`, `Scheduler.Admit` | No production caller outside cognition | Move evaluation and scheduler implementation types behind the facade. |

Test callers outside the package are in `internal/episodes/internal/app/runner_test.go`,
`assembler_test.go`, and `rebind_test.go`. They construct cognition engines and
call `Process`; update them to the configured service. The production constructor
caller is `internal/engine/engine_construction.go`; its field is in
`internal/engine/engine.go`. No runtime package constructs cognition directly.

## Safety and behavior risks

- `NewEngine` accepts a database argument but never reads or stores it. Tests
  pass nil successfully. Database access occurs only through the transaction
  passed to `Process` or `RecordCostRejectionReason`.
- A nil clock defaults to the physical clock. Keep this default. The ID
  generator is passed to the scheduler, which defaults a nil generator; the
  Engine also stores the generator without using it. Remove only that dead
  field, not the scheduler default.
- The engine and scheduler carry duplicate spec, clock, and ID-generator
  configuration. Consolidate the state in the application service.
- Trigger gate order is observable: condition, score, material delta, then
  threshold. A material-delta error retains the score but adds no reason.
- Evaluation time, item timing, supersession, reconsideration persistence, and
  notifications read the clock at distinct points. Preserve their order and
  number of reads.
- Processing, cost refusal, schedule-ledger transitions, episode supersession,
  approval withdrawal, and notifications share the caller's transaction.

## Cross-module data access

### Cognition queries other modules

The current correction query joins `commands`, `intents`, `decisions`,
`episodes`, and `outcomes` to find the latest accepted successful action. The
current supersession query reads Situation version/trace rows and scheduler
items. Other reads obtain Situation history and scheduler queue counts. Keep
these SQL statements in cognition's store in this migration; owner-provided
read ports that preserve the exact caller transaction are a focused follow-up.

### Other modules query cognition tables

- `internal/scheduleledger/lifecycle.go` reads `trigger_evaluations` to find
  trigger IDs for queue coalescing.
- `internal/episodes/internal/store/assembler.go` reads `trigger_evaluations`;
  its reconsideration query is in `reconsideration.go` under the same store.
- `internal/policy/internal/store/approval_context.go` reads
  `trigger_evaluations` to assemble approval evidence.

These are current read relationships, not additional mutation owners.
`architecture_ownership_test.go` assigns `trigger_evaluations` and
`reconsiderations` to cognition; cognition's only permitted foreign write is
the documented `situations.last_reasoned_version` handoff. Preserve that
ownership and report the cross-module reads separately from write ownership.

## Smaller defects and test-only code

- `NewScheduler`, `Scheduler`, `Scheduler.Admit`, and `Evaluation` are exported
  by Go but have no production callers outside cognition. Make implementation
  types private; keep an external evaluation result only if a real caller
  appears.
- `Engine.idGen` is assigned but never read. Remove it; keep the scheduler's
  random-generator fallback.
- The database constructor argument is unused. Remove it in the new constructor
  and record the signature change.
- `judgeTrigger` and the evaluator path carry a context that is not checked or
  used by CEL; keep cancellation at the I/O use-case boundary and remove it
  from pure rules.
- `fmt.Errorf("%w", err)` wrappers in `scheduler_supersede.go` and
  `scheduler_items.go` add no operation context and should be removed while
  preserving error identity.
- `TestInsertItemIgnoresDeterministicIDCollision` tests a meaningful
  no-overwrite behavior through private scheduler helpers. Keep the behavior
  test at the store boundary or rewrite it through the service; do not discard
  the assertion.
- `nullablePrevious`, SQL fixture insertion functions, and database builders
  are test-only setup helpers. Keep them in tests; they are not production dead
  code.
