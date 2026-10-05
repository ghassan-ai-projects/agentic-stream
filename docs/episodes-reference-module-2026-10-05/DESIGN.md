# Completion design

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade | Public domain aliases, executor port, validated configuration, three delegating service methods, catalog delegation | SQL, transaction ownership, epoch classification or lifecycle rules |
| App | Assembly and bounded lifecycle sequencing; clock reads, identity allocation, telemetry, cancellation; chooses which domain decision to persist | Database imports, raw SQL transaction access, ledger/cost methods accepting SQL |
| Domain | Request/outcome vocabulary, cached wall budget, catalog admission, provenance, decision validation input/digests, failure classification, freshness/retry/terminal and shadow rules | I/O, control implementation, clock reads |
| Store | Join caller transactions, open runner units of work, SQL projections/writes, ledger/cost/epoch/shadow calls on the original transaction | Domain decisions, hidden commits during joined operations |
| Fixture executor | Deterministic decision adapter for demo/replay fixtures | Episode lifecycle or effect mutations |

Public surface: `Service`, `Config`, `ExecutionConfig`, `New`, `Request`,
`Outcome`, `Executor`, both budget errors and `CompileIntentCatalog`.
`Service` exposes `Assemble`, `Persist`, `RunOnce`; rebind is a private runner use case. `Execution == nil`
is explicit assembly-only configuration: `RunOnce` refuses it. Configured
execution requires a database, executor and decision-epoch check. Clock and
ID generator keep their previous defaults. Cost accounting remains an
optional feature. A configured shadow spec requires shadow persistence.

Admission/replay still own `*sql.Tx` and commit or roll back their whole unit
of work. The facade only calls `store.Join`; app receives an opaque transaction.
Runner claims and conclusions continue to open separate transactions through
store, with execution outside both. Epoch checks receive the same underlying
transaction. Refusal reasons and wrapped sentinel errors retain precedence.

No schema, wire, digest, golden fixture or dependency changes. Forward flow
and mutation ownership remain unchanged. Domain may depend on pure public
contracts from `spec`, `decisions`, ledgers and qualification: their packages
sit below it in the reviewed dependency graph; it calls no I/O method.

Enforce import direction, domain purity, app infrastructure isolation, SQL
ownership, effect isolation and facade delegation. Prove the additional gates
with injected violations. Use behavioral regression tests for transaction
rollback, epoch refusal before and after execution, and replay parity.
