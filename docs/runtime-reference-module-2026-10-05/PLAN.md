# Plan

| Round | Scope | Proof |
| --- | --- | --- |
| R0 | Survey and layer design | Approval baseline full CI; reviewed public/private callers |
| R1 | Reports/rules, transaction/source adapters and pipeline facade | Runtime/policy/API/CLI tests, original transaction and source behavior |
| R2 | Recovery/readiness, worker transport and facade; route classification | Atomic recovery, claim time, native/remote selection, teardown and fencing tests |
| R3 | Package guides, layer gates, coverage, self-review | Lint autofix, race tests, production reachability, full CI and diff checks |

Preserve error precedence and identities, clocks and clock call counts,
transaction ownership and commit order, source processing and watch paging,
owner/epoch fences, calibration/approval flow, effect routing and idempotency,
heartbeat cancellation, asynchronous failure reporting and teardown order.

Tests and implementation move together because existing private tests must
follow their extracted owners. Add targeted boundary regressions in the same
round. Do not fix the separately audited synchronous-source/backlog concerns as
part of this behavior-preserving migration.

R0: design recorded against `e8f33a0`; no runtime migration code changed yet.
