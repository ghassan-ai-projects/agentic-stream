# Target design

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade | `New(Config)`, public value aliases, transaction joining and delegation | Hold SQL or governance rules. |
| App | Evaluation, approval resolution, command/approval publication, ordered checks | Import SQL/storage or decide pure policy rules. |
| Domain | Canonical policy/assertions/commands, schema/digest/identity validation, risk/freshness/expiry rules, approval/notification values | Perform I/O, read clocks or import adapters. |
| Store | Original transaction, read-only handoff projections, SQL and ledger/notification calls | Choose risk outcomes or verify signatures. |

`New(Config)` requires runtime-ownership and decision-epoch check functions and
an interlock reader. Check functions keep the original `*sql.Tx`; app calls them
through store plumbing. Production binds real control methods. Existing unowned
simulation/test composition supplies explicit checks, preserving that supported
mode without nil safety branches inside policy. Optional calibration means
missing calibration selects approval, never uncalibrated automation.

The facade accepts the caller's transaction. `store.Join` wraps it without
opening, committing or rolling it back. Policy status, command/outbox,
approval lifecycle, notifications and evaluation audit remain one atomic unit.
Cross-module writes remain constrained to the existing handoff phases.

Rules retain their order: owner → load → episode epoch → existing result or
pending document → lifecycle → freshness → completeness → expiry → risk route
→ interlock → command identity/idempotency → rate limit → publication/audit.
Approval resolution retains stale-before-expiry-before-authorization precedence,
then re-runs full evaluation after an authorized approval.

All JSON contracts and digest inputs remain unchanged. Typed governance values
retain original documents where canonical bytes and evidence depend on them.
Register domain/store/app/facade dependencies and SQL ownership in the existing
gates; extend transitive policy exclusion to every private policy layer for
reasoning. Read-only replay definition exports remain allowed.

The implemented public API and preserved operation sequences are recorded in
[MODULE_PATTERN.md](../../internal/policy/README.md).
