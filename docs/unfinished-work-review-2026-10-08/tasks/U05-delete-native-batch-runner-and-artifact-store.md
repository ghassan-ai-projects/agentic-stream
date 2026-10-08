# U05 — Delete the native batch runner and the artifact store port

Status: todo · Decision: **delete** · Priority: P2 · Size: S

## Finding

- `RunBatch`, `RunBatchJSON`, `executeBatchCell`, `settleBatchCell`,
  `producedBatchCell` (`internal/executor/native/internal/app/batch.go`) run an
  external benchmark harness. Nothing in this repository, the CLI or `docs/eval`
  calls them.
- `native.Config.ArtifactStore` receives tool results larger than the budget.
  Production never sets it (`internal/runtime/internal/transport/worker.go:85`),
  so an oversized result fails the episode with `tool_result_oversized`. The
  only implementation is `MemoryArtifactStore`, used by tests.
- The design's content-addressed artifact store (TECHNICAL_DESIGN §14.3) was
  never built; the `artifacts` table has no reader or writer.

## Decision and reasoning

Delete the batch runner: no caller, no plan item.

Delete the `ArtifactStore` port, `MemoryArtifactStore` and the
`Observation.Artifact` branch. Production already fails closed on oversized
results, and that is the right v1 behavior: the budget exists to keep results
small, and storing a large result elsewhere would let an episode read more than
its budget through the reference. Production behavior does not change. Remove
§14.3 from the plan ([PLAN_CHANGES](../PLAN_CHANGES.md) P03); if a domain later
needs large evidence, it should come through an evidence tool with its own
bound, which ADR-015 already sketches.

## Steps

1. Delete `batch.go`, `batch_test.go` and the facade exports.
2. Remove `ArtifactStore`, `MemoryArtifactStore`, `NewMemoryArtifactStore` and
   the artifact branch in `observationForResult`; keep the
   `tool_result_oversized` failure and its test.
3. Check `internal/executor/remote/internal/domain/request.go` and the proto
   `artifact_manifest` field: they describe the *request* manifest, not this
   store. Leave them unchanged.

## Done when

- 11 `executor/native` symbols are gone from `deadcode ./...`.
- The native README and UBIQUITOUS_LANGUAGE no longer mention a batch runner or
  an artifact store.
