# DUP-004: Episode lifecycle and attempt state sets are re-spelled as literals and SQL lists

- Status: open
- Severity: high
- Verdict (finders): DIVERGED, REAL
- Themes: business rules, persistence
- Wave: 2
- Finder sources: P2, R3 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Resolve the owner conflict: the domain packages of evidence, actions and policy are the same layer as `episodeledger`, so a domain-to-ledger import is rejected by the layer gate. Use the persistence finder's shape: the stores (which sit above the ledger) compute typed results through `episodeledger` predicates and hand the domains a bool or typed value. SQL `IN` lists are built from exported ledger constants. Document why evidence treats `cancelling` as not live. Add the all-values table test.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P2: Episode lifecycle and attempt status sets are re-spelled in Go and SQL outside the owner, and disagree in shape

- Verdict: DIVERGED
- Shared meaning: which episode states are "open/live", "terminal", "produced a Decision", and which attempt states are "active/unfinished".
- Sites:
  - migrations/003_lifecycle_fencing.sql:47-56 CHECK 7 lifecycle values; :103 partial unique index `WHERE lifecycle_status IN ('admitted','running')` (the "live" set); :110-120 CHECK attempt statuses.
  - internal/episodeledger/internal/domain/status.go:19-26 `Closed()` = concluded|closed|superseded|expired|abandoned; :51 `IsTerminalAttempt`; fence.go:23,43 use them; fence.go:43 live = admitted|running.
  - internal/episodeledger/internal/store/recovery.go:89-94 SQL `lifecycle_status NOT IN ('concluded','closed','superseded','expired','abandoned')` (hand copy of `Closed()`); :77,83 `status IN ('dispatched','running','cancelling')` (= not terminal).
  - internal/episodeledger/internal/store/supersession.go:26-31 `lifecycle_status IN ('admitted','running')`, :35 attempts `IN ('dispatched','running')` (excludes cancelling); episode.go:42 same live set.
  - internal/episodes/internal/store/dispatch.go:50 `lifecycle_status IN ('admitted','running')`; :56 `'superseded' AND current_attempt_id IS NULL AND EXISTS epoch_control state='killed'`; assembler.go:75 attempts `IN ('failed','timed_out','cancelled')`; runner.go:24 compares with `episodeledger.LifecycleSuperseded` (correct use).
  - internal/control/internal/store/epoch.go:81 `lifecycle_status = 'admitted' AND current_attempt_id IS NULL`.
  - internal/evidence/internal/domain/attempt.go:15 five literal terminal lifecycle strings (= `Closed()`), :23,:39 attempt live = "dispatched"|"running" (excludes cancelling), :31 completion requires lifecycle "running".
  - internal/actions/internal/domain/authorization.go:93 and internal/policy/internal/domain/rules.go:46 - "episode produced a Decision" = lifecycle `"concluded" || "closed"` (a third set, neither live nor `Closed()`).
- How they differ / already diverged: five different subsets of one 7-value enum, spelled as raw strings in 3 modules whose domain packages cannot import the owner. `closed` and `expired` are never written by any production code (only CHECK, `Closed()`, and the SQL lists) yet are carried in all five copies. Attempt "active" is {dispatched,running} in evidence, {dispatched,running,cancelling} in recovery/`CheckAttemptNotTerminal`, {dispatched,running} in cancellation. The differences may be intentional (cancelling is not callable for evidence) but nothing names or pins that.
- Risk if left: adding a lifecycle value (the CHECK is cumulative, migration-only) requires editing 9 files in 5 modules; evidence would keep accepting calls for a state the ledger considers terminal, or policy/actions would reject a new "success" state.
- Proposed canonical owner: `internal/episodeledger` (layer 13; zero dependencies; the `LifecycleStatus`/`AttemptStatus` aliases already re-export the domain methods). Add methods `Live()`, `ProducedDecision()` (concluded|closed) on `LifecycleStatus`, `Active()` on `AttemptStatus`, and constants for the SQL fragments (e.g. an exported `LiveLifecycleSQL`/placeholder helper, or expose the three reads in C9). Layer constraint: evidence/actions/policy `internal/domain` packages are layer 13, the same layer as `episodeledger`, so a domain->episodeledger edge is rejected. Therefore the stores (evidence 17, actions 23, policy 20, all above 13) import `episodeledger` (new allowedImports edges: `internal/evidence/internal/store`, `internal/actions/internal/store`, `internal/policy/internal/store` -> `internal/episodeledger`) and hand their domains a typed result (bool or `episodeledger.LifecycleStatus`-derived field), or the architecture layer table is revised.
- Proposed fix: (1) delete the five literal copies; evidence domain takes `state.Terminal bool` computed in its store via `episodeledger.LifecycleStatus(x).Closed()`. (2) actions/policy domains take `EpisodeProducedDecision bool`. (3) Build the SQL `IN` lists from `episodeledger` constants (see placeholders helper note below) or keep SQL literals only inside the episodeledger store. (4) Remove or document dead values `closed`/`expired` (they are in a CHECK so removal needs a migration; at minimum add a test that every enum value is written by some transition or is listed as reserved).
- Behaviour to preserve: error text "evidence attempt is stale"/"is terminal"/"no longer current"/"no longer active" (evidence/internal/domain/attempt.go), policy reason `episode_not_concluded` (policy/internal/domain/routing.go:29), actions "command authorization is stale"; the exclusion of `cancelling` in evidence liveness.
- Verification: existing episodeledger lifecycle tests (`internal/episodeledger/lifecycle_test.go`, `cancellation_fence_test.go`); new table test iterating all 7 lifecycle values and 9 attempt values asserting each predicate used by evidence/actions/policy equals the ledger's, plus a migration-contract test (migrations_test.go style) that the CHECK list equals the Go constants.

