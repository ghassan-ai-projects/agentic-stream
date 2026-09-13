# A-054 · `internal/policy/policy_approval.go`

LOC: 177 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- SQL statements contain no no-op assignments or dead clauses.
- (Cross-reference: the approval re-entry path terminating in a new approval instead of dispatch is filed as HIGH F1 in `A-041-policy-policy_evaluate.md`; `ResolveApproval` line 57 is the re-entry point.)

## Findings
- **[LOW] F1. No-op SQL assignment `nonce = nonce`** — `internal/policy/policy_approval.go:163-165`. The UPDATE that records `assertion_sha256` also sets `nonce = nonce`, a literal no-op with no effect on conflict or locking behavior (the `assertion_sha256` write already mutates the row). Dead clause that suggests an intended binding that does not exist. Remove it.

## Checked, not an issue
- P1: errors wrapped with `%w`; uniform authorization-failure messages are deliberate (no oracle); contexts honored; serial tx.
- P2: strong — approval resolution re-runs the FULL policy gate before any Command (`ResolveApproval` → `EvaluateIntent`, lines 44-57); stale-situation withdrawal before acceptance; expiry checked; signature verified over canonical assertion bytes reconstructed from durable rows; nonce re-read from the pending row; relay/approver must be distinct active principals with an ed25519 key; authority scoped per tenant+entity+risk class; single-use enforced by `WHERE status = 'pending'`.
- P3: `approvalRow`, `loadApproval`, `approvalExpired`, `approvalDecision`, `updateApproval`, `denyUnauthorizedApproval` all single-use, single-purpose; no duplicated logic within the file (withdrawal duplication with cognition is filed under A-014 F3).
- P4: policy plane owns approval state; cognition's parallel withdrawal is the structural concern tracked in A-014.
- P5: assertion bytes are canonical JSON with domain separation (`canonicaljson.DomainApproval`); exported symbols documented.
- P6: `policy_test.go:130-200` covers approval resolution including an unauthorized-signature denial; `go test ./internal/policy/` passes.
- P7: nonce and assertion digest derived deterministically from durable identities; decisions depend only on row state + `now`.
