# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, merge decision, design, plan | Review | Complete |
| 1 | Merge `scheduleledger` into `episodeledger` as one flat package; rename queue operations; update callers | Full tests, lint, gates | Complete |
| 2 | Layer `episodeledger`: domain, store, app, facade; layer table; ownership | Layer, purity and ownership gates; tests per layer | Complete |
| 3 | Gates, injection proof, `UBIQUITOUS_LANGUAGE.md`, module guide, docs | Injected failures, full CI | Complete (episodeledger) |
| 4 | `approvalledger`: drop `notify`, layer it, language file, gates | Same, plus the withdrawal ordering test | |

## Behavior that must not change

Fence and identity error reasons and their precedence (episode fence → owner epoch → attempt state);
the superseded-episode cancellation acknowledgement; transition table; rejection id derivation and
idempotency; recovery counting (abandoned, requeued, abandoned episodes) and cost release only for
canceling attempts; admission default of `shadow`; `ErrLiveEpisodeConflict` only for reconsiderations;
scheduler item upsert identity rules; coalescing predicates; all SQL.

## Deliberate changes

- `scheduleledger` removed; names per the language file.
- Non-cost `RecoverUnfinishedAttempts` and unowned `StartAttempt` stay only if production uses them
  (round 2 decides from `deadcode`).
- No-op error wraps removed; the owner-epoch assertion takes its time from the facade.

## Deferred

Read ports for `runtime_owner`, `trigger_evaluations` and `intents`; typed lifecycle columns.

## Result for episodeledger (rounds 2 and 3)

- Layers: domain 0, store 1, app 2, facade 3 (the minimum the import graph allows); executors, `episodes`,
  `actions`, `device`, `control`, `admission`, `api`, `authority`, `soak` and `runtime` moved up one level.
- The 13 pre-existing ledger tests (fencing, recovery, cancellation acknowledgement, ownership, queue)
  were converted to external tests of the facade and pass unchanged in behavior.
- Gates: `architecture_episodeledger_test.go` plus the generic purity, layering, SQL-location, ownership and
  language gates; proven by ten injected violations.
- Deliberate differences: `RecoverUnfinishedAttemptsWithCost` is now `RecoverUnfinishedAttempts` with an optional
  settler; the owner-lease check takes its instant from the facade; `ErrLiveEpisodeConflict` wraps the
  store's "insert episode" error rather than the reverse; the two coalescers report "... rows affected";
  one episode-read error text is shared by the fence and startable reads.
- Found and recorded: the lease comparison uses a different time encoding than control writes (follow-up 3b).

## Unused functionality (checked with `deadcode ./...` and production callers)

Every facade operation has a production caller except three that only tests used:
`ValidateWorkerIdentity` (production validates inside `TransitionAttempt`), `IsIdentityReason` and
`CanTransitionAttempt` (the domain keeps them for its own checks). They were removed from the facade and
their tests now go through `TransitionAttempt` and the domain; `deadcode` reports nothing for the module.
