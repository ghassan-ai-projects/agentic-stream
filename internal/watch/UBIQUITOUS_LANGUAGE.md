# Watch ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Watch condition | A bounded, expiring derived trigger scoped to one tenant, Situation version and target. | `domain.Condition` | `watch_conditions` |
| Watch identity | The command's idempotency key, or its command ID when none is set. Re-installing the same identity must carry the same condition. | `domain.WatchID` | `watch_id` |
| Watch expression | A CEL boolean over `features` that selects matching evidence. | `domain.ValidateExpression`, `domain.Evaluate` | `expression` |
| Allowance | The remaining number of fires. Spending the last one disables the watch. | `remaining_fires`, `max_fires` | same |
| Fire | One recorded, deduplicated match of an event against a watch. | `store.Tx.RecordFire` | `watch_fires` |
| Expiry | The instant an active watch stops matching; the row is kept for audit. | `domain.Condition.ExpiresAt` | `expires_at`, status `expired` |

| State | Meaning |
| --- | --- |
| active | Installed, unexpired, with allowance left. |
| disabled | Allowance spent. |
| expired | Passed `expires_at` while active. |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Effector (for the module type) | Watch service | The effect port is a role the service plays, not its identity. |
| Trigger (for a stored watch) | Watch condition | Triggers belong to cognition's scheduler. |
