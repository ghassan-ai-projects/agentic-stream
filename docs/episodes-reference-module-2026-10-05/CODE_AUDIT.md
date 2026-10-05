# Code reachability audit

Pending implementation. Compare production entry-point reachability with
`deadcode ./cmd/...` and test reachability with `deadcode -test ./...`, then
verify candidates with source references. Test-only reachability is not enough
to justify keeping a public API. Preserve useful boundary regression tests.

| Candidate | Production use | Decision | Proof |
| --- | --- | --- | --- |
| Deterministic fixture executor | Runtime demo default | Keep as a separate executor adapter | `runtime/internal/composition/planes.go` |
| Intent catalog compilation | Episode assembly, replay baseline, conformance | Keep domain rule and facade delegation | Production callers |
