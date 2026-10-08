# U04 — Delete the engine per-partition run path

Status: todo · Decision: **delete** · Priority: P2 · Size: S

## Finding

`internal/engine/internal/app/run.go` keeps a per-partition loop (`Service.run`,
`drainPartition`, `runBatch`, `applyPartitionRecords`, `readPartitionRecords`)
that only tests call. Production and replay both use `RunGlobal`, which applies
all partitions in durable event-log position order.

## Decision and reasoning

Delete it; this was already the "certain removal" in both earlier reviews.
Invariant 4 (serial per virtual partition) holds under `RunGlobal`, which is
serial overall. Two run loops mean two sets of watermark and checkpoint code
that can drift, and tests that pass on the loop production never uses.

## Steps

1. Port every test that calls the per-partition path to `RunGlobal` (or
   `RunDueTimers`). Keep the assertions; change only the entry point.
2. Delete the five functions. Keep `watermarkForRecord` and `runBeforeApply`
   only if `RunGlobal` still uses them.

## Done when

- The five symbols are gone from `deadcode ./...`.
- Golden replay hashes are unchanged.
- engine README no longer mentions a per-partition run.
