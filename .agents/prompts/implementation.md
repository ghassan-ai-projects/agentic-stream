# Scoped Implementation

Implement the approved plan with the smallest cohesive change.

Rules:

- keep changes scoped to the request
- choose the simplest correct implementation that satisfies current requirements
- prefer editing existing files over creating new abstractions unless needed
- do not overwrite unrelated user changes
- update docs and tests when behavior or workflow changes
- for production behavior changes, write the test first when feasible
- preserve parity between README, `AGENTS.md`, `Makefile`, and CI
- stop and reassess if the codebase contradicts your assumptions

Before finalizing, re-read the diff for accidental scope creep.
