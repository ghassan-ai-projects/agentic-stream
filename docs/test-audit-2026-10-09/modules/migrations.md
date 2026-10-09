# migrations

Status: done
Round: 3

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `migrations` | 82.9% | 88.6% | 1.2 s | 1.1 s | 2 / 4 | 3 / 9 |

Tests are top-level / passing subtests. Hygiene lint 0, repository lint 0, `-race -shuffle=on -count=3` passes.

## Findings and changes

### Added
- `TestReadMigrationLoadsOnlyVersionedScripts`: a directory, a non-SQL file, a script without a version prefix and one with a non-numeric prefix are skipped without error; a versioned script loads with version, name and SQL. This is the rule the existing count test relies on.

### Kept
- `TestAllReturnsEveryScriptInContiguousOrder` (embedded scripts equal loaded migrations, versions contiguous from 1) and `TestParseName`: both already meet the bar.

### Speed
- Nothing slow. Applying the scripts is proven in `internal/storage/internal/store` (`TestOpenCreatesTheDatabaseWithEveryMigrationAndTheRuntimePragmas`, `TestLifecycleMigrationMapsEveryFormerEpisodeStatus`, `TestAFailedMigrationRollsBackAndNamesTheMigration`).

## Production code touched
- none.

## Invariants proven here
- Migration history cannot silently lose or reorder a script: `TestAllReturnsEveryScriptInContiguousOrder`.

## Open items
- The remaining uncovered statements are `embed.FS` read errors that cannot occur for an embedded tree.
