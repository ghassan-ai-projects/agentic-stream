# DUP-030: Insert-or-compare idempotent append repeated in notify, policy and watch

- Status: open
- Severity: low
- Verdict (finders): REAL
- Themes: mechanisms
- Wave: not scheduled
- Finder sources: M12 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Weak candidate; the finder did not read every site. Investigate first; close as won't-fix if the compare steps differ.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M12: Insert-or-compare idempotent append repeated in notify and policy (weak)

- Verdict: REAL (weak)
- Shared meaning: `INSERT ... ON CONFLICT DO NOTHING`, then if no row was inserted compare the stored identity and either accept an identical replay or fail on a different payload.
- Sites: notify/internal/store/append.go:73-97 (insert, `RowsAffected` ignored) + notify/internal/app/publish.go:11-53 + notify/internal/domain/seal.go:40 `CheckSamePayload` (SHA compare, releases the allocated cursor); policy/internal/store/command_writes.go:30-47,56,75 (command and outbox insert, `commandInsertResult`); watch/internal/domain/condition.go `SameAs` ("watch command idempotency conflict") over watch/internal/store/watches.go (two ON CONFLICT statements).
- How they differ: each compares a different identity (payload SHA, command SHA, full struct equality) and words the conflict differently; no shared typed conflict error.
- Risk if left: low; listed so that a future conflict-handling change (typed error, audit row) knows the three sites. Other `ON CONFLICT DO NOTHING` sites (eventlog events.go:35, episodeledger scheduler.go:67 and rejection.go, spec deployments.go, engine situations.go) were not read to the compare step and are not claimed.
- Proposed canonical owner: none yet; a typed `storage.ErrIdempotencyConflict` would be the first step.
- Proposed fix: introduce the sentinel only; do not unify the insert logic.
- Behaviour to preserve: each module's current conflict text and notify's cursor-release behaviour.
- Verification: existing duplicate-append tests in notify, policy and watch.

## Outcome

Not started.
