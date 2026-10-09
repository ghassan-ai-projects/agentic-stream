# storage (with storagetest)

Status: done
Round: 3

Audited in round 3 with [telemetry](telemetry.md) and [migrations](migrations.md);
the shared verification numbers are repeated in each report.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/storage` | 64.0% | 100.0% | 3.0 s | 2.8-3.4 s | 6 / 14 | 6 / 3 |
| `internal/storage/internal/domain` | 100.0% | 100.0% | 1.2 s | 1.1 s | 4 / 0 | 4 / 0 |
| `internal/storage/internal/store` | 78.4% | 89.8% | 8.4 s | 4.0-4.7 s | 15 / 13 | 23 / 26 |
| `internal/storage/storagetest` | 81.4% | 81.4% | 5.4 s | 3.5-4.1 s | 9 / 4 | 10 / 3 |
| `internal/telemetry` | 66.7% | 100.0% | 1.3 s | 1.2 s | 3 / 0 | 2 / 0 |
| `internal/telemetry/internal/domain` | 93.1% | 100.0% | 1.2 s | 1.1 s | 4 / 0 | 8 / 27 |
| `internal/telemetry/internal/transport` | 71.2% | 94.9% | 1.3 s | 1.2 s | 3 / 0 | 9 / 11 |
| `migrations` | 82.9% | 88.6% | 1.2 s | 1.1 s | 2 / 4 | 3 / 9 |

Slowest tests after (under load, race): `TestLifecycleMigrationMapsEveryFormerEpisodeStatus` 2.8 s,
`TestOpenFreshReservesPathUntilClose` 2.7 s, `TestOpenCreatesTheDatabaseWithEveryMigrationAndTheRuntimePragmas` 2.5 s,
the two storagetest cold-build tests 2.3 s, `TestFacadeDelegatesToTheStore` 1.7 s. None is above 5 s; every
one of them is a cold migration run on purpose (1.5 s each when run alone, see Speed). They now run
concurrently, so the package time is the longest, not the sum.

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 13 findings before, 0 after
(the two `//nolint:paralleltest` below are the only non-parallel tests). Repository
`golangci-lint`: 0 issues, also at `dupl` threshold 60. `time.Sleep`: none in the three modules.
`go test -race -shuffle=on -count=3` passes on all three. `go test ./internal/architecture/...` passes.

## Findings and changes

### Removed
- `TestFacadeOpensRunsTransactionsAndCollectsRows` (storage): repeated `WithTx` and `CollectRows`, which the store package tests own (T2). The facade now has one wiring test.
- `TestFacadeRetriesOnlyBusyFailures` (storage): repeated `TestRetrySQLiteBusyPassesThroughOtherOutcomes`.
- `TestFacadeNullIfEmptyAndQueryAll` (storage): `QueryAll` was proven twice; it lives in the facade file, so the more complete store-side test moved here.
- `TestOpenIsIdempotent` (store): asserted `ModTime` not before, which holds even when the file is rewritten (T4). Replaced by `TestOpeningAMigratedDatabaseAgainAppliesNothing`.
- `TestStoredTimeFunctionRefusesNullAndBlobs` (storage): merged into the `stored_time_ok` table (NULL and BLOB are two more rows).
- `TestTemplateBuildDeletesOlderTemplatesOnly` (storagetest): merged into the build-once test; it paid its own cold build for the same directory lifecycle.
- Three `TestTemplateIsRebuiltWhenTheCachedFileIsUnsound` subtests: each paid a 1.5 s cold build. The three unsound shapes are now checked against `readSoundTemplate` directly (no build); one end-to-end rebuild remains.
- Dead fixture row `sit-legacy` (store lifecycle test): inserted, never referenced.
- Comments inside the telemetry domain tests (`P8 ... exit gate 6`, `ISSUE-061`): phase and ticket references inside a module (T3, AGENTS.md no-comments rule).

### Renamed or moved
- `internal/storage/storage_test.go` → `internal/storage/facade_test.go` (T3: names its subject).
- `internal/storage/stored_time_test.go` → `internal/storage/internal/store/stored_time_test.go`: `stored_time_ok` is registered in the store package; the facade package does not own it (T1). Store coverage of `storedTimeOK` rose from 0%.
- `internal/storage/internal/store/storage_test.go` → `legacy_migration_test.go` (lifecycle migration) and `open_test.go` (Open, pragmas, ledger).
- `store/maintenance_test.go` dissolved: `TestOpenFreshReservesPathUntilClose` → `fresh_database_test.go`; the busy-retry tests → `sqlite_retry_test.go`.
- `store/rows_test.go` kept for `CollectRows`; `QueryOptional` moved to `optional_test.go`; `QueryAll` moved to the facade (it is implemented in the facade).
- `fixture_test.go`: `openOwnerDB` (a name from `control`, not storage) → `openSingleConnectionDB`; the unused time return is gone.
- `telemetry/internal/transport/otel_test.go` → `tracing_test.go`.
- `TestOTLPHTTPProviderExportsSpans` (facade) and `TestDurableW3CContextBecomesOpenTelemetryLink` (facade) → `transport/tracing_test.go`, where the code lives. `TestRuntimeCountersAndMetricsAreLowCardinality` (facade) → table in the domain package plus `transport/metrics_test.go`.
- `TestTemplateIsBuiltOnceAndReusedFromItsDirectory` → `TestTemplateIsBuiltOnceReusedAndOlderTemplatesAreDeleted`.

