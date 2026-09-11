# A-052 · `internal/storage/runtime_owner.go`

LOC: 178 · Audit date: 2026-09-11 · Verdict: PASS

## Bar (close only when every line is true)
- Lease takeover only when the incumbent lease has expired; same epoch/instance renews in place.
- Recovery runs inside the claim transaction; recovery failure rolls ownership back.
- Assert is a transaction-scoped write fence; expired or foreign epochs are refused.
- Errors are sentinels (`ErrRuntimeOwnerBusy`) usable with `errors.Is`; timestamps compare correctly as SQLite TEXT.

## Checked, not an issue
- P1: `claimTx` upsert guards takeover with `lease_until <= excluded.heartbeat_at` and re-reads the row to return `ErrRuntimeOwnerBusy`; `Renew`/`Release` are single-statement guarded updates with `RowsAffected` checks; all errors wrapped `%w`; contexts honored.
- P2: `Assert` is the fencing predicate used inside engine/action transactions; lease expiry is enforced on every assert (`lease_until > now`).
- P3: no dead code — `Claim`, `ClaimAndRecover` (used by `internal/runtime/recovery.go`), `Renew`, `Release`, `Assert` all grep-verified in production paths; no speculative abstraction.
- P4: matches `migrations/011_runtime_owner.sql` exactly (singleton CHECK, STRICT table).
- P5: exported symbols documented; injectable `Now`/`Lease` for deterministic tests; `formatRuntimeTime` fixed-width nanoseconds documented for TEXT ordering.
- P6: `runtime_owner_test.go` covers claim/renew/release, unexpired rejection (incl. same-epoch different-instance), expired takeover, expired assert refusal, and claim+recover rollback.
- P7: recovery-in-transaction (`ClaimAndRecover`) prevents committing ownership without completed recovery; test pins rollback.
