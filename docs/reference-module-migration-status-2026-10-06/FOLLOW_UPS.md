# Follow-ups to do later

Consolidated, prioritised list of work found during the notify and control
migrations and the earlier module migrations. Each item names where it was
found and what "done" looks like. Priority: **P1** safety or correctness gap,
**P2** structure or consistency, **P3** polish.

## P1 — safety and correctness

| # | Item | Found in | Done when |
| --- | --- | --- | --- |
| 1 | **Notification retention is never scheduled.** `notify.Service.Prune` exists and is tested, but nothing calls it, so `notifications` grows without bound and `ErrCursorExpired` can never occur in production. | notify FINDINGS | An operator chose a retention value and a command or runtime job runs `Prune` (design §14.5: deletion is a durable job with dry-run and legal-hold checks). |
| 2 | **`Prune` is three autocommit statements.** A crash between tombstoning and deleting leaves harmless but inconsistent state. | notify PLAN | One transaction, with a crash-between-statements test. |
| 3 | **Optional cost settler in recovery.** `RecoveryCoordinator.Costs` may be nil and recovery then skips cost release silently. | control FINDINGS | Cost settler is a constructor requirement; a missing one fails construction. |
| 4 | **No operator path for several safety levers**: interlock trip (`interlock.Set`), calibration activation, quarantine release/redrive, replay shadow/counterfactual/baseline. Reachable from tests only. | DEADCODE.md | Each is either wired to the CLI or removed with its tests. |

## P2 — structure and consistency

| # | Item | Found in | Done when |
| --- | --- | --- | --- |
| 5 | **Owner-provided read ports.** Modules join tables they do not own: actions, policy and cognition read `intents`, `decisions`, `episodes`, `situations`, `approvals`, `policy_evaluations`; control's kill reads `episodes` joined with `cost_reservations`. | DEFERRED #1, control FINDINGS | Each owner exposes a read port; consumers read only their own tables; the SQL/ownership gate forbids foreign reads. |
| 6 | **Constructors with errors for `RuntimeOwner` and `EpochControl`.** They are built as literals at about 70 sites and refuse use at call time. | control PLAN | `NewRuntimeOwner`/`NewEpochControl` return errors for a missing database; literals are gone. |
| 7 | **Typed enum values** at Go level: notify payload statuses, `verdict`, `risk_class`, epoch state at the database boundary. They are only schema-validated. | notify PLAN, control PLAN | Named string types with a closed set parsed once at the boundary. |
| 8 | **Source drift for `situation.trigger.evaluated`**: published with source `//agentic-stream/tenants/<id>` (plural) while lifecycle events use `//agentic-stream/tenant/<id>`. It is not a contract type and is part of stored digests. | notify FINDINGS | Decision whether to register it as a contract type and unify the source, with a golden for it. |
| 9 | **Notification audit ids use the policy prefix** (`ids.PrefixPolicy`). | notify FINDINGS | A notification-specific prefix, if ids are ever parsed. |
| 10 | **Typed records** still pending: provider and observed-effect results (actions), simulator records (ingress), operator-state and timer payloads (engine, operators). | DEFERRED #2 | Typed structs parsed once at the boundary, original bytes kept where a digest needs them. |
| 11 | **Transaction scope**: one transaction for an ingress batch and its checkpoint; one per global engine batch; per-watch fan-out in one transaction. Each changes delivery semantics and needs a decision. | DEFERRED #3 | Decision recorded, tests for crash between steps. |
| 12 | **Remaining unmigrated packages.** `qualification` and the three ledgers (`episodeledger`, `scheduleledger`, `approvalledger`) are the shared transaction-scoped stores still in their old shape; `executor/native`, `executor/remote`, `runartifact`, `worker`, `soak` need a shape review. | status README | Each migrated or recorded as "decided: no". |
| 13 | **Test-only code** from the dead-code review: `engine` per-partition run path (certain removal), `episodeledger` non-transactional recovery wrapper, `ids.Sequence`, `canonicaljson.MarshalString`, exported wrappers in `authority`. | DEADCODE.md | Deleted with their tests, or moved to test support. |

## P3 — polish

| # | Item | Done when |
| --- | --- | --- |
| 14 | `notify` payload `Delta`/`Action` are open objects; policy normalises nil to `{}`. Consider a typed delta. | Typed or documented as contract-open. |
| 15 | The layer table is a reviewed renumbering. After notify and control it has gaps (for example `authority` 6). Re-derive it from the import graph and keep it minimal. | A script checks the table is the minimal longest-path assignment. |
| 16 | `proto-check` needs the pinned `protoc`; it was not run locally. | Run on a machine with the pinned version. |
| 17 | Re-run `deadcode ./...` after each migration and keep `deadcode-production-unreachable.txt` current (192 lines at this commit, mostly replay modes and the reference worker). | The file is regenerated by a make target. |

## Not follow-ups (decided)

`Prune` stays a service operation, not removed. `costcontrol` merged into `control`
(cycle cut with a port). `notifycontract` merged into `notify`.
