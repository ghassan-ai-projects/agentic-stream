# Architecture ownership improvement rounds

Baseline: `4b58e44`, branch `code-improvments-2`. Pre-existing untracked
`agentic-stream` executable is outside this work. No push is requested.

## Plan

1. Commit the accepted design and measurable architecture bar before extraction.
2. Move runtime control, device authority and qualification out of storage.
3. Separate effect ports, governed dispatcher and concrete device adapters.
4. Route episode/scheduler lifecycle mutations through owning ledger modules.
5. Enforce layer ordering, SQL ownership, contract isolation and transitive safety.
6. Review evidence against A1–A7, fix findings in committed rounds, and run full gates.

Mechanical moves carry existing regression suites with their owning modules.
Tests for changed boundary contracts are added in the same round; no behavior
change is intended, so unchanged acceptance fixtures remain the reference.

## Rounds

- Round 0: accepted ADR-017, design refinement and architecture bar. Implementation pending.

- Round 1: extracted `control`, `authority`, and `qualification`; `storage` now imports only migrations. Migrated authority/lease/epoch tests and added shadow transaction rollback, duplicate-evidence and inert-action checks. Focused race tests passed across nine affected packages; full-tree lint reports zero issues. New package coverage: control 79.9%, authority 63.5%, qualification 80.0%; storage 80.6%. Cross-package errors retain their original text and unwrap causes. A4 lifecycle ownership and A3 adapter isolation remain pending.

- Round 2: extracted the `actionport` boundary and `device` adapters; moved concrete route composition to `runtime`. Dispatch imports no device implementation. Added unknown-outcome cause preservation, denied/missing authorization, unavailable-device fail-closed routing and verification-routing tests. A typed nil device pointer is not boxed into the composition's interface. Focused race/coverage passed: actions 68.9%, actionport 100.0%, device 79.6%, runtime 67.8%, CLI 68.5%; final focused tests and whole-tree lint pass. Lifecycle ownership enforcement remains pending.

- Round 3: `episodeledger` exclusively owns episode, attempt and rejection mutations. Cognition supersession and epoch kill call its concrete transaction-scoped APIs. Execution retains its request assembly, budget, telemetry and provider responsibilities. Added admission/shadow-default and live-conflict tests, composed mutation rollback, cancellation recovery, and idempotent unknown-worker rejection audit. Focused tests/race tests pass across lifecycle, execution, control, cognition, runtime, native/worker conformance and replay; zero lint issues. Episode ledger coverage 63.2%, episodes 72.4%. Queue and approval lifecycle ownership remain pending.

- Round 4: `scheduleledger` owns queue admission, coalescing and skipped/cost-rejected transitions; `approvalledger` owns request/assertion/decision/expiry/supersession state. Approval permission and signature checks remain in policy. Runtime control now supplies the read-only final authorization capability, eliminating a dispatcher callback. Added queue conflict identity/rollback, pending-only skip, terminal approval/assertion binding, atomic withdrawal notification, and current-interlock/read-only/misconfiguration regressions. Focused race/final tests and full-tree lint pass; coverage schedule ledger 77.6%, approval ledger 69.4%, control 80.7%. ADR-017 records the discovered approval boundary. Next round pins ownership and transitive reachability mechanically.

- Round 5: pinned strictly downward dependency levels, transitive reasoning/replay effect isolation, contract purity, control-only final authorization construction, and all current SQL mutation owners. Shared intent/command/outbox/Situation handoffs are restricted by operation and columns; prepared-command cleanup is restricted to its exact pending identity predicate. Added negative bypass tests and SQL classifier cases for CTEs, quotes, comments and upserts. The scan exposed runtime's trigger cost-refusal audit write; cognition now owns it, with rollback/queue-isolation proof. Repaired the three stale documentation links exposed after extraction and published the complete module/ownership map. Architecture tests, focused cognition/runtime race tests, zero lint and docs (59 pages) pass. Final semantic/Q7 review and full gates remain pending.

- Round 6: completed the readability review of the new admission, queue, approval supersession and calibration boundaries. Entry points now delegate value preparation, ordered row processing and SQL to their owning steps. Preserved streamed withdrawal order, per-row clock reads, error prefixes, queue-conflict retry inputs and calibration transaction scope. Added regressions for clock reads after ordered withdrawals, rollback of all withdrawals/notifications on a later publication failure, and failed replacement retaining active calibration. Focused race tests across the four ledgers/qualification and root architecture tests pass; full-tree lint reports zero issues. Final CI and uncached full race gates follow.