### Improved
- T6: every test parallel except two (global tracer provider): `TestConfigureInstallsTheProcessTracerProviderAndPropagator` (transport) and `TestSpansStartedThroughTheFacadeReachTheConfiguredProvider` (facade). Both carry `//nolint:paralleltest // Configure replaces the process-wide tracer provider and propagator` and restore the previous provider and propagator in `t.Cleanup`. `TestOpenTempGivesAMigratedDatabaseThatClosesWithTheTest` no longer needs a sequential subtest: a cleanup registered before `OpenTemp` runs after the close.
- T4/T10: `TestOpenTempWithoutForeignKeysAllowsOrphanRows` tolerated any non-FK error and asserted through a misnamed variable; it now inserts a real orphan row and asserts one connection. New counterpart `TestOpenTempEnforcesForeignKeys`.
- T4: `TestEveryObservationIncrementsItsCounter` only compared a sum, which a counter incrementing the wrong field would pass. `TestEachObservationIncrementsOnlyItsOwnCounter` asserts that exactly one named counter moved, for all 18 observers.
- T4: the metrics handler test only grepped four substrings. It now parses the served text and requires it to equal `Snapshot` plus `LatencySnapshot`, with the content type, with no labels.
- T4: scan-error tests use `errors.Is` instead of substring checks where a sentinel exists; the OTLP test asserts path, content type and that the payload carries the service name and span name, not only that a request arrived.
- T11: the 230-line lifecycle migration test is a sequence of named steps (`seedLegacyEpisodes`, `insertLegacyEpisode`, `assertEveryFormerStatusIsMapped`, ...). Pragma checks are a table, run on both pooled connections.
- T5: the 6-attempt exhaustion test and the contention tests share `holdWriterLock`/`openContender`; the contention test asserts exactly 2 attempts (it said `< 2`).
- Scan helpers return the scanned value, not an operand evaluated before the call (`return v, r.Scan(&v)` has unspecified order); `scanOne[T]` is the one shared helper.

### Added
- `TestAFailedMigrationRollsBackAndNamesTheMigration` (store): a conflicting table makes migration 1 fail; the error names `migration 1 initial: execute` and no other table is left behind (atomic migration).
- `TestOpenReportsAReservationItCannotInspect` (store): the `inspect replay reservation` error branch of `Open`.
- `TestWithTxCommitsWhenTheWorkSucceeds`, `TestWithTxRollsBackAndReturnsTheWorkError`, `TestWithTxReportsATransactionThatCannotBegin` (store): `WithTx` had 0% store coverage (used by `notify`, `cmd`).
- `TestCheckpointSucceedsOnAnOpenDatabaseAndNamesItsFailureOnAClosedOne` (store).
- `TestEveryPooledConnectionKeepsTheRuntimePragmas` (store): all five runtime pragmas on two simultaneous connections (was `busy_timeout` only).
- `TestRetrySQLiteBusyRecoversWhenTheWriterLockIsReleased`, `...StopsWhenTheContextIsCanceledDuringBackoff` (store): the two halves of the old combined contention test, now separate and parallel.
- `TestRowsAffectedCountsChangedRowsAndReportsZeroWhenTheDriverCannot`, `TestBoolIntIsOneForTrueAndZeroForFalse`, `TestFacadeDelegatesToTheStore` (storage facade): were 0% covered.
- `TestAnUnsoundCachedTemplateIsRefused` (storagetest): the three unsound-cache shapes, without a build.
- `TestOpenTempEnforcesForeignKeys` (storagetest).
- Telemetry domain: `TestPipelineReportAddsEachFieldToItsOwnCounter` (negative and zero ignored), `TestSnapshotNamesEveryCounterExactlyOnce`, `TestNilRuntimeIsInert` (all observers on nil), `TestRuntimeSpansAreRecordedOnlyWhenATracerIsConfigured`, `TestSnapshotsAreIndependentCopies`, `TestLatencySnapshotReportsNanosecondsAndClampsNegativeDurations` (`latencyNanos` and `LatencySnapshot` were 0%).
- Telemetry transport: `TestTracerProviderNamesTheServiceAndDefaultsToAgenticStream`, `TestTracerProviderRejectsAnUnparsableEndpoint`, `TestAddLinkFromW3CLinksOnlyAValidTraceContext` (tracestate kept, all-zero id, garbage, nil span), `TestRecordErrorMarksTheSpanFailedWithAFixedDescription`.
- `migrations`: `TestReadMigrationLoadsOnlyVersionedScripts` (directory, non-SQL file, missing version prefix and non-numeric prefix are skipped without error).

