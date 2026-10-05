# Plan

Each round is reviewed, focused-tested, linted and committed.

| Round | Scope | Evidence |
| --- | --- | --- |
| P0 | Findings, language, design and plan | Baseline policy/root tests, diff check |
| P1 | Remove the unused capability host; extract pure rules and domain records | Domain boundary tests, policy regressions, lint |
| P2 | Store/app/facade together; required checks, original transaction, callers and ownership gates | Policy/store/facade tests, rollback/fencing regressions, lint |
| P3 | Typed governance documents, named requests, rule/stepdown review and docs | Focused tests, injected gate probes, full race/CI/coverage |

## Preserve

Error precedence and `errors.Is`, stable reason/audit fields, schema/digest
inputs, ID-generator call order, clocks, original transaction and write order,
signature/assertion single-use binding, human approval re-evaluation, calibration
fallback, hourly dispatch accounting, command/outbox idempotency and notification
identities. No schema, protocol or top-level dependency change.

## Deliberate changes

- Remove unused capability-host APIs and their implementation-only tests.
- Replace mutable gateway construction/setters with `New(Config) (*Service,error)`.
  Invalid versions return errors rather than panicking. Missing safety checks are
  constructor errors; supported unowned test/simulation checks are explicit.
- Replace long resolution/evaluation argument lists with named request values;
  update callers instead of adding compatibility shims.

## Status

P0: baseline policy and root tests pass; committed `c9bc465`.

P1: pure domain extraction and unused capability-host removal complete. Focused
policy/root tests pass; domain coverage 94.5%, facade/orchestration 66.9%; lint
passes. Schema/digest/identity, target fallback, expiry and assertion regression
tests preserve the existing boundary semantics. Authority/device comparison
confirmed the facade/app/domain/adapter pattern and caller-owned transaction
adaptation described in DESIGN.md.

## Limits

This is governance structure and behavior preservation, not deployment or
physical-device qualification. Production approval entrypoints and broader
runtime ownership mode design are separate work. Read-only handoff projections
stay on the original transaction; lifecycle writes remain with their owners.

P2: facade/app/store extraction complete. `New(Config)` requires supplied owner,
epoch and interlock checks; production runtime composition binds existing lower
control methods. Evaluation and approval use named request values. Store joins
the original transaction; ledger lifecycle remains delegated to approvalledger.
Dependency, ownership and module-map gates are updated. Focused policy, runtime
and root tests pass, with facade 95.5%, app 67.5%, domain 94.0%, store 81.7%
coverage. Whole-repository lint passes. Transaction identity/rollback, epoch
refusal, command/outbox idempotency, exact prepared-command deletion and approval
ledger/read-projection tests pass on real migrations.

P3: typed governance documents, pure risk/freshness/approval/signature rules,
named evaluation/approval attempts and publication/outcome values complete.
All five injected architecture violations are rejected and removed. Full CI
passes using pinned protoc 35.1; lint autofix reports zero issues. Phase-based
calibration tests are renamed to `calibration_test.go`, with behavior names and
explicit approval-versus-command assertions. Durable module/language guides
now live at the policy package root; this dated folder retains work records.
Full uncached race verification passes. `policy.go` contains only the service
type and two delegation methods; configuration/validation is in `config.go`.
Documentation, focused tests and lint autofix pass after the final file moves.

P4: replace the concrete calibration store dependency with a transaction-scoped
function port, adapting qualification at runtime composition. Remove unused
`Result.WithReason` and its implementation-only test. Move facade delegation
tests into `policy_test.go` and pure export tests into `api_test.go`. Production
and test-inclusive deadcode analyses distinguish the unwired approval workflow
from removable helpers; retain the required approval contract.
