# DUP-014: Episode fence and attempt-status point reads are re-written in evidence and episodes

- Status: open
- Severity: medium
- Verdict (finders): REAL
- Themes: persistence
- Wave: 2
- Finder sources: P9 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `episodeledger`. Partly done already: `evidence` and `episodes` stores each have their own attempt-status read; consolidate onto exported ledger reads and map refusals to each module's existing text.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P9: Episode fence and attempt-status point reads are re-written outside the ledger

- Verdict: REAL
- Shared meaning: read an episode's (lifecycle, current attempt, fence) and an attempt's status by (attempt, episode, fence).
- Sites:
  - Owner: internal/episodeledger/internal/store/fence.go:18 `SELECT lifecycle_status, current_attempt_id, current_fence FROM episodes WHERE episode_id=?` (`ReadEpisodeFence`), :50 `SELECT status, owner_epoch FROM episode_attempts WHERE attempt_id=? AND episode_id=? AND fence=?`, :65 same key, status only, :74/:84 two more status-by-id variants; recovery.go:71 lifecycle only.
  - internal/evidence/internal/store/reads.go:31 and :49 (same three columns with COALESCE; one tenant-scoped, one not), :62 status by attempt/episode/fence.
  - internal/episodes/internal/store/runner.go:13 and :24 lifecycle only, :31 status by attempt/episode/fence (byte-identical to fence.go:65).
  - Go-side twins: evidence `EpisodeState{Lifecycle, AttemptID, Fence}` + `CheckLiveEpisode/CheckCompletionEpisode` versus episodeledger `EpisodeFence` + `CheckOpenIdentity/CheckIdentity` (internal/episodeledger/internal/domain/fence.go:19-39). Both ask: is this identity the current open attempt with the current fence.
- How they differ: evidence adds a tenant filter and collapses all refusals to one text; the ledger distinguishes stale/wrong/closed (`RejectStaleAttempt`, ...). Same table, same key, same meaning.
- Risk if left: any change to fence semantics (e.g. owner-epoch fencing, which already exists only on the ledger side at fence.go:50 `owner_epoch`) is not applied to evidence tool calls.
- Proposed canonical owner: `internal/episodeledger` (evidence/store layer 17 and episodes/store layer 21 are above 13; episodes already imports it; evidence/store needs edge `-> internal/episodeledger`).
- Proposed fix: export read functions on the facade (`ReadEpisodeFence`, `ReadAttemptStatus`, `ReadEpisodeLifecycle`) taking `*sql.Tx` like the existing operations; delete the six SQL copies; evidence maps the typed refusal to its own error text.
- Behaviour to preserve: evidence refusal messages and tenant scoping, `ErrNoRows` handling (found=false vs error), clocks unaffected.
- Verification: evidence/internal/store/store_test.go, episodes store tests, episodeledger cancellation_fence_test.go; new test showing evidence refuses a stale-owner-epoch attempt the same way the ledger does.

## Outcome

Not started.
