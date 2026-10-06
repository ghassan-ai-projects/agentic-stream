# Production reachability review

Method: `deadcode ./...` roots at `main` and lists functions unreachable from
production code (189 at this commit; raw list in
[deadcode-production-unreachable.txt](deadcode-production-unreachable.txt)).
`deadcode -test ./...` reports none, so every item is reachable from a test.
Limits: reflection-based registration is invisible to the tool; my per-package
classification was made from grep and reading, not symbol by symbol.

Concern score: 1 = legitimate test or contract support, 10 = production needs a
capability it cannot trigger.

| Package | Unreachable | What it is | Concern | Suggested action |
| --- | --- | --- | --- | --- |
| `interlock` | 2 | `Set`: nothing in production can trip the interlock | 8 | Wire an operator command, or document DB-only |
| `replay` (+4 layers) | 93 | Recorded, shadow, counterfactual, baseline modes, `RunNTimes`; CLI wires only `Run` | 8 | Wire to the CLI (shadow mode is a design acceptance item) |
| `engine/internal/app` | 5 | Per-partition run path kept only for tests | 8 | Delete; move tests to `RunGlobal` |
| `qualification` | 6 | Calibration `Activate`, `ShadowComparisonStore.Record` | 7 | Wire, or accept that consequential intents always need approval |
| `notify` | 3 | `Prune`: retention never runs | 7 | Schedule from the runtime |
| `eventlog` (+3 layers) | 14 | Quarantine release/redrive, `RecordGap`; no operator path | 6 | Operator command or remove |
| `executor/native` | 8 | `RunBatch`, `MemoryArtifactStore`; evaluation harness | 4 | Keep or move behind an eval package |
| `episodeledger` | 1 | Non-transactional `RecoverUnfinishedAttempts` (the `Tx` variant is used) | 3 | Delete |
| `ids` | 2 | `Sequence`, test-only | 3 | Delete or move to test support |
| `worker` | 34 | Reference `Server`, registered only in tests; `internal/` prevents outside use | 2 | Keep; consider a `workertest` package |
| `executor/conformance` | 6 | Conformance harness | 2 | Keep |
| `soak` | 3 | `Compute` wrappers (export uses `ComputeTx`) | 2 | Keep or trim |
| `authority` | 2 | Exported wrappers over domain helpers | 2 | Unexport or delete |
| `contractsv1` | 6 | Conformance fixtures | 1 | Keep |
| `notifycontract` | 2 | `Types`, `GoldenEvents` contract fixtures | 1 | Keep |
| `episodes` | 1 | `CompileIntentCatalog` wrapper | 1 | Delete or keep |
| `canonicaljson` | 1 | `MarshalString` | 1 | Delete |

Decisions needed from the owner: the first six rows (wire or remove). Only the
engine row is a certain removal.
