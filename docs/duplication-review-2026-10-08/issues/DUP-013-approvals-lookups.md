# DUP-013: pending and latest-approved approval lookups are written in policy and actions

- Status: fixed
- Severity: medium
- Verdict (finders): REAL
- Themes: persistence
- Wave: 2
- Finder sources: P8 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `approvalledger`. Pin the tie-break for two approved rows sharing a `decided_at`, and make actions read one row rather than two independent subqueries.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P8: `approvals` is queried with status literals by policy and actions although approvalledger owns it

- Verdict: REAL
- Shared meaning: the pending approval of an intent, and the latest approved approval of an intent.
- Sites:
  - internal/policy/internal/store/approval_lookup.go:15 pending (id, expires_at) by intent; pending_approval.go:12 pending id by intent (same predicate, narrower projection, both in policy); approval_lookup.go:27 latest approved id `ORDER BY decided_at DESC LIMIT 1`; approval_lookup.go:46 pending (expires_at, nonce) by approval id; approval_reads.go:15 approval joined to intents.
  - internal/actions/internal/store/authorization.go:19-20 two scalar subqueries on `approvals` with the same `status='approved' ORDER BY decided_at DESC LIMIT 1` (one for approval_id, one for expires_at; two independent subqueries may pick different rows on a `decided_at` tie).
  - Owner writes: internal/approvalledger/internal/store/approval.go:10-38 and read view approval_views.go:17.
  - Go-side vocabulary: approvalledger/internal/domain/approval.go:3-7 exports only pending/expired/denied; "approved" is not a constant anywhere, policy uses the literal in domain/rules.go (`ApprovalDecision`) and SQL.
- How they differ: no divergence yet, but two modules each hold their own copy of "latest approved" and "pending"; the status word `approved` exists only as scattered literals.
- Risk if left: renaming/adding a status or changing tie-breaking ("latest approved") would need edits in 3 modules; actions authorization could bind to a different approval than policy evaluated.
- Proposed canonical owner: `internal/approvalledger` (layer 13; policy store 20 and actions store 23 are above it; policy already imports it, actions/internal/store needs a new allowedImports edge `-> internal/approvalledger`).
- Proposed fix: add `approvalledger.PendingFor(tx, intentID)`, `ApprovedFor(tx, intentID)` returning `(id, expiresAt)` in one statement, add `StatusApproved`; replace policy's 4 selects and actions' two subqueries (join one row: `LEFT JOIN` or one lookup) with them.
- Behaviour to preserve: `one_pending_approval_per_intent` semantics, error text "approved intent has no approved approval record", approval expiry check at dispatch (actions/internal/domain/authorization.go:79-86).
- Verification: policy/internal/store/reads_test.go and transaction_test.go, actions store_test.go; new test with two approved rows sharing a `decided_at` to pin the chosen row.

## Outcome

Commit: pending (reviewer commits)

Verified:
- Confirmed: policy held three separate `approvals` selects by intent (pending id, pending id+expiry, latest approved id) plus the pending expiry/nonce read by approval id; actions held two independent scalar subqueries for the latest approved id and expiry. All used `status` literals and `ORDER BY decided_at DESC LIMIT 1` with no tie-break.
- Confirmed latent defect: with two approved rows sharing a `decided_at`, the two actions subqueries were free to pick different rows, binding the id of one approval to the expiry of another.
- Left alone: `LoadApproval` in policy (joins `intents` for the tenant check and reads `approval_json`; a policy projection, not a duplicate lookup).

Changed:
- `internal/approvalledger`: new facade reads `PendingOfIntent`, `LatestApprovedOfIntent` (one statement, `ORDER BY decided_at DESC, approval_id DESC`, returns id and expiry from one row) and `PendingBinding`; `domain.StatusApproved`, `domain.Approval`, `domain.AssertionBinding`; store `approval_lookup.go`, app `approval_lookup.go`. All three queries bind the status as a parameter from the domain constants.
- `internal/policy/internal/store/approval_lookup.go`: `PendingApproval`, `PendingApprovalExpiry`, `ApprovedApproval`, `AssertionBinding` now delegate to the ledger (signatures unchanged); `pending_approval.go` deleted.
- `internal/actions/internal/store/authorization.go`: the two subqueries removed from `loadAuthorizationRecordsSQL`; `LoadAuthorizationRecords` reads the approval once through `approvalledger.LatestApprovedOfIntent` on the same transaction. Error text "approved intent has no approved approval record" and the expiry check in actions domain are untouched.
- `architecture_test.go`: allowedImports edge `internal/actions/internal/store -> internal/approvalledger` (layer 23 over 13, legal). `architecture_approvalledger_test.go`: the three new facade operations added to `approvalLedgerOperations`.
- `internal/approvalledger/UBIQUITOUS_LANGUAGE.md`: two rows for the new terms.

Decisions: the tie-break is the larger `approval_id` (deterministic, independent of insertion order). Behaviour change is limited to the previously undefined tie case.

Pinning tests: `TestLatestApprovedOfIntentPicksOneRowAndBreaksDecidedAtTiesByLargerID`, `TestPendingLookupsReadTheUnresolvedApprovalOnly` (approvalledger), `TestAuthorizationRecordsBindOneApprovedApprovalWhenDecisionsTie` (actions store). Existing policy tests (`TestApprovalLedgerAndReadProjections` etc.) pass unchanged.