### Speed
- `storagetest` cold template build: 1.5 s under `-race` per build (0.06 s without `-race`; measured by running one build alone, three times). It was run 5 times (build-once, three unsound subtests, older-templates test), all concurrently: 4.0 s per test under load. Now 2 builds, one per behavior that is only observable through a build: build + reuse + older-template cleanup, and rebuild of an unsound file. "Reused" is proven by file identity (`os.SameFile`) and unchanged mod time between the first and second load instead of a third build. Package time 5.4 s → 3.5-4.1 s.
- `internal/storage/internal/store` 8.4 s → 4.0-4.7 s: its four cold migrations (`TestOpenCreates...`, lifecycle migration, `OpenFresh`, the old idempotent test) ran one after the other because none was parallel; the idempotence test now seeds from the template and the others run concurrently. The 3 remaining cold migrations are the behavior under test (`Open` on an empty file, migrating a legacy database, `OpenFresh`).
- `internal/storage` facade: `OpenFresh` was the only cold migration and stays (it is the wiring proof of the replay entry point).

## Production code touched
- none.

## Invariants proven here
- 8 (stable identities, idempotent persistence) is supported by the migration ledger tests: `TestOpeningAMigratedDatabaseAgainAppliesNothing`, `TestAFailedMigrationRollsBackAndNamesTheMigration`, `TestAllReturnsEveryScriptInContiguousOrder`.
- 9 (replay performs no external effects) is supported by the isolation of the replay database: `TestOpenFreshReservesPathUntilClose`, `TestOpenFreshRejectsCollisionsWithoutRemovingExistingFiles`, `TestOpenFreshRejectsDanglingSidecarBeforeDatabaseCollision`, `TestOpenFreshReleasesReservationWhenMigrationIsCanceled`.
- Telemetry carries no verdicts (see `internal/telemetry/UBIQUITOUS_LANGUAGE.md`): `TestMetricsHandlerServesEveryCounterAndLatencyAsUnlabeledPrometheusText` proves no labels and no tenant data are exported.
- The invariant-to-test map in `invariants.md` is the final round's job (T12).

## Open items
- Proposal, `OpenFresh` copying a migrated template (not done: production change). One cold migration run (33 migrations) costs 1.5 s under `-race` (1.51, 1.52, 1.51 s in three runs) and 0.06 s without; a seeded open (`storagetest.OpenTemp`) costs 0.16-0.2 s under `-race`. `OpenFresh` has one production caller, `internal/replay/internal/transport/database.go:26`, so every replay pays the cold run once, and in tests each isolated replay pays about 1.3 s more than a template copy would. It would also pay at runtime in production, where `-race` is off and the cost is ~0.06 s, so the gain there is small. The change would have to keep the reservation directory, the sidecar check and the `O_EXCL` file creation of `reserveFreshDatabase`, and either embed a migrated database in the binary or build it at first use; both need a design decision (a template tied to `migrations.All()` digest, as `storagetest` does). Not recommended for production on speed alone; worth it only if replay-heavy tests remain the bottleneck after rounds 14-15.
- `telemetry.RecordError` documents "without leaking request or evidence payloads", but `span.RecordError(err)` stores the error text in an `exception` event; only the status description is fixed. `TestRecordErrorMarksTheSpanFailedWithAFixedDescription` pins the current behavior (status fixed, exception event present). Whether the event should carry the error text is a product decision, not changed here.
- `TestTracerProviderExportsSpansOverOTLPHTTPToTheTracesPath` posts to an `httptest.Server` on loopback (allowed by T6). It is the only proof that endpoint defaulting, the OTLP/HTTP exporter and the batcher are wired together; in-memory exporters cannot reach `otlpBatcher`. It no longer touches the process-global provider (it uses `NewTracerProvider`), so it runs in parallel.
- `storage.QueryAll`, `InClause`, `RowsAffected`, `BoolInt` and `NullIfEmpty` have real logic in the facade package, while the architecture makes the facade otherwise a pure delegator; they are tested where they live. Moving them into `internal/store` is a production change left to a later refactor.
- Remaining uncovered code is I/O error branches that need a failing filesystem or driver (`newScratchDirectory`, `migrateAndClose`, `scanMigrationVersions`, `requireStoredTimeFunction`); not worth fakes.
