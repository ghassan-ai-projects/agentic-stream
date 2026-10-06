# Completion survey

The starting worktree already moves episode use cases to `internal/app`, adds
a public delegation file and registers the app layer. Preserve that work.
The previous guide's claim that a SQL-free application layer is impossible is
incorrect: `internal/policy/internal/store/tx.go` already joins caller-owned
transactions behind an opaque wrapper.

| Problem | Evidence | Resolution |
| --- | --- | --- |
| App can still reach SQL | `internal/store/tx.go` aliases `sql.Tx`; app calls ledger and cost APIs with it | Opaque `Tx`; SQL and transactional owner calls stay in store |
| Facade contains an epoch rule | `epoch_refusal.go` classifies control errors and silently allows missing control | Store asserts the supplied epoch check; domain classifies the result; execution configuration requires it |
| Pure rules drifted to app | `decision.go`, `failure.go`, `shadow.go`, `intent_catalog.go`, request budget cache | Move rules and vocabulary back to domain with their tests |
| Mutable public configuration | Constructors plus `With*` setters allow incomplete dispatch wiring | One `Service`, `New(Config)`; three operations, explicit assembly-only construction, required execution dependencies |
| Concrete executor leaks into lifecycle | `app/fake_executor.go`; runtime demo composition uses it | Move to `executor/fixture`; keep production demo behavior and test it there |
| Documentation describes a different layout | module README and this folder retain facade use cases and deferred app migration | Rewrite around the actual final layers |
| Shadow vocabulary is wrong | guide calls inserted intents shadow-mode intents; active mode inserts, shadow only scores | Correct guide, comments and language |

Direct reads span scheduler items, trigger evaluations, situations/versions,
episode lifecycle/attempts, reconsiderations, commands, intents, decisions,
outcomes and epoch control. Keep existing read projections in store and record
ownership limits; do not widen mutation authority. Decisions and intent producer
writes already belong to the episode store. Lifecycle transitions remain owned
by their ledgers, all on the original transaction.

Production consumers use assembly/persistence through admission and replay,
execution through runtime, and request/outcome/error contracts through the
native and remote adapters. Catalog compilation is also used by replay and
executor conformance. Fixture execution is used by live demo composition, so
it is not test-only code. Audit remaining reachability with and without tests.
