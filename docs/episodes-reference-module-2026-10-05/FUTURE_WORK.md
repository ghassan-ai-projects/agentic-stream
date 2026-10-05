# Focused future work

The reference-module migration is complete. These improvements remain outside
its scope; none requires another broad package refactor.

| Priority | Improvement | Completion evidence |
| --- | --- | --- |
| 1 | Replace store projections over scheduler, Situation and reconsideration tables with read ports supplied by their owning modules as those modules migrate. | All reads retain the original caller transaction; rollback, freshness and replay regressions pass; no new foreign mutations or reverse imports. |
| 2 | Replace map-based canonical document projections with closed types where the field set is known. Retain original documents when digests depend on exact bytes. | Golden canonical bytes, request/Decision digests, unknown-field handling and error precedence stay unchanged; no unchecked field assertions. |
| 3 | Strengthen application failure-path tests beyond the current 64.8% short-race statement coverage. | Target meaningful admission, claim and conclusion failures; prove atomic rollback, cancellation/fencing and cost settlement without duplicating domain tests. |

Current review: layering 10/10; domain rules, fail-closed safety, language,
tests and simplicity 9/10; data encapsulation and type safety 8/10.
The full evidence and constraints are in [VALIDATION.md](VALIDATION.md).
Passing these checks does not establish deployment qualification.
