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
| R1 | Pending |
| R2 | Pending |
| R3 | Pending |
