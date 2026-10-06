# Production dead code: decisions

Method: `deadcode ./...` (no `-test`) roots at `main` and lists functions that
no production path reaches: 182 at this commit, raw list in
[deadcode-production-unreachable.txt](deadcode-production-unreachable.txt).
`deadcode -test ./...` reports none, so every item is exercised by a test.
Limit: reflection and gRPC service registration are invisible to the tool;
classification below was made by reading callers.

Actions: **Delete** (no value in production or tests worth keeping), **Move**
(keep as test support, out of the production package), **Wire** (a real
capability with no operator path: connect it to the CLI or runtime), **Keep**
(documented reason). Wire items are owner decisions; each has a recommendation.

## Delete: 17 symbols

| Package | Symbols | Why | How |
| --- | --- | --- | --- |
| `engine/internal/app` | `run`, `drainPartition`, `runBatch`, `applyPartitionRecords`, `readPartitionRecords` (5) | Per-partition run path kept only for tests; production uses `RunGlobal` (already recorded in the earlier review as the certain removal) | Delete; port tests to `RunGlobal` |
| `executor/native` | `RunBatch`, `RunBatchJSON`, `executeBatchCell`, `settleBatchCell`, `producedBatchCell` (5) | Runner for an external benchmark harness; no caller in this repository or the CLI | Delete `batch.go` and `batch_test.go` |
| `authority` | `ParseReconciliationEvidence`, `PhysicalEvidenceComplete` (2) | Facade wrappers over domain helpers; used by one device test and one domain test | Delete wrappers; the device test builds evidence through the facade's real operations |
| `episodes` | `CompileIntentCatalog` (1) | Facade wrapper; only tests and the conformance fixture call it | Delete; replace the conformance catalog with a checked-in JSON fixture (domain data stays in JSON, per AGENTS.md); update `architecture_episodes_test.go` and `architecture_decisions_test.go`, which name it |
| `soak` | `Compute`, `ComputeTx`, `commitSoakReport` (3) | Export uses `ComputeTenantTx` | Delete during the `runartifact` merge |
| `control` | `SetCostLimit` (1) | Production writes ceilings through `ApplyCostCeilings`; only cost tests call this | Unexport; tests that need arbitrary limits use a `controltest` helper |

## Move to test support: 49 symbols

| Package | Symbols | Destination |
| --- | --- | --- |
| `worker` | 34: `Server`, `Execute`, handshake and stream validation | `worker/workertest`; production `worker` keeps constants, `ValidateBudget` and the socket helpers |
| `executor/conformance` | 6 | Stay as is; it is already a support package with no production importer. Listed here so `deadcode` noise is expected |
| `contractsv1` | 6: `Conformance*` fixtures | `contractsv1/contractstest` |
| `executor/native` | 3: `MemoryArtifactStore` | Test helper next to its tests |

## Wire or remove: 116 symbols (owner decision)

These are safety or recovery capabilities the design promises, implemented and
tested, with no production caller. Leaving them is the worst option: it is
untested-in-production code on a safety path and gives a false sense of coverage.
Recommendation per row; the cheapest honest outcome is the one that matches the
design acceptance list.

| Area | Symbols | Gap | Recommendation |
| --- | --- | --- | --- |
| `replay` modes: recorded, shadow, counterfactual, baseline, `RunMode`, `RunNTimes` | 93 | CLI wires only `Run`; shadow mode is an acceptance item in the design README | **Wire**: `replay --mode recorded\|shadow\|counterfactual\|baseline` and `--repeat N` (uses `RunNTimes`, `AllHashesEqual`); one round, CLI plus tests. Without it, 93 symbols and `shadow_comparisons` have no production path |
| `eventlog` quarantine release and redrive | 10 | No operator path to release or redrive quarantined events; quarantine is a one-way sink | **Wire**: `agentic-stream quarantine list\|release\|redrive`, fenced by runtime ownership |
| `eventlog` `RecordGap` | 4 | Nothing writes `event_gaps`, so gap tracking in the watermark cannot happen in production | **Verify first**: if ingress has a bounded-overflow path, call it there; if no such path exists, delete `RecordGap`, `Gap` and the table's writer |
| `interlock.Set` | 2 | The interlock is seeded `ready` by migration and nothing can trip it | **Wire**: `agentic-stream interlock trip\|clear --reason`, owner-fenced (the doc comment already requires it). A kill switch with no trigger is not a kill switch |
| `notify` `Prune` | 7 | Retention never runs (already decided: keep as a service operation, schedule once an operator picks a retention) | **Keep**; track as P1 follow-up 1 |

If the owner declines to wire a row, the fallback is delete with its tests; do
not leave it unreachable.

## Keep

| Package | Why |
| --- | --- |
| `executor/conformance` | Contract harness for fixture, native and remote executors |
| `notify` retention | See above |

## Expected result

After the delete and move rounds, `deadcode ./...` should list only the wire
rows (116) until they are wired, then nothing except the `executor/conformance`
harness. A make target should regenerate the raw file and fail when a new
symbol appears that is not in an allow-list (follow-up 17 in the earlier status
folder).
