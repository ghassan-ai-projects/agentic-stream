# DUP-021: SQLite constraint classification lives in episodeledger and depends on message text

- Status: fixed
- Severity: low
- Verdict (finders): REAL
- Themes: contracts and shapes
- Wave: 1
- Finder sources: S11 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Add `storage.IsUniqueViolation` next to `IsSQLiteBusy`; episodeledger drops its driver import.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S11: SQLite error classification outside `internal/storage`

- Verdict: REAL
- Shared meaning: classify a driver error (busy/locked, unique-constraint) from `modernc.org/sqlite` result codes.
- Sites:
  - internal/storage/internal/store/sqlite.go:16-25 - `IsSQLiteBusy`: `errors.As(*sqlite.Error)`, `Code() & 0xff`
  - internal/episodeledger/internal/store/scheduler.go:36-47 - `IsSchedulerItemIDConflict`: `errors.As(*sqlite.Error)`, code in {CONSTRAINT_PRIMARYKEY, CONSTRAINT_UNIQUE}, then `strings.Contains(err.Error(), "UNIQUE constraint failed: scheduler_items.scheduler_item_id")`
  - internal/episodeledger/internal/store/admission.go:30-37 - `IsLiveEpisodeViolation`: code == CONSTRAINT_UNIQUE only, then `strings.Contains(err.Error(), "UNIQUE constraint failed: episodes.situation_id")`
- How they differ: the two constraint matchers accept different code sets for the same class of failure (primary key vs unique) and both depend on the driver's message text; episodeledger imports the driver directly although AGENTS.md/A2 say SQLite infrastructure lives in storage.
- Risk if left: a driver upgrade that changes message text or extended codes silently turns a benign duplicate into a hard error (scheduler items are inserted idempotently) or makes the live-episode conflict undetectable (`ErrLiveEpisodeConflict` then never fires).
- Proposed canonical owner: `internal/storage` (layer 7; episodeledger/internal/store already imports it).
- Proposed fix: add `storage.IsUniqueViolation(err error, column string) bool` (accepting both PRIMARYKEY and UNIQUE extended codes, one text match) next to `IsSQLiteBusy`; episodeledger calls it; drop its `modernc.org/sqlite` imports.
- Behaviour to preserve: the exact `ErrLiveEpisodeConflict` mapping for reconsiderations only (`Admission.ReportsLiveConflict`) and idempotent scheduler insert behaviour.
- Verification: episodeledger store tests for duplicate item and second live episode; add a storage test for both extended codes and a wrapped error.

## Outcome

Verified: all three sites confirmed. The two constraint matchers did accept different code sets (PRIMARYKEY+UNIQUE versus UNIQUE only) and both matched driver message text; episodeledger imported `modernc.org/sqlite` directly. The accepted-code difference is harmless in practice (a primary key clash on `episodes.situation_id` cannot occur, since that column is only constrained by a partial unique index), so one rule accepting both codes preserves behaviour.

Changed:
- `internal/storage/internal/store/sqlite.go`: new `IsUniqueViolation(err, column)` next to `IsSQLiteBusy` (extended codes CONSTRAINT_PRIMARYKEY or CONSTRAINT_UNIQUE, then one text match on `UNIQUE constraint failed: <table.column>`).
- `internal/storage/storage.go`: facade `IsUniqueViolation`.
- `internal/episodeledger/internal/store/scheduler.go` and `admission.go`: `IsSchedulerItemIDConflict` and `IsLiveEpisodeViolation` now delegate to `storage.IsUniqueViolation` with their column; the driver, `errors` and `strings` imports are gone. The two named predicates stay because they name the domain meaning and fix the column, and `app` and the existing tests call them. No new architecture edge was needed (episodeledger/internal/store already imports storage).
- `internal/storage/internal/store/unique_violation_test.go`: new.

Pinned by `TestIsUniqueViolationClassifiesPrimaryKeyAndUniqueIndex` (real SQLite primary key, partial unique index, wrapped error, other column, non-sqlite and plain-text errors, nil) in storage, and by the existing episodeledger store tests `TestSchedulerQueueRoundTrip` (id clash) and the second-live-episode assertion that still exercise the real driver through the shared rule.

Results: go build, go vet, go test -count=1 on internal/storage/..., internal/episodeledger/..., internal/runtime/... and the root architecture gates pass; golangci-lint on storage and episodeledger: 0 issues.
