# A-059 · `internal/notifycontract/contract.go`

LOC: 161 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The contract's type list exists in exactly one place in this file.

## Findings
- **[LOW] F1. The eight notification types are hand-duplicated in two structures** — `internal/notifycontract/contract.go:26-35` (`knownTypes` map) and `50-61` (`Types()` slice). Membership and order must be kept in sync by hand; the existing tests pin only counts (`contract_test.go:13` compares golden count to `len(Types())`; `GoldenEvents` compares to `len(knownTypes)`), so a same-count rename in one list compiles and passes while `Known` and `Types` disagree. This is exactly the duplicated-logic-that-can-diverge case. Fix: keep one ordered slice and derive `knownTypes` from it (or vice versa with a shared slice literal).

## Checked, not an issue
- P1: errors wrapped `%w` throughout; schema compile results cached under a mutex (129-155) — no race, no repeated compile cost.
- P2: `denyNetworkLoader` (157-161) blocks any external `$ref` fetch during schema compilation, so validation cannot become a network instruction channel; `Validate` rejects non-Channel-B types (89-90), pins the dataschema (92-94), and enforces the relational bindings JSON Schema cannot express: `data.tenant_id == envelope tenantid` and `data.source_authority == envelope source` (113-122), plus tracestate-requires-traceparent (123-125).
- P3: `ContractID`/`SchemaID` are the only pinned identities and both are used; goldens are verified against envelope digests in tests.
- P4: contract package sits beside `contractsv1` as a versioned surface; no runtime logic, no domain branches.
- P5: exported symbols documented; `embed.FS` for data (40-41) — contract content is data, not Go literals, consistent with the repo's domain-data rule.
- P6: `contract_test.go` verifies golden conformance and rejects unknown versions and authority mismatches.
- P7: pure function of its input; no clock, no randomness.
