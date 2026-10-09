# interlock

Status: done
Round: 2

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/interlock` | 96.9% | 96.9% | 1.2 s | 1.3 s | 3 / 3 | 4 / 4 |
| `internal/interlock/internal/domain` | 66.7% | 100% | 1.1 s | 1.1 s | 2 / 2 | 4 / 15 |
| `internal/interlock/internal/store` | 80.0% | 92.0% | 1.3 s | 1.3 s | 2 / 2 | 7 / 7 |

Timings are noisy (other workers were running); the database tests cost about 0.1 s each through `storagetest.OpenTemp`.

## Findings and changes

### Removed
- `internal/store/change_test.go`: merged into `store_test.go` (one file per subject, T3). `git rm`.
- `openDB` and `allow` helper wrappers in the facade test (`storagetest.OpenTemp` is called directly; `allow` is a plain function).

### Renamed or moved
- `TestAssertFailsClosedAndVersionIsMonotonic` (not parallel, `context.Background()`) split into `TestAssertAdmitsReadyAndRefusesTrippedWithItsReason` and `TestSetRefusesAVersionThatIsNotNewer`.
- `TestChangesNeedAReason` → `TestChangesNeedAReasonAndWriteNothingWithout` (trip and clear, asserts the message and the unchanged row).

### Improved
- T4: every error assertion states which error (`ErrTripped` with the stored reason, `sql.ErrNoRows`, `row was not updated`, domain messages, "a fence is required").
- T6: `store_test.go` test was not parallel; all are.
- T5: `context.Background()` replaced by `t.Context()`.
- Domain tests are tables with named rows (`ValidateChange`: unknown/empty status, missing reason, version 0 and -1, missing time).

### Added
- `TestNextStateAdvancesTheVersionAndRecordsTheChange`, `TestNextStateRefusesAnInvalidChangeAndReturnsNoState`: `NextState` was the 0% function that held the domain at 66.7%.
- `TestRequireReadyFailsClosedWithTheStoredReason` (extended): empty and unknown status fail closed.
- `TestReadReturnsTheSeededReadyInterlock`, `TestChangeRefusesAnInvalidChangeAndWritesNothing`, `TestSetRefusesInvalidValuesWithTheDomainMessage`, `TestAssertFailsWhenTheInterlockRowIsMissing`.
- `TestTripInAndClearInChangeTheInterlockInsideTheCallersTransaction`: the `TripIn`/`ClearIn` facade operations had no test.

### Speed
- Nothing slow.

## Production code touched
- none

## Invariants proven here
- 7 (policy revalidates immediately before dispatch: the interlock is asserted again before the effect; clear needs the fence): `TestTripBlocksAndClearReopensTheActionPlane`, `TestClearIsRefusedWithoutTheFenceAndWritesNothing`, `TestAssertAdmitsReadyAndRefusesTrippedWithItsReason`, `TestRequireReadyFailsClosedWithTheStoredReason`. The invariant-to-test map in `invariants.md` is the final round's job.

## Open items
- Doc/behavior gap: the `Assert` doc comment says it fails closed "wrapping `ErrTripped`, when it is absent or not ready". With the row absent it fails closed (error) but wraps `sql.ErrNoRows`, not `ErrTripped`. The test pins the current behavior; either the comment or `store.Assert` should change (production change, out of this round's scope).
