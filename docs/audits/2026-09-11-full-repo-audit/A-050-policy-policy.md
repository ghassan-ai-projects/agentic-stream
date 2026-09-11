# A-050 · `internal/policy/policy.go`

LOC: 182 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The policy digest recorded in every audit row and command document is guaranteed non-empty, or construction fails.
- Every exported symbol has at least one consumer outside its own file.

## Findings
- **[MED] F1. `newGateway` swallows the policy-digest error** — `internal/policy/policy.go:142-148`. `policyDigest, _ := DigestForVersion(policyVersion)` discards the error (returned only when `policyVersion == ""`). A gateway built with an empty version then writes an empty `policy_digest` into every `policy_evaluations` audit row (`internal/policy/policy_store.go:71-83`) and every command document (`internal/policy/policy_command.go:101`), silently breaking the digest-binding that exporters verify (`internal/runartifact/export_snapshot.go:247`). Fix: validate `policyVersion` in `NewGateway`/`NewGatewayWithOwner` (panic or return an error) so an unusable gateway cannot be constructed.
- **[MED] F2. `CanonicalApprovalAssertion` is exported with no external consumer** — `internal/policy/policy.go:48-62`. Repo-wide grep finds it only in this file (called by `ApprovalAssertionSigningBytes`, line 66); no other package or test uses it. Speculative API surface on a security-sensitive type. Unexport it (`canonicalApprovalAssertion`).

## Checked, not an issue
- P1: no other swallowed errors; struct is immutable after construction.
- P2: the type system is right — the Gateway holds no effectors; `ApprovalAssertion` reconstructs bytes from durable rows for verification; builders (`WithCalibration`, `WithEpochControl`, `WithInterlock`) are all consumed (`internal/runtime/pipeline.go:142`).
- P3: `NewGateway`, `NewGatewayWithOwner`, `DigestForVersion`, `CanonicalDocumentForVersion`, `Result.WithReason` all have live callers; `intentRow` fields all scanned in `loadIntent`.
- P4: clean boundary — policy owns only the accepted-Intent to Command authorization; dependencies point downward (ids, canonicaljson, interlock, storage).
- P5: policy document is canonical JSON digested (RFC 8785); all exported symbols documented.
- P6: `policy_test.go` uses both constructors and the assertion signing bytes; `go test ./internal/policy/` passes.
- P7: digest is a pure function of the canonical policy document.
