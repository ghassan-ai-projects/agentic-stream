# Plan and round record

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| E0 | Survey, architecture and plan | Read production/tests/consumers and ownership | Complete |
| E1 | Pure domain, token wire codecs and rules | Focused tests, token/fingerprint parity, root gates, whole-tree lint | Pending |
| E2 | Opaque SQL store, app use cases, transport and configured facade; all consumers | Durable replay/integrity, cancellation/ownership/rollback tests, worker/runtime regressions, coverage and lint | Pending |
| E3 | Architecture injection proofs, reachability audit, documentation and final review | Full CI, non-short uncached race, all packages at least 60%, diff check | Pending |

Existing behavior tests move with their implementation; add focused pure tests
and constructor/rollback regressions before accepting each implementation round.
Preserve exact token claims and fingerprint bytes, error precedence/status,
clock reads, transaction/lease predicates, context persistence and query limits.
No golden fixtures, schema, protocol, dependencies or thresholds change.

Deliberate changes: missing configured keys/ownership/call ledger are constructor
errors; key material is copied and private; test-only in-memory calls and public
ledger mutation methods are removed; mutable dependency setters are replaced by
configuration. Tests will exercise the same durable query path as production.

Deferred: owner-provided transactional attempt read ports, coordinated with the
owner module migration. Existing reads remain in store and grant no foreign
mutation authority. Round validation and final rating will be recorded here.
