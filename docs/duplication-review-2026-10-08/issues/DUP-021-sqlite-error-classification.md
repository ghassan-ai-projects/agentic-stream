# DUP-021: SQLite constraint classification lives in episodeledger and depends on message text

- Status: open
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

Not started.
