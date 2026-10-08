# DUP-014: Episode fence and attempt-status point reads are re-written in evidence and episodes

- Status: fixed
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

Verified: the core claim held. `evidence` (reads.go) and `episodes` (runner.go) each re-wrote the episodes fence read, the attempt status by (attempt, episode, fence) and lifecycle point reads that `episodeledger` already owned. Partly right: the finder's "stale-owner-epoch" test idea does not apply to evidence, because evidence calls carry no attempt owner epoch and are fenced by the runtime owner assertion (`AssertOwner`), which stays in evidence; owner-epoch fencing of attempts stays in the ledger's `ValidateWorkerIdentity`. The recovery.go lifecycle read was a seventh copy of the same SELECT inside the ledger.

Changed:
- `internal/episodeledger/reads.go` (new facade) exports `EpisodeFence`, `ReadEpisodeFence(tx)`, `ReadAttemptStatus(tx, identity)` and `ReadEpisodeLifecycle(db)`; `internal/episodeledger/internal/app/reads.go` (new) delegates to the store.
- Ledger store `fence.go`: `ReadEpisodeFence` now also reads `tenant_id` (`EpisodeFence.TenantID`); `ReadTerminalEpisodeFence` and `ReadAttemptStatusOf` are deleted (the terminal-cancellation path uses `ReadEpisodeFence` and `ReadAttempt`); `recovery.go` `EpisodeLifecycle` reads through `ReadEpisodeFence`. `app/identity.go` gained `attemptRecord` (one found-or-refuse step).
- `internal/evidence/internal/store/reads.go`: the three SQL copies are gone; `LiveEpisode` and `CompletionEpisode` read the ledger fence (tenant scoped in `LiveEpisode`) and ask `fence.CheckIdentity` whether the call is the current attempt; `attemptInFlight` uses `ReadAttemptStatus`. `EpisodeState` is now `{Current, Closed, Running}` and `CheckLiveEpisode`/`CheckCompletionEpisode` lost their unused key parameters (evidence domain, app/ledger.go).
- `internal/episodes/internal/store/runner.go`: `EpisodeLifecycle` (now returns `LifecycleStatus`), `AttemptStatus` and `EpisodeSupersededNow` delegate to the ledger; `app/runner_failure.go` compares typed lifecycle.
- `architecture_episodeledger_test.go` lists the new facade operations.

Decisions: refusal text is unchanged (evidence keeps "evidence attempt is stale / no longer current / terminal / episode is unknown"; episodes keep "read episode lifecycle"/"read episode attempt status" prefixes). A missing attempt row at the ledger now refuses as `RejectWrongAttempt` (was a raw sql error) on the terminal-acknowledgement path and on the shared facade read. `ReadAttemptStatus(attemptID)` and `ReadEpisodeAttemptStatus(episode, attempt)` stay in the ledger store: they are keyed differently and used only inside the ledger.

Pinned by: `TestEvidenceFenceVerdictsMatchTheLedgerIdentityRules` (evidence store: every lifecycle x current/stale/wrong attempt, evidence refusal iff ledger `CheckOpenIdentity` refuses, plus tenant mismatch is unknown), `TestEpisodeAndAttemptStateFollowTheLedgerPredicates`, `TestEpisodeFenceReadsReportUnknownEpisodes`, `TestLifecycleReadsAndAttemptCounts`, `TestEpisodeLedgerFacadeOnlyDelegates`.
