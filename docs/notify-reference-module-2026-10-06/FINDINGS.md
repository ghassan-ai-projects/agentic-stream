# Findings

## Mixed responsibilities

- `internal/notify/notify.go` holds sealing (canonical JSON, SHA-256), the
  duplicate/tombstone decisions, cursor allocation SQL, retention rules and audit
  SQL in one file. `read.go` mixes the lag/expiry decision, row scanning and
  record decoding; `poison.go` mixes the retry-budget decision with its SQL.
- `lifecycle.go` builds events in the persistence package and reaches a second
  package (`notifycontract`) for validation.

## Dead and test-only code (`deadcode ./...` from `main`)

| Symbol | Reachable from | Decision |
| --- | --- | --- |
| `notify.Prune` (+ two helpers) | tests only | **Keep** as `Service.Prune`. It is the only way `ErrCursorExpired`, `ErrEventExpired` and the tombstones can ever occur; the delivery contract documents "cursor expiry after retention". It is not scheduled: it deletes audit-relevant rows, needs an operator-chosen retention, and design §14.5 says deletion is a durable job with dry-run. Recorded as an open decision, not wired silently. |
| `notifycontract.Types` | tests only | Remove; the ordered list becomes the single source of `known`. |
| `notifycontract.GoldenEvents`, `ContractID` | tests only | Remove from production; goldens load from the contract file inside tests. The golden file stays beside the schema. |
| `notifycontract.Known` | `Validate` | Becomes domain-private. |

## Leaking public surface

- `AppendLifecycleEventWithTrace` takes ten positional arguments (eight callers).
- Lifecycle type strings are written twice: `notify.Type*` constants and a map in `notifycontract`.
- `Record`, `Page` and four sentinel errors are defined in the persistence file.
- `ReadPage` takes seven positional arguments and a raw `*storage.DB` on every call.

## Optional safety dependencies

- None are skipped silently. `ReadPage` with a nil database panics at the first query;
  the new `New` makes the database a constructor requirement.

## Cross-module data access

- No other module reads or writes `notify`'s tables (the ownership gate already
  names `internal/notify`). Producers append through the public functions inside
  their own transaction, which is the intended plumbing.

## Vocabulary drift

- Notification audit ids use `ids.PrefixPolicy`. Kept (stored ids are not parsed); recorded as retired-later.
- `cognition` publishes `situation.trigger.evaluated` with source `//agentic-stream/tenants/<id>`
  (plural) while lifecycle events use `//agentic-stream/tenant/<id>`. It is not a contract
  type and is part of stored event digests, so it is left as is and listed as deferred.
- "notification", "event" and "record" are used interchangeably for the stored row.

## Untyped records

- Lifecycle `data` is `map[string]any` at all eight producers; producers also repeat
  `tenant_id` and `source_authority`, which the contract requires to equal the envelope.
  The JSON Schema plus the binding check catch mismatches, so typing is deferred.

## Smaller defects

- `Prune` runs its three statements as independent autocommit statements. Harmless
  (idempotent, tombstone before delete) but not atomic; preserved, listed as deferred.
- `audit`/`auditTx` discard the error from marshalling `details_json`.
- `releaseRacedCursor` mixes a decision (payload equality) with the rollback SQL.
