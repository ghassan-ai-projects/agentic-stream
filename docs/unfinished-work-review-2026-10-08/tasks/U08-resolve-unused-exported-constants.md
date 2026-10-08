# U08 — Resolve unused exported constants

Status: todo · Decision: **delete three, keep the status sets** · Priority: P3 · Size: XS

## Finding (from DEADCODE_REPORT, re-checked 2026-10-08)

| Constants | Where | Production use |
| --- | --- | --- |
| `ClassificationPublic`, `ClassificationConfidential`, `ClassificationRestricted` | `contractsv1/internal/domain/envelope.go` | none; production only uses `ClassificationInternal`; the spec stores input classification as a plain string |
| `watch.StatusActive`, `StatusDisabled` | `watch/internal/domain/condition.go` | written as SQL literals `'active'`, `'disabled'` in `watch/internal/store/watches.go` |
| `approvalledger.StatusPending`, `StatusDenied` | `approvalledger/internal/domain/approval.go` | SQL literals in `approvalledger/internal/store` |
| `actions.CommandPending`, `CommandDispatching`, `OutboxPending` | `actions/internal/domain/status.go` | SQL literals in `actions/internal/store` |

## Decision and reasoning

- **Delete** the three unused classification constants. The facade already
  exposes only `ClassificationInternal`; the other values are spec data and the
  schema keeps them. A Go constant that no code reads documents nothing the
  schema does not already say.
- **Keep** the status constants. Each set is the Go name of a closed database
  enum; the store writes some values as SQL literals. Deleting half a closed set
  leaves an incomplete vocabulary. Exclude them from the unused-symbol scan with
  a comment saying why. Making the store use the constants is FOLLOW_UPS #7
  (typed enums), not this task.

## Done when

The three constants are deleted, and the U12 allow-list notes why the status
sets stay.