- Round 7: challenged the SQL ownership guard with producer replacements/upserts and mixed scalar/tuple payload updates. These bypass forms are now rejected for shared handoffs; normal inserts and conflict-ignore behavior remain accepted. Added positive/negative classifier and ownership regressions. Focused root race tests and zero-issue lint pass. The initial full CI and uncached race gates passed but began before this test-only guard change; final gates are rerun after its commit.

## Final acceptance: A1–A7 met

Validated code revision: `702fa07`. The final follow-up changes only this evidence
record; documentation validation and diff checks are repeated before committing it.

| Criterion | Accepted evidence |
| --- | --- |
| A1: named responsibilities and downward imports | [Module map](../../documentation/architecture/modules.md), pinned [approved dependencies](../../architecture_test.go) and [strict layer checks](../../architecture_flow_test.go); unclassified, upward and sideways imports fail. |
| A2: infrastructure and contract isolation | Storage imports only migrations internally. Contract persistence/transport exclusions and lower-layer imports pass. Control, authority and qualification now own their business mutations. |
| A3: effect ports separate from adapters | Actions cannot import device. Device cannot reach policy, dispatch or episode execution. Composition routing tests preserve final authorization and forbid fallback on missing device routes. |
| A4: durable ownership | [Mutation guard](../../architecture_ownership_test.go) covers all 50 currently mutated tables; four shared handoffs have explicit operation/column contracts. New ledger tests cover transaction rollback, identity, admission, terminal state and recovery. Replacements/upserts and unsupported tuple handoff updates are rejected. |
| A5: forward data and downward control | Transitive reasoning/replay isolation checks pass. Final readiness is constructed only by lower control, remains read-only, and observes current interlock state. Feedback uses returned values or durable evidence/outcome records. |
| A6: preserved runtime boundaries | Full replay/golden, worker, cancellation/rebind, policy/action and device safety suites pass. Tests retain epoch kill atomicity, safe-stop evidence, unknown outcomes and clock/order boundaries. Protocols, migrations, dependencies, domain catalogs and example/golden fixtures are unchanged from `4b58e44`. |
| A7: reviewed rounds and full gates | Every round has focused tests/review evidence and a commit. Both final full gates pass on `702fa07`, including the new package coverage floor. |

### Validation

- `make ci-check`: passed. Protobuf parity, dependency tidiness, build, vet,
  full-tree lint, shuffled short race/coverage, deadcode, vulnerability and
  documentation checks passed. Lint: zero issues. Vulnerabilities: none found.
  Documentation: 59 public pages and volatile surfaces verified.
- `go test -race -count=1 ./...`: passed, 43 tested packages; includes the full
  integration and replay suites. Generated protobuf has no handwritten tests.
- All handwritten packages meet the unchanged 60% coverage floor. New packages:
  actionport 100.0%, device 80.8%, episodeledger 63.1%, scheduleledger 78.0%,
  approvalledger 73.9%, control 80.7%, authority 63.2%, qualification 83.9%.
- `git diff --check`: passed. No protocol, migration, dependency, domain-data or
  example fixture changes. No complexity thresholds or coverage floors loosened.
- `pre-commit` is unavailable; the constituent full CI checks above passed.
- Host verification: Go 1.27.1 on darwin/arm64; pinned protoc 35.1. Local socket
  integration tests ran with sandbox approval; no external effects were exercised.

### Commits

| Round | Commit |
| --- | --- |
| 0: architecture bar and accepted design | `fda66e5` |
| 1: control, authority and qualification | `ad058cf` |
| 2: ports, adapters and composition | `bb5e223` |
| 3: episode lifecycle ownership | `11d895d` |
| 4: queue, approval and final readiness | `a0980fe` |
| 5: architecture enforcement and module documentation | `d989ecc` |
| 6: semantic/readability review fixes | `12e7e32` |
| 7: adversarial handoff guard cases | `702fa07` |

### Scope and limits

This accepts the architecture bar for the current modular monolith. The import
and SQL guards are source-level checks, complemented by behavioral tests. They
do not certify every possible future implementation, physical hardware or
deployment readiness. Shared transactions and cross-module evidence reads remain
intentional; writes and imports now have enforced narrower authority. No new
service, queue, database, external dependency or public protocol was introduced.
The pre-existing untracked executable remains untouched. Commits are local;
no push was requested.
