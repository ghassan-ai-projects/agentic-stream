# A-004 · `internal/storage/authority.go`

LOC: 836 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported symbol has at least one production caller (grep-verified).
- No duplicated authority-assertion or ownership predicates that can diverge.
- Claim/Assert/Release transactions are atomic, fail-closed, and errors wrapped %w.
- Reconciliation barrier cannot silently regress (boot-bound, resolution fields reset on boot change).
- Schema usage matches migrations/030_authority_reconciliation_soak.sql.

## Findings
- **[MED] F1. SafetyLedger/SafetyEvent/Record are a dead public writer API** — `internal/storage/authority.go:706-755`. Grep across `cmd/` and `internal/` (non-test) shows no caller of `SafetyLedger`, `SafetyEvent`, or `Record`; only `authority_test.go` uses them. `internal/soak/report_compute.go` and `internal/runartifact/export_snapshot.go` read `device_safety_events`, but nothing in production writes it. Either wire it (CLI/evidence bridge entry point) or delete it until a caller exists (AGENTS.md: no speculative surface).
- **[MED] F2. Redundant double authority assertion in BindState and Resolve** — `internal/storage/authority.go:386-392` and `internal/storage/authority.go:522-537`. Both call `AssertRuntime` (its own transaction) and then `assertOrdinaryTx` again inside the following `WithTx`. The first assert guards nothing (a second, in-transaction check follows and is the only one covered by the write); it doubles round trips and implies the first check is load-bearing when it is not. Drop the pre-transaction `AssertRuntime` calls.
- **[LOW] F3. Ownership predicate duplicated** — `internal/storage/authority.go:92` and `internal/storage/authority.go:128`. `existing.AuthorityEpoch == claim.AuthorityEpoch && existing.OwnerInstance == claim.OwnerInstance` appears twice with different extra conditions (`expires`/`status` vs `status` only). Extract one predicate; the current shape already produced the F4 mislabel.
- **[LOW] F4. Expired same-owner reacquisition is recorded as `claim_renewed`** — `internal/storage/authority.go:103-131`. When the row is owned but `!expires.After(now)`, the fence is incremented (takeover semantics) yet the event type is still `claim_renewed` because the type check ignores lease expiry and `boot_id` changes. Audit trail mislabels a fence-bumping takeover as a renewal.
- **[LOW] F5. ErrTargetClaimBusy has no consumer** — `internal/storage/authority.go:18`. Grep-verified: no caller distinguishes it via `errors.Is` (unlike `ErrTargetClaimNotOwned`/`ErrReconciliationRequired`, which are consumed in `internal/actions/`). Sentinel is exported API without a user.
- **[LOW] F6. Four unrelated concerns in one 836-line file** — `internal/storage/authority.go`. `TargetAuthority`, `ReconciliationStore`, `SafetyLedger`, and evidence validation (`ValidateDeviceReconciliationEvidence`/`VerifyStoredJSONDigest`) are distinct responsibilities; the `now()`/`leaseDuration()` clock-indirection pair is also copy-pasted three times across `TargetAuthority`, `ReconciliationStore`, and `RuntimeOwner`. Split per concern.

## Checked, not an issue
- P1: all transactions via `WithTx` (rollback on error); errors wrapped `%w`; contexts threaded into every `ExecContext`/`QueryRowContext`; immediate txlock closes SELECT/UPSERT races.
- P2: claims and barriers are durable in SQLite, not memory; `Resolve` binds evidence to latest state digest and refuses while unresolved commands exist; `RequireAfterAuthorityLoss` deliberately skips the owner fence and documents why; raw evidence never becomes instructions.
- P4: all tables/columns match `migrations/030_authority_reconciliation_soak.sql`; `commands.status` values match the 001 CHECK.
- P5: exported symbols documented; `context.Context` first param; canonical JSON used for all stored blobs.
- P6: `authority_test.go` covers takeover, barrier persistence across restart, manual review, authority-loss barrier, and cross-device isolation.
