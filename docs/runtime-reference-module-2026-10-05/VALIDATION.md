# Runtime validation

R1 and R2 focused runtime, policy, API, CLI and root architecture checks passed.
Uncached race checks passed for those packages. Lint used `--fix`; all thresholds
remain unchanged. Short tests cover every runtime layer above the 60% floor.

Temporary runtime probe files demonstrated that domain I/O and clock reads,
application SQL imports, and SQL outside store fail the architecture gates. The
probe files were removed. Existing downward-import, reasoning/replay isolation
and mutation-ownership checks cover the new layers.

Production reachability initially found six unused CompositeEffector facade
functions. Production uses the app router. The facade was removed and its
regressions moved to the owning layer. RecoveryCoordinator is likewise internal.

Self-review checked original transaction/clock/error ordering, watch paging,
reconsideration admission, worker selection, partial cleanup, asynchronous failure
channel ownership and teardown. It found a redundant pending-read error prefix
introduced during extraction; that prefix was removed and exact original read
error text is now tested. Source adapters forward original ingress errors for
app-level context and cancellation classification.

## Final checks

- `PATH=/private/tmp/agentic-stream-protoc-35.1/bin:$PATH make ci-check`: passed.
  This includes build, vet, lint, shuffled uncached short race tests and the
  per-package coverage floor, protocol parity, tidy and documentation checks.
- `golangci-lint run --fix ./...`: zero issues.
- Uncached race tests for runtime, policy, API, CLI and root gates: passed.
- Production analyzer without `-test`, filtered to runtime/policy: zero
  unreachable functions. Analyzer includes the CLI entrypoint under the current
  host/default build configuration; it does not prove every possible build tag.
- Documentation validation: 68 public pages and volatile surfaces passed.
- `git diff --check`: passed.

The Makefile's optional `deadcode` and `govulncheck` binaries are not installed.
Production reachability was checked explicitly using the locally built analyzer;
no vulnerability scan is claimed.

| Runtime layer | Final short race coverage |
| --- | --- |
| Facade | 88.2% |
| App | 80.9% |
| Composition | 76.5% |
| Domain | 100.0% |
| Store | 86.5% |
| Transport | 94.4% |

Policy coverage remains facade 100%, app 82.9%, domain 94.3%, store 82.0%.
Approval tests retain decision-bound signatures, tenant/clock/owner scope,
rollback, concurrency and full policy re-evaluation.

## Behavior-named test follow-up

The old `p8_mode_control_test.go` phase label is replaced by
`dispatch_mode_test.go` and `epoch_control_test.go`, sharing
`runtime_mode_fixture_test.go`. The public documentation link is updated.
Safety regressions remain: fixture rejection/demo admission, active/shadow policy,
recorded epoch, scoped kill and drain/kill admission refusal.

The drain/kill pipeline regressions now ingest a fresh entity after the control
change; the previous kill test reused a duplicate event and could pass through
deduplication. Shadow tests assert one persisted shadow decision and no intents
or commands. The kill test name/comment now states the pipeline behavior it
actually proves, rather than claiming a hostile worker-attempt test.

Full CI passed before this test-only follow-up. The renamed/strengthened tests
then passed uncached with `-race`; lint autofix, docs and diff checks passed again.

## Review rating

Overall runtime score: **8.5/10**, for maintainability and current test evidence.
This is not deployment qualification.

| Dimension | Score | Evidence / limit |
| --- | --- | --- |
| Layering | 9 | Thin facades; downward composition; app has no SQL/network; mutation owners unchanged |
| Domain rules | 9 | Pure option/routing/cadence/shutdown rules; all statements covered |
| Fail-closed safety | 8.5 | Owner/epoch/approval/final effect checks retained; real authority still depends on correct CLI composition |
| Ubiquitous language | 9 | Runtime guide and package language distinguish service, pipeline, source, maintenance and worker resource ownership |
| Tests | 8.5 | Focused regressions plus full shuffled race/coverage gate; some error branches remain uncovered |
| Data encapsulation | 8 | Private use-case/resource fields; explicit configuration and reports; lower opaque payload maps remain |
| Type safety | 8 | Named options/reports/route kinds and typed lower ports; effect payloads retain existing map contracts |
| Simplicity | 8.5 | Layers follow actual responsibilities; no new repositories, lifecycle owners or ports for every plane |

Remaining improvements, in order:

1. Investigate synchronous live-source processing and backlog/fairness behavior
   against the separate maintainability audit before changing scheduling.
2. Narrow opaque effect/trigger payload contracts where the owning module has a
   concrete requirement, while preserving canonical digest and wire inputs.
3. Extend resource-failure branch tests when changing asynchronous server or
   readiness behavior. Current coverage and preserved behavior do not replace
   deployment/physical-HIL qualification.

