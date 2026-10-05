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

R1: pipeline and effect-routing use cases extracted to app; construction to
composition; source creation to transport; policy/owner/cost transactions to
store; batch report to domain. Public pipeline and effect facades delegate.
Focused runtime/policy/API/CLI and architecture checks pass; lint autofix is
clean. Runtime layers with statements exceed 60% coverage. Pagination and
reconsideration admission regressions remain, with staged ports constructed
explicitly rather than reaching through the public pipeline's private fields.

R2: recovery units of work moved to store; readiness/heartbeat and worker setup
sequencing to app; connection/evidence/TLS/native resources to transport; worker
construction to composition; report and option/routing rules to domain.
Recovery regression tests retain atomic rollback and the ownership claim time.
New tests cover setup order, partial failure cleanup, TLS configuration, resource
closure and loss of readiness. Native-constructor injection is private to
composition tests. Root delegates through the actual production constructor.

Deliberate API reduction: test-only public RecoveryCoordinator and
CompositeEffector are removed. Production routing remains in app, and its
authorization/watch/device/fallback regressions follow that owner. The real
gateway dispatch test stays with device. No live caller or wire contract changes.
Uncached runtime/policy/API/CLI race checks pass, and all runtime layers exceed
60% short-test coverage; domain rules have 100%.
