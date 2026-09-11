# A-029 · `internal/evidence/ledger.go`

LOC: 298 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- No dead exported symbols.
- Lease comparisons are chronologically correct, not dependent on string collation of a variable-width timestamp format.
- Reserve and Complete apply the same tenant scoping to the same tables.

## Findings
- **[MED] F1. `Ledger.Recover` is dead code** — `internal/evidence/ledger.go:224-233`. Repo-wide grep finds zero callers: runtime recovery uses `RecoverTx` (`internal/runtime/recovery.go:58`) and startup reclamation uses `ReclaimExpired` (`internal/runtime/service.go:117`). Three near-identical entry points for the same reclaim semantics is exactly the duplication that diverges. Fix: delete `Recover` (or make `ReclaimExpired` unexported and keep one public path).
- **[LOW] F2. Lease windows compared as RFC3339Nano strings, which is not order-preserving** — `internal/evidence/ledger.go:113,157,186,210` with `formatLedgerTime` at 298. RFC3339Nano trims trailing fractional zeros, so `2026-01-01T00:00:00Z` sorts lexicographically greater than `2026-01-01T00:00:00.5Z` (`Z` > `.`). When a lease deadline lands exactly on a whole second, `lease_until > ?` (Complete, 157) can pass after chronological expiry and `lease_until <= ?` (ReclaimExpired, 210) can skip an actually-expired lease. Probability is low (nanos must be exactly 0 at the boundary second) but the fix is mechanical. Fix: store/compare a fixed-width format (always 9 fractional digits) or integer epoch nanos.
- **[LOW] F3. `Complete` looks up the episode without the tenant filter `Reserve` uses** — `internal/evidence/ledger.go:145` vs `71`. Correct today because `episode_id` is a global primary key (`migrations/001_initial.sql:293`), but the two transaction bodies should scope identically so a future key change cannot silently cross tenants. Add `AND tenant_id = ?`.

## Checked, not an issue
- P1: every error wrapped `%w`; rows/scans checked; `Complete`/`Fail` deliberately detach onto `context.Background()` with a 5s timeout (137, 176) so result persistence survives an aborted request ctx — intentional and bounded.
- P2: ledger is append-only in effect — status transitions are single-guarded `UPDATE ... WHERE status='running'` with owner/token/epoch/lease predicates (157, 186); stored results are tamper-evident: sha256 stored at write and re-verified with byte-length and row-count sanity checks on every replayed read (99-105); capability bytes never persisted; completed calls are replayed from the ledger without re-invoking the provider (146-148).
- P3: fingerprint pins the full request identity so a reused call ID with different arguments is rejected (91-93); no speculative abstraction beyond F1.
- P4: pure infrastructure; depends only on `internal/storage`; runtime epoch fencing flows downward.
- P5: exported symbols documented; `Now` injection everywhere except the ctx-detached paths which also honor it.
- P6: `ledger_test.go` + `uds_integration_test.go` cover reserve/complete/fail/reclaim and replay.
- P7: all time through `l.Now` injection (51-54, 133-136, 178-181, 228-231); identities stable (`tenant, episode, attempt, fence, call_id` key).
