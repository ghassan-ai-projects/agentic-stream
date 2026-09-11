# A-034 · `internal/evidence/capability.go`

LOC: 284 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every declared field on the issuer/verifier types is read by the signing or verification logic.

## Findings
- **[MED] F1. `Issuer.ClockSkew` is a dead field** — `internal/evidence/capability.go:57`. Declared on `Issuer` and never read: `Issue` (62-127) uses `MaxTTL` and `Now` only; only `Verifier.ClockSkew` is consumed (182-185). A deployer tuning issuance skew believes they configured something that does nothing. Fix: delete the field, or apply it in `Issue` when validating `NotBefore`/`IssuedAt` symmetrically with `Verify`.
- **[LOW] F2. `Verifier.ClockSkew == 0` silently means 1 second** — `internal/evidence/capability.go:182-185`. Zero is indistinguishable from unset, so a verifier cannot run with strictly zero skew; a misconfigured `time.Nanosecond` is equally overwritten. Fix: use `*time.Duration` or document that 0 is a sentinel for the 1s default.

## Checked, not an issue
- P1: parse/signature failures fail closed; no swallowed errors; no I/O, no races (value types only).
- P2: tokens are HMAC-SHA256 over version+keyID+payload with constant-time compare (`subtle.ConstantTimeCompare`, 153); keys must be ≥32 bytes (93, 147); verifier re-runs the full `validateScope` on decoded claims (258) so a hand-malleated-but-re-signed payload still cannot widen scope; TTL capped on both sides (105-107, 179-181); token carries no secrets and grants no effectors/filesystem/network — the scope struct is the read-only evidence boundary; `tokenID` uses `crypto/rand` (269).
- P3: no duplicated logic — issue and verify share `tokenPayload`/`validateScope`.
- P4: pure crypto/claims package, no dependencies beyond stdlib.
- P5: exported symbols documented; errors wrapped `%w`.
- P6: exercised via `server_test.go` and `uds_integration_test.go` (happy path and rejection paths).
- P7: all clock reads through injectable `Now`; validity-window math deterministic.
