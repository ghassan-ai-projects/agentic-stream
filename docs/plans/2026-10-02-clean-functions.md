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
