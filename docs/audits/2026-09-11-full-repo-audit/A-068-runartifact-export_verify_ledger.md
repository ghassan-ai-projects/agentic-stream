# A-068 · `internal/runartifact/export_verify_ledger.go`

LOC: 116 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported ledger row's integrity digest is verified (P2: Verify "checks all checksums", export_verify.go:16-17) — FAIL: three digest-bearing ledgers are silently skipped.
- Unknown ledgers fail loudly rather than pass silently (P2) — FAIL: default case returns nil.
- Digest schemes correctly distinguished (domain-tagged vs raw sha256) (P1) — pass.
- Errors wrapped, digest decoding validated (P1) — pass.

## Findings
- **MED F1. Binding verification silently skipped for digest-bearing ledgers** — `internal/runartifact/export_verify_ledger.go:48-61`. `verifyLedgerRow` returns nil for `observations.jsonl` (`event_log.payload_sha256`, `NOT NULL`, `migrations/001_initial.sql:67`), `device-results.jsonl` (`outcomes.outcome_sha256`, `NOT NULL`, `migrations/001_initial.sql:468`), and `device-command-bindings.jsonl` (`device_command_bindings.command_sha256`, `migrations/030_authority_reconciliation_soak.sql`), and the `default: return nil` also makes any future ledger skip binding checks without a signal. `Verify`'s doc claims it "checks all checksums ... does not trust the manifest", yet 3 of 9 exported ledgers carry unverified internal digests — a tampered or corrupted row in those files passes as long as `checksums.sha256` was regenerated. The raw scheme these tables use (sha256 over the JSON document, cf. `internal/eventlog/log.go:218`) is already implemented as `verifyRawJSONRowDigest`. Fix: extend the switch to those three ledger/digest pairs and make the default case reject unknown ledger names.
- **LOW F2. Near-duplicate digest verifiers** — `internal/runartifact/export_verify_ledger.go:63-81` vs `:83-104`. `verifyRawJSONRowDigest` and `verifyRowDigest` share the fetch-document/decode-digest/compare-bytes shape and differ only in how the expected digest is derived (raw sha256 vs domain-tagged). Collapse into one helper taking a `digest func(map[string]any) ([]byte, error)` to keep the comparison logic single-sourced.

## Checked, not an issue
- P1: all errors wrapped with field/operation context; base64 decoding length-checked to 32 bytes; scanner errors propagated; per-line scanning capped by `maxJSONLLineBytes`.
- P5/P1: domain-tagged digests (`canonicaljson.Digest` + `DecodeDigest`) and raw sha256 correctly kept apart per table; canonicalization performed before both digest paths.
- P4: per-table mapping is a data-free switch on export names, not domain logic.
- P6: `export_verify_ledger_test.go` exists and exercises the verified bindings.
