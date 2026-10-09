# Review Checklist

Use this before the final answer.

## Scope

- Did the change solve the requested problem and nothing broader?
- Did you avoid speculative application code beyond the documented design?
- Is this the simplest correct change you could defend in review?

## Specificity

- Are instructions specific to `agentic-stream`?
- Did you remove generic advice that is not actionable here?
- Is `AGENTS.md` acting as an entrypoint rather than a dumping ground?

## Consistency

- Do README, `AGENTS.md`, prompt files, and context files agree?
- Do documented commands match `Makefile` and CI behavior?
- Did you avoid duplicating canonical rules across bridge files?

## Quality

- Were tests added for production-code changes?
- For behavior changes, were tests written first when feasible, or was the exception explained?
- Do the tests prove the behavior change rather than merely execute code?
- Do changed tests meet the test bar (`docs/test-audit-2026-10-09/TEST_BAR.md`) T1-T12 (see [testing.md](testing.md)): named for behavior, assert got and want and which error, parallel, no `time.Sleep`, shared fixtures, no dead or duplicate tests?
- Does a change that touches a product invariant update its proving tests in [invariants.md](../../documentation/architecture/invariants.md)?
- Did you run the relevant validation commands?
- Did you record validation failures accurately?
- Did you run `git diff --check`?
- Does the change meet every rule in [quality-bar.md](quality-bar.md) without `//nolint` for complexity or new layering exceptions?
- Q8: did you run the duplication scan in AGENTS.md on the files you changed, and fix every duplicate it found instead of leaving it or noting it?
- Q7: does every new or changed function name its intent, do one thing at one level of abstraction, and sit below the function that calls it? Does each exported entry point read as a short sequence of domain steps?

## Architecture

- Do production imports remain explicitly allowed and strictly downward?
- Are lifecycle writes performed by the owning ledger, within the original transaction?
- Are shared handoffs limited to their producer/consumer fields and operations?
- Can reasoning or replay reach an effect implementation transitively?
- Is final readiness supplied by control without a dispatcher callback?
- Does the public module map agree with the implementation and ADR-017?

## Handoff

- Did you list created and updated files?
- Did you list validation results and skipped checks?
- Did you call out remaining risks and next improvements?
