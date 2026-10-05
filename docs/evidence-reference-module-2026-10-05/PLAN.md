# Plan and round record

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| E0 | Survey, architecture and plan | Read production/tests/consumers and ownership | Complete |
| E1 | Pure domain, token wire codecs and rules | Focused tests, token/fingerprint parity, root gates, whole-tree lint | Complete |
| E2 | Opaque SQL store, app use cases, transport and configured facade; all consumers | Durable replay/integrity, cancellation/ownership/rollback tests, worker/runtime regressions, coverage and lint | Complete |
| E3 | Architecture injection proofs, reachability audit, documentation and final review | Full CI, non-short uncached race, all packages at least 60%, diff check | Pending |

Existing behavior tests move with their implementation; add focused pure tests
and constructor/rollback regressions before accepting each implementation round.
Preserve exact token claims and fingerprint bytes, error precedence/status,
clock reads, transaction/lease predicates, context persistence and query limits.
No golden fixtures, schema, protocol, dependencies or thresholds change.

Deliberate changes: missing configured keys/ownership/call ledger are constructor
errors; key material is copied and private; test-only in-memory calls and public
ledger mutation methods are removed; mutable dependency setters are replaced by
configuration. An omitted scope KeyID now defaults to the configured signing key,
so remote callers no longer inspect key metadata. Negative configured TTL/skew
are rejected. Tests will exercise the same durable query path as production.

Deferred: owner-provided transactional attempt read ports, coordinated with the
owner module migration. Existing reads remain in store and grant no foreign
mutation authority. Round validation and final rating will be recorded here.

E1: token claims are pinned byte-for-byte; pure scope/time and cryptographic
round-trip/tampering tests pass. Existing evidence tests pass, including UDS
after approved local socket execution. Root architecture gates pass; whole-tree
lint reports zero issues. Scope/time rules and v1 encoding now have separate
private packages. Public constructors remain temporary until E2.

E2: facade and every private layer compile and pass focused uncached race tests,
including runtime recovery, remote-worker capability issuance, CLI and private
UDS regression paths. Layer coverage exceeds 60% (facade 100%, app about 82%,
domain about 96%, store about 84%, transport about 94%, wire about 84%).
Whole-tree lint reports zero issues and root gates pass. New tests prove owner
loss, supersession before completion, detached canceled-query failure persistence,
joined rollback and immutable copied keys. Table ownership now points only to
store. Test-aware reachability finds zero unreachable functions.

Self-review moved remaining issuer/audience/default-lease decisions into domain,
result encoding into wire, and eventlog source reads behind its owner API in
transport. Removed the test-only Recover convenience, unused result-hash field
and unused issuer clock-skew field. Callers now use the configured Service;
RecoverTx joins the original owner transaction, and no caller mutates an Owner
field after construction.
