# U04 — Delete the engine per-partition run path

Status: done · Decision: **delete** · Priority: P2 · Size: S

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

## Result

The five functions and the test-only `Run` export are gone; the engine tests run
`RunGlobal`, the production path. Porting them exposed X10: `RunGlobal`
re-read the whole log on every call. With X10 the ported tests keep their
original expectations (an idle second run processes nothing).
