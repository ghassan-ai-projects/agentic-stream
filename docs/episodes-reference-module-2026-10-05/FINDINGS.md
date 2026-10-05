# Findings

Survey of `internal/episodes` (15 production files, ~2,400 lines, 47 importing
files) against the reference-module principles.

## The defining constraint: a transaction-threaded public API

Unlike runtime, replay and eventlog, episodes' public operations take the
caller's `*sql.Tx`: `Assembler.Assemble(ctx, tx, ...)`, `Persist(ctx, tx, ...)`,
`Rebind(ctx, tx, ...)` and the runner's claim/conclude sequences run inside
one caller-owned transaction. Admission, replay's store and runtime depend on
this contract (they open the transaction and hand it in) — the same pattern as
`episodeledger` and `scheduleledger` ("durable lifecycle owners shared through
transaction-scoped operations").

A conventional app layer (no `database/sql`) is therefore impossible without
redesigning that cross-module contract, and the layer arithmetic agrees: the
facade must stay at level 5 because `internal/admission` and the executors
(level 6) import it, leaving no room above domain/store for an app level that
imports `control` (level 3) inside transactions. The chosen shape is
facade · domain · store with the transaction-scoped use cases remaining in the
facade package; a full app layer is recorded as a deferred follow-up that
first needs an episodeledger-style unit contract agreed across callers.

## Mixed responsibilities

- `assembler_inputs.go` mixes SQL loads (scheduler item, evaluation, snapshot)
  with pure snapshot validation (schema, identity, entity, digest).
- `runner_claim.go` mixes the dispatchable-episode SQL and hydration with
  freshness decisions (stale vs re-bind vs quarantine) and attempt fencing.
- `runner_decision.go` mixes decision validation, digest rules and the
  `decisions`/`intents` INSERT statements.
- `runner_failure.go` mixes failure classification (pure) with attempt-status
  SQL and episode-terminating transactions.
- `intent_catalog.go` mixes spec compilation glue with pure catalog rules
  (type admission, schema and parameter validation, entry construction).
- `budget.go`, `reconsideration_request.go` document assembly, and the
  quarantine reason mapping are pure rules living beside SQL.

## Durable ownership (verified)

Lifecycle writes already go through owner APIs: `episodeledger.Admit`,
`StartAttempt(Owned)`, `TransitionAttempt`, `Rebind`, `BindRequest`;
`scheduleledger.MarkAdmitted`. Direct SQL is read-only against
`situations`/`situation_versions`/`scheduler_items`/`trigger_evaluations`/
`reconsiderations`/`epoch_control`, plus the module's own handoff writes:
`decisions` (owned) and `intents` INSERT (explicit handoff phase). This moves
to the store layer unchanged.

## Public surface

Used across 47 files: `Request` (70 uses), `Outcome` (43), `NewFakeExecutor`
(24), `NewAssembler` (18), `Executor` (17), `NewRunner` (14),
`BudgetExceededError` (12), `CompileIntentCatalog` (8), `Assembler`, `Runner`,
`BudgetTelemetryMissingError`, `NewRunnerWithEpoch`. Every symbol and
signature is preserved, including the `*sql.Tx` parameters.

## Smaller defects

- `newRequest` asserts `executorDocument["prompt_sha256"].(string)` — an
  unchecked map assertion on a document the assembler itself built.
- Failure reason/status classification is interleaved with the transactions
  it decides for, so it has no table-driven tests of its own.
- `Request` carries parsed budget state (`wallTime`, `wallTimeValidated`)
  beside its durable fields — honest caching, but undocumented as a
  boundary-parse.

## Non-findings (verified clean)

- No effect imports (`forbiddenImports` blocks policy/actions/worker/proto/
  evidence); no clock reads outside `clk.Now()`; lint-clean.
- Claim atomicity, rebind bound (3), epoch kill gate, budget errors, shadow
  scoring and intent handoff are pinned by ten test files including
  `assembly_boundary_test.go`, `p8_*`, `rebind_test.go`, `cancellation_test.go`.
