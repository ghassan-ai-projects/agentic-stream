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
- Did you run the relevant validation commands?
- Did you record validation failures accurately?
- Did you run `git diff --check`?

## Handoff

- Did you list created and updated files?
- Did you list validation results and skipped checks?
- Did you call out remaining risks and next improvements?
