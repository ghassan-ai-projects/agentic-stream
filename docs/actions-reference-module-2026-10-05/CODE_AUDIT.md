# Code and boundary audit

| Candidate | Production evidence | Decision |
| --- | --- | --- |
| `Dispatcher`, `NewDispatcher`, `WithRuntimeOwner`, `WithInterlock`, `WithTelemetry` | Only runtime composition constructed and configured it; tests used the setters | Replace with `New(Config)` and `Service`; ownership and interlock are constructor requirements |
| Exported `ReconcileUnknown` | Called only inside dispatch after device verification; external callers were tests | Private app use case; tests reach it through `export_test.go` |
| `CountUnresolvedOutcomes` | `cmd` passes it to the device authority; soak and device tests use it | Keep as a one-line transaction-scoped facade delegate; SQL moves to store |
| `leasedCommand.CommandJSON/CommandSHA` | Written, never read after leasing | Dropped from `domain.LeasedCommand` |
| `markLeaseFailure` receiver method | Never used its receiver | Plain store operation `FailInvalidCommand` |
| `leaseHeld` returning booleans from SQL | Decision hidden in a read helper | Store returns the lease row; domain decides standing |
| `wire` layer in the design | Document codecs are schema and digest checks over `canonicaljson` and `contractsv1` | Merged into domain (`domain.Document` and the candidate/authorization checks) |
| Test-only effectors and ledger readers | Real SQLite fixtures for behavior tests | Kept in app tests, with a smaller copy of the seed in store tests |

Actions owns `commands`, `outbox`, `outcomes` and `verifications` mutations.
Policy still alone inserts prepared commands and outbox rows, and still alone
discards its pending command before publication; the ownership gate keeps those
phases and now restricts actions to the status, lease and error columns in
`internal/actions/internal/store`. Reads of `intents`, `decisions`, `episodes`,
`situations`, `approvals` and `policy_evaluations` stay read-only store
projections on the dispatch transaction; owner-provided projections are future
work.

Double-counted lease expiry is preserved: an abandoned lease is observed when it
is reclaimed and again when its recovery outcome finds the lease expired. Counting
it once would change a telemetry counter and is a separate decision.
