# Code and boundary audit

| Candidate | Production evidence | Decision |
| --- | --- | --- |
| Public Engine/Scheduler/Evaluation constructors | Only engine used NewEngine; Scheduler/Evaluation were package implementation/test values | Replace public implementation with configured Service; keep rules in private domain and scheduler private in app |
| NewEngine database argument and Engine.idGen | Database parameter never read; ID field never read | Remove; preserve actual scheduler generator and its fallback |
| Mutable SQL transaction in use cases | Every Process operation used the supplied transaction | Encapsulate the exact pointer in store.Tx; expose named operations only |
| RecordCostRejectionReason | Admission calls it in production | Retain delegating facade operation; its SQL and JSON handling belong to store |
| Test-only fixtures | Populate real SQLite records for timing, capacity, correction and refusal tests | Keep in test files; move behavior tests to their owning layers |
| Helper wrappers | ApplyTiming and FindTrigger wrappers merely repeated domain calls | Remove redundant app/domain wrappers |

Cognition owns trigger_evaluations and reconsiderations. It changes only
situations.last_reasoned_version through the existing narrow handoff. Scheduler,
episode and approval lifecycle changes use their owners on the same transaction.
Read-only Situation, queue and accepted-action joins remain in store; replacing
these with owner-provided transactional projections is documented future work.

The SQL gate falsely classified the existing error text `insert item: %w` as
SQL. It now recognizes SQLite INSERT/INSERT OR ... INTO syntax, with regression
cases for actual SQL and error messages. No executable statement exemption or
lint/coverage threshold was added.

Missing caller transaction and nil cost-refusal reason now return explicit
errors. Required constructor identity/spec checks replace possible later panic.
These are recorded deliberate input changes; valid flow bytes, times and order
are preserved.
