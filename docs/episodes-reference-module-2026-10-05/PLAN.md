# Plan

| Round | Scope | Proof |
| --- | --- | --- |
| R0 | Survey, layer design, this folder | Full CI baseline green on `2838d05` |
| R1 | Domain layer: budget rules, snapshot evidence validation, request document building, decision digest rules, failure classification, quarantine reason mapping, catalog entry construction, reconsideration document assembly — with table tests; registered in gates | Domain tests, unchanged package suites, lint |
| R2 | Store layer: every SQL statement with the caller's transaction; facade rewires loads/inserts through it | Unchanged package suites green, SQL gate, coverage ≥ 60% |
| R3 | Injection proofs, module README and language guide, AGENTS.md and map updates, rating | Injected violations rejected, `make ci-check`, final report |

## Behavior that must not change

- Claim atomicity (quarantine commits inside the claim transaction), rebind
  bound of 3, epoch kill refusal, supersession watch, detached persist context
  with its five-second budget.
- Digest inputs: admission keys, request JSON documents, snapshot digests,
  decision digests and validation-failure documents.
- Error strings and precedence across assembly, budget validation, decision
  validation and failure classification.
- Transaction boundaries and their owner APIs (episodeledger, scheduleledger,
  qualification, costcontrol), locks and fence semantics.
- The public API, including every `*sql.Tx` parameter.

## Deliberate behavior changes

None.

## Deferred follow-ups

- A full app layer requires an episodeledger-style unit contract agreed with
  admission, replay and runtime so use cases can sequence transactions
  without naming `database/sql`; recorded as a cross-module design change.
- `CompileIntentCatalog` and the executor document stay facade-side until the
  spec projection has a second consumer.

## Status

| Round | Status |
| --- | --- |
| R0 | Accepted, this commit |
| R1 | Accepted: domain layer at level 3 with pure rules and table tests (budgets, snapshot evidence, failure classification with the budget error types, decision digests); facade aliases the error types; suites unchanged and green, domain 88.9% |
| R2 | Accepted: every SQL statement moved to the store layer behind domain-named methods taking the caller's transaction; durable ownership of `decisions` and the `intents` insert handoff moved to the store; the P8-labelled test files renamed to behavior names. Store coverage 67.1% |
| R3 | Accepted: injected violations (SQL in facade, `os` in domain, forbidden imports in store and facade) each rejected; module guide and language guide added; AGENTS.md and repository map updated. Full `make ci-check` recorded in the final report |
