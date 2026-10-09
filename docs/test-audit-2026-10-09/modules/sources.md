# sources

Status: done
Round: 2

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/sources` | 100% | 100% | 1.2 s | 1.1 s | 6 / 6 | 6 / 10 |
| `internal/sources/internal/domain` | 89.7% | 98.3% | 1.2 s | 1.1 s | 6 / 6 | 11 / 13 |
| `internal/sources/internal/transport` | 90.0% | 90.0% | 1.1 s | 1.1 s | 2 / 2 | 3 / 3 |

The remaining 10% of `transport` is the `panic` when `crypto/rand` fails: not reachable without replacing the OS random source.

## Findings and changes

### Removed
- `sources.PersistGrace == 5*time.Second` assertion in the facade test (T10, constant equals literal; the bound check `0 < remaining <= PersistGrace` stays).
- Facade assertions that repeated domain proof (virtual clock advance; deterministic first id) (T2). The facade keeps wiring checks only: quality of each clock, prefixed and distinct random ids, replayable deterministic ids, the `Or*` defaults, `NowUTC`/`NowFunc`, `OrLease`.

### Renamed or moved
- `TestDetachedContextKeepsValuesDropsCancellationAndIsBounded` (facade) → owned by the domain, `internal/domain/detached_test.go` (T2: the rule lives in `domain.DetachedContext`, which had 0% in its own package). The facade keeps `TestFacadeDetachedContextDropsCancellationAndIsBounded`.
- `TestFacadeExposesPhysicalAndVirtualTime` → `TestFacadeExposesPhysicalAndVirtualClocks`; `TestPhysicalClockIsUTCAndTimersFire` → `…AndItsTimersFire`.

### Improved
- T6: the four virtual-clock tests in `clock_test.go` were not parallel; all are, and `OrLease` is a table of subtests.
- T5: `time.After(time.Second)` in the physical-timer test replaced by a deadline bound to `t.Context()`.
- T4: failure messages state got and want.
- `clock_test.go`: shared `fired(timer)` helper replaces three copies of the select/default; the regression comment is now the test name.

### Added
- `TestQualityNamesTheClockKind`: virtual vs any other clock (domain `Quality` was 0%).
- `TestVirtualClockStartsAtItsStartInUTCAndAdvances`: a start in another zone reads back in UTC.
- `TestVirtualTimerFiresOnlyOnceTheClockReachesItsDueTime`: no fire one nanosecond early; fires with its due time.
- `TestVirtualTimerFiresOnce`, `TestVirtualTimerStop` (stopped timer, double stop, stop after fire).
- `TestPhysicalTimerStopPreventsFiring`.
- `TestIdentityPrefixesAreDistinctAndEndWithAnUnderscore`: identity spaces cannot collide.
- `TestDetachedContextEndsWhenItsCancelFunctionIsCalled`.

### Speed
- Nothing slow.

## Production code touched
- none

## Invariants proven here
- 8 (stable identities, the `sources` row of the map) and 9 (replay is deterministic and clock-free): `TestDeterministicSequenceIsReproducible`, `TestVirtualAdvanceFiresDueTimersBehindLaterHead`, `TestFacadeGeneratesRandomAndDeterministicIdentifiers`.

## Open items
- Same-due-time virtual timers fire in scheduling order (documented). The order is not observable through separate timer channels, so no test pins it; it would need an ordered-delivery hook.