### Finder report R3: Episode lifecycle and attempt state sets restated as literals

- Verdict: REAL (with one DIVERGED member)
- Shared meaning: which episode lifecycle states are terminal / governable / live, and which attempt states are live.
- Sites:
  - Owner today: internal/episodeledger/internal/domain/status.go:11-15 (consts), :26-33 `LifecycleStatus.Closed()`, :45-56 `CanTransitionAttempt`, :59-66 `IsTerminalAttempt`; fence.go:23 (`Closed()`), :43 (`!= Admitted && != Running`); recovery.go:42 `IsRequeued`.
  - internal/evidence/internal/domain/attempt.go:15 - `Lifecycle == "concluded" || "closed" || "superseded" || "expired" || "abandoned"` (hand copy of `Closed()`).
  - internal/evidence/internal/domain/attempt.go:23,39 - `status != "dispatched" && status != "running"` (live attempt = non-terminal minus cancelling).
  - internal/evidence/internal/domain/attempt.go:31 - `Lifecycle != "running"`.
  - internal/policy/internal/domain/rules.go:46 - `EpisodeConcluded`: `"concluded" || "closed"`.
  - internal/actions/internal/domain/authorization.go:93 - `(episode.Lifecycle == "concluded" || episode.Lifecycle == "closed")` (same predicate, second copy).
  - internal/episodeledger/internal/store/recovery.go:80,87 - SQL `status IN ('dispatched','running','cancelling')`; :93-94 `lifecycle_status NOT IN ('concluded','closed','superseded','expired','abandoned')` (SQL copy of `Closed()`).
  - internal/episodeledger/internal/store/supersession.go:31,41 - `lifecycle_status IN ('admitted','running')` and attempt `status IN ('dispatched','running')`; episode.go:44 same set again.
  - internal/episodes/internal/store/dispatch.go:50,56 - `lifecycle_status IN ('admitted','running')` + `'superseded'`.
  - internal/episodes/internal/store/assembler.go:75 - `status IN ('failed','timed_out','cancelled')` ("failed attempt" count used for the 3-attempt retry rule, domain/lifecycle.go:5).
  - internal/episodes/internal/store/runner.go:25 and app/runner_failure.go:90 - compare lifecycle to `episodeledger.LifecycleSuperseded` (correctly typed).
  - internal/control/internal/store/epoch.go:81 - `lifecycle_status = 'admitted'`.
- How they differ / already diverged: evidence treats a `cancelling` attempt as terminal at reservation (`CheckLiveAttempt`), episodeledger treats it as live (`unfinishedAttemptsSQL`, `IsTerminalAttempt` false). Plausibly intentional (no new evidence calls while cancelling) but nowhere named. policy/actions deliberately use the narrower "decision-bearing" set {concluded, closed}, which is a distinct concept from `Closed()`, yet is spelled twice with no name. SQL IN-lists are never tied to the Go predicates.
- Risk if left: a new terminal state (or renaming `closed`) fixes `Closed()` and silently leaves evidence, policy, actions and 6 SQL statements wrong; `evidence` would keep accepting calls for an episode in the new state.
- Proposed canonical owner: `internal/episodeledger` (13). It already exports `LifecycleStatus`, `IsTerminalAttempt`; evidence/domain (allowed only contractsv1 now), policy/domain and actions/domain would each need one new edge to episodeledger (all higher rank, no cycle; cognition and episodes already import it).
- Proposed fix: export `LifecycleStatus.Closed()` and add `LifecycleStatus.Governable()` (concluded|closed) and `AttemptStatus.Live()` (dispatched|running|cancelling) / `AcceptsEvidence()` (dispatched|running); evidence, policy, actions call them on `episodeledger.LifecycleStatus(string)`; build the SQL IN-lists from exported slices (like `actions.UnresolvedCommandStatuses` already does) in episodeledger store; episodes/control stores use the same slices. Document the cancelling decision in evidence.
- Behaviour to preserve: durable status spellings (frozen), evidence error texts "evidence attempt is stale" / "is terminal" / "is no longer active" / "no longer current", `cancelling`/`cancelled` misspell nolint directives.
- Verification: episodeledger rules_test, evidence attempt tests, policy/actions authorization tests. New: a test iterating every `LifecycleStatus` / `AttemptStatus` const and asserting evidence/policy/actions predicates equal the episodeledger ones (catches a new state).

## Outcome

Not started.
