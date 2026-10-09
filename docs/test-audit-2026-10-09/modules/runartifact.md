# runartifact

Status: done
Round: 14 (speed round; the module was already fast)

## Metrics

| Package | Coverage before | after | Time before | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/runartifact` | 100.0% | 100.0% | 1.9-2.2 s | 1.9 s | 8 / 16 | 8 / 16 |
| `internal/runartifact/internal/app` | 75.6% | 75.6% | 2.4 s | 2.1 s | 10 / 20 | 10 / 20 |
| `internal/runartifact/internal/domain` | 79.4% | 79.4% | 1.4 s | 1.1 s | 11 / 11 | 11 / 11 |
| `internal/runartifact/internal/store` | 71.3% | 84.0% | 1.6 s | 1.6 s | 2 / 2 | 8 / 8 |
| `internal/runartifact/internal/transport` | 76.6% | 76.6% | 1.4 s | 1.1 s | 3 / 3 | 3 / 3 |

No test above 0.5 s of its own; the package times are Go test binary start-up and the race runtime.

## Findings and changes

### Renamed or moved
- `export_test.go` → `export_verify_test.go` (facade) and `internal/app/export_test.go` → `internal/app/export_verify_test.go`: they were ordinary test files named like Go export hooks (T3).

### Improved
- T6: every top-level test and subtest in `export_verify_test.go` (both), `soak_test.go`, `verification_boundaries_test.go`, `internal/domain/rules_test.go`, `internal/transport/directory_test.go` calls `t.Parallel()` (20 `paralleltest` findings before; all use `t.TempDir` and `storagetest.OpenTemp`).
- T4: assertions that only checked `err != nil` now name the failure: existing output, tampered ledger (`checksum mismatch for commands.jsonl`), stale command digest, stale safety-event digest, missing store/output/tenant, empty or missing artifact directory, path-escaping file name, file used as directory.
- `internal/store` tests rewritten (T8): the two tests ran every query against an empty database, so `scanRow` and tenant filtering were never exercised.

### Added (store)
- `TestLedgerReturnsOnlyTheTenantsRowsInPositionOrder` (tenant isolation, ordering), `TestSnapshotReadsTheNewestDeploymentAndPolicyOfTheTenant`, `TestSnapshotReadsDeviceStateByIdOrTheFirstDevice`, `TestSafetyEvidenceCountsTheTenantsActionOutcomes` (commands, unresolved, awaiting verification, other tenants excluded), `TestSnapshotReportsTheAppliedMigrationVersion`, `TestStoreWithoutADatabaseIsZeroAndASnapshotNeedsOne`, `TestLedgerRefusesAFileThatIsNotPartOfTheArtifact`, `TestEveryLedgerFileCanBeReadFromAnEmptyRunDatabase` (the old first test).

### Removed
- Nothing.

## Production code touched
- none

## Invariants proven here
- none directly (run artifacts are an evidence projection, never a write path).

## Open items
- `internal/app` (75.6%) and `internal/transport` (76.6%) sit just above the 75% target; the uncovered statements are I/O error branches of the artifact directory.
