# A-041 · `internal/policy/policy_evaluate.go`

LOC: 225 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The risk-class policy document (`R3`/`R4` = denied) is enforced before any approval routing, and an approval resolution can terminate in dispatch or a terminal denial — never in another approval request for the same intent.
- SQL errors other than `ErrNoRows` are propagated, never silently mapped to a benign outcome.

## Findings
- **[HIGH] F1. `requires_approval` shortcut creates an unbounded approval loop and bypasses the R3/R4 denial** — `internal/policy/policy_evaluate.go:194-208`. `routeIntent` checks `row.RequiresApproval != 0 && row.RiskClass != "R2"` BEFORE the risk switch, so:
  1. An R3/R4 intent whose catalog entry declares `requires_approval` enters the approval workflow instead of the documented unconditional denial (`CanonicalDocumentForVersion`, `internal/policy/policy.go:125-134`: `"R3": "denied", "R4": "denied"`; the switch at lines 203-204 is unreachable for it).
  2. Worse, for ANY risk class with `requires_approval` (R0/R1 included), the flow never terminates: `ResolveApproval` approves, resets `policy_status` to `pending` and re-runs `EvaluateIntent` (`internal/policy/policy_approval.go:44-57`), `routeIntent` routes to `requireApproval` again, `pendingApproval` finds no pending approval (the resolved one is `approved`) and creates a NEW approval (`internal/policy/policy_command.go:156-195`). Each human approval spawns another approval request; the intent can never dispatch and every cycle appends approvals, notifications, and audit rows. Only the R2 path consults already-approved approvals (`routeConsequentialIntent`, line 219). The existing test (`rate_limit_test.go:70-90`) asserts only the first evaluation, so the loop is untested. Fix: order the gates as R3/R4 denial first, then `requires_approval` as an override for R0/R1 only, and in that override consult an existing approved approval (mirroring `routeConsequentialIntent`) before creating a new request. Add a test that an approved R0/R1 `requires_approval` intent dispatches, and that R3/R4 is denied regardless of `requires_approval`.
- **[MED] F2. Swallowed SQL errors in `evaluateExisting`/`expireExistingApproval`** — `internal/policy/policy_evaluate.go:55,58,63-67`. Lines 55/58 discard every error from the command/approval lookup (`_ = tx.QueryRowContext(...).Scan(...)`), and lines 65-67 map ANY scan error to "no pending approval, not expired". A real DB failure silently yields an audit row with a missing `command_id`/`approval_id` or a wrong "already_evaluated" result. Fix: propagate all errors except `errors.Is(err, sql.ErrNoRows)`.

## Resolution (2026-09-11) — FIXED
- **F1 (HIGH)** fixed: `routeIntent` now switches on risk class first — R3/R4 are denied unconditionally (`risk_policy_denied`) regardless of the `requires_approval` flag. `requires_approval` is an override only for the R0/R1 tier, routed through the new `approveOrRequireApproval`, which consults an already-`approved` approval and dispatches (mirroring `routeConsequentialIntent`) before creating a new request — breaking the approve → re-pending → new-approval loop. `routeConsequentialIntent` shares the same helper.
- **F2** fixed: `evaluateExisting` (command/approval lookups) and `expireExistingApproval` now propagate every error except `sql.ErrNoRows` instead of silently mapping DB failures to a benign outcome. The shared `approveOrRequireApproval` also propagates non-ErrNoRows from the approved-approval probe (previously swallowed).
- Tests: `TestEvaluateIntentDeniesHighRiskDespiteRequiresApproval` (R3/R4 + requires_approval → denied) and `TestEvaluateIntentApprovedRequiresApprovalDispatches` (approved R1 requires_approval dispatches with exactly one approval row, no loop).
- Verified: `go build ./...` and `go test ./internal/policy/` pass.

## Checked, not an issue
- P1: all remaining errors wrapped with `%w`; contexts honored; serial tx.
- P2: the pending gate otherwise revalidates everything against current durable state immediately before routing — validation status, decision/intent schema, digest match, cross-row identity, compensation target tenancy, episode conclusion, situation-version staleness, source-health completeness, expiry — this is the strongest part of the file.
- P3: helpers (`decodeDocument`, `matchesDecisionIdentity`, `matchesIntentIdentity`, `episodeConcluded`, `sourceHealthIncomplete`, `markStale`) all used, single-purpose; no dead code beyond F1's ordering.
- P4: no cognition leakage; calibration is a data-driven artifact check, not a domain branch; the calibration error falling back to approval (211-217) is fail-safe and acceptable.
- P5: rejection reasons map to the durable reason registry; canonical digest verification via `canonicalDocumentMatches`.
- P6: `policy_test.go` covers R3 denial, R2 approval, staleness, digest mismatch — but not the `requires_approval` re-entry loop (gap noted in F1).
- P7: evaluation is a pure function of durable rows + `now`; deterministic.
