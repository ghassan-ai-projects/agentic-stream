# Control module

Control is the runtime's control plane: the singleton owner lease that fences
live writers, the epoch drain/kill record, the read-only final dispatch
readiness gate, and durable aggregate cost control (ceilings, the kill switch,
reservations and settlements). `costcontrol` was merged into it: a kill already
releases cost reservations, and every cost caller also asks control about the
epoch.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `RuntimeOwner`, `EpochControl`, `CostLedger`, `SetCostLimit`, `ApplyCostCeilings`, `NewDispatchAuthorization`; sentinel errors; joins the caller's `*sql.Tx` |
| App | Owner claim/renew/release/assert; kill (record, supersede, release unstarted costs), drain, state and the four epoch gates; reserve, settle, set limit, apply ceilings; the dispatch gate |
| Domain | Lease duration and holder rules, epoch refusals by state, cost validation, admission, settlement and ceiling-merge decisions, time encoding |
| Store | The only SQL for `runtime_owner`, `epoch_control`, `cost_limits` and `cost_reservations`; an opaque `Tx` that is the caller's transaction, one it opens, or autocommit |

`RuntimeOwner` and `EpochControl` keep exported configuration fields because they
are built at about seventy sites; an unconfigured value (no database, epoch or
instance) refuses every call with a sentinel error rather than skipping a check.
`CostLedger` is stateless; every cost operation runs on the caller's transaction.

`episodeledger` settles abandoned attempts during recovery through its
`CostSettler` port, which `CostLedger` satisfies; it never imports control.
Kill reads `episodes` joined with `cost_reservations` to find admitted episodes
that never started; that foreign read is recorded as deferred work.

[Migration record](../../docs/control-reference-module-2026-10-06/README.md).
Architecture gates enforce a pure domain, no SQL or `database/sql` in app, exact
write ownership of the four tables, opaque store types and facade delegation.
