# Policy as a reference module — October 2026

Refactor `internal/policy` to the reference-module standard used by authority
and device. Preserve deterministic governance, approval signatures, command
identities and transaction-scoped producer handoffs. Internal Go compatibility
is not a goal. External contracts, database schema and replay output stay fixed.

- [Findings](FINDINGS.md)
- [Ubiquitous language](UBIQUITOUS_LANGUAGE.md)
- [Design](DESIGN.md)
- [Plan](PLAN.md)

```text
runtime / exporters / replay
            |
      policy facade
            |
       internal/app
        /         \
internal/domain  internal/store
  pure rules      original transaction, SQL, ledger handoffs
```
