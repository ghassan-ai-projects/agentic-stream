# Branch clean-function completion

Date: 2026-10-02. Branch: `code-improvments`.
Status: in progress.

## Scope and acceptance

Finish the existing branch refactoring, including the uncommitted cognition,
ingress, notification, operator, and Situation extractions. Review affected
production entry points against Q7, and close the remaining whole-tree lint
findings exposed by the stricter Q2 limits. Keep signatures, ordering, identity
and digest inputs, clock reads, error precedence, transactions, cancellation,
and effect boundaries unchanged. Generated files and domain data are excluded.

The five clean-function principles are recorded in `AGENTS.md` and Q7. Passing
mechanical thresholds is necessary; it does not replace reading the domain
steps. Existing golden replay and predictive-maintenance tests remain unchanged.
Each round includes focused regression tests, diff review, and a commit. Final
validation uses the full repository gate and verifies the pushed branch ref.

## Delivery rounds

1. Finish ingress and notification extractions, split SSE delivery from subscriber
   lifecycle, and make the agent review requirements concrete.
2. Finish cognition, operators, and immutable Situation materialization.
3. Refactor runtime composition, ingestion, and cost admission.
4. Refactor replay setup and recorded/shadow comparison.
5. Refactor CLI composition and worker/provider transport.
6. Validate the final tree, close review findings, and push.

## Evidence

- Starting HEAD: `97821b2`; three existing local commits were awaiting push.
- Starting whole-tree lint: 20 issues (five length/statement limits and fifteen
  cognitive-complexity limits). These are completion work, not waived debt.
- Starting five-package race tests passed with local socket access. The restricted
  sandbox prevented the ingress deadline test from binding its Unix socket.
- The root-level untracked `agentic-stream` executable predates this work and is
  excluded from commits.

### Round 1: ingress and notifications

Completed the pending line admission, simulator record, live connection, cursor
allocation, and subscriber lifecycle extractions. Split SSE framing/delivery into
its own file to keep production files below 300 lines. Added regression coverage
for bounded-line resynchronization and EOF, duplicate/conflicting notification
payloads without cursor gaps, and subscriber problem-response precedence.

Validation: ingress/notification race tests, package vet, documentation checks,
and diff whitespace checks passed. Diff review retained admission order,
checkpoint advancement, subscriber authorization, bounded state, and shutdown
ownership. The remaining baseline lint findings are assigned to later rounds.

### Round 2: cognition, operators, Situations

Completed the pending trigger, supersession, reconsideration, boot fencing,
heartbeat, window configuration, aggregate, and publication extractions.
Cognition entry points now separate trigger evaluation from admission bookkeeping.
Reconsideration evidence is canonicalized before writes, preserving the original
failure boundary. Window aggregate selection uses an explicit switch rather than
adding a mutable global function registry.

Added tests for trigger gate precedence and partial scores on evaluation errors,
all numeric aggregates and empty-input precedence, canonical evidence ordering,
private event-time persistence, and publication independence from later state
mutation. Package race tests, vet, and diff checks passed. Existing correction
replay/deduplication tests passed unchanged.

### Round 3: runtime and stream transactions

Separated pipeline defaults, effector composition, cognition/policy/dispatcher
wiring, episode execution, command dispatch, and cost-limit configuration.
Separated transactional record application and global batch accounting. Preserved
owner checks, transaction boundaries, rollback restoration, processing order,
partial reports, and telemetry timing.

Added cost-configuration tests for unspecified values, kill-switch semantics,
and rollback of global changes when tenant configuration fails. Added a global
run failure/resume test that pins inbox deduplication and the existing report
semantics (global runs count redelivered records while suppressing state writes).
Runtime and engine race tests, focused lint, vet, and diff checks passed.

### Round 4: replay and paired shadow comparison

Separated isolated replay preparation, ingress, virtual-clock advancement,
processing, and result collection. Separated recorded-ledger matching, baseline
parameter completeness, paired executor validation, and canonical comparison
construction. Replay still has no action-plane import or effector capability.
Schema registration precedes clock derivation and ingestion as before.

Added comparison regression tests for decision/manifest difference order,
manifest-only differences preserving decision equality, canonical digest and
comparison identity, and independence from recording wall time. Replay race
tests (including unchanged predictive-maintenance golden traces and thermal
fixtures), focused lint, vet, and diff checks passed.

### Round 5: CLI and worker/provider transport

Separated CLI validation, telemetry/worker-monitor startup, trace execution, and
artifact export from flag registration. Separated evidence-socket preparation,
worker execution deadlines/status handling, provider request construction,
response classification, and SSE tool-delta accumulation. Preserved status codes,
wire-error identity, deadline order, reverse cleanup, request/stream limits,
retry classification, and sorted tool-call indices.

Added regression tests for CLI path rejection before state creation, worker
parent/request/budget deadlines and cancellation, gRPC status preservation,
fragmented tool arguments with out-of-order indices, usage-only chunks, DONE
termination, retryable provider statuses, and bounded error bodies. Package race
tests and vet passed with local socket access. Whole-tree lint now reports zero
issues, without weakened thresholds or complexity suppressions. Diff checks pass.
