# Validation and review

Focused correctness tests pass. Coverage: facade 100%, app 71.4%, domain 79.4%,
store 64.5%. Existing trigger/timing/capacity/coalescing/reconsideration tests are
retained. New tests cover caller rollback, missing construction/transaction
inputs, pure gate precedence, timing boundaries, source evidence and digest
refusal. Full repository CI and uncached non-short race are pending the final
integrated migration gate.

| Dimension | Rating / 10 | Remaining limitation |
| --- | --- | --- |
| Layering | 9 | Read-only owner projections still use store joins |
| Domain rules | 9 | Dynamic CEL inputs are schema-shaped maps |
| Fail-closed safety | 9 | Caller must supply its authorized transaction |
| Ubiquitous language | 9 | Canonical language guide tracks durable names |
| Tests | 8 | Targeted storage faults could cover more refusal branches |
| Encapsulation | 9 | Opaque transaction; caller compiled spec remains an immutable contract |
| Type safety | 8 | CEL and snapshot schema data remain dynamic |
| Simplicity | 9 | Four layers, no new table abstraction or commit boundary |

Future work is ordered in PLAN.md: owner transactional read ports, scheduler
trigger-ID projection ownership, then targeted I/O fault tests. This migration
does not claim deployment qualification.
