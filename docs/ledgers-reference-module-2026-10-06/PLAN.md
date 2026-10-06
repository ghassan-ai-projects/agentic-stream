# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, merge decision, design, plan | Review | Complete |
| 1 | Merge `scheduleledger` into `episodeledger` as one flat package; rename queue operations; update callers | Full tests, lint, gates | Complete |
| 2 | Layer `episodeledger`: domain, store, app, facade; layer table; ownership | Layer, purity and ownership gates; tests per layer | |
| 3 | Gates, injection proof, `UBIQUITOUS_LANGUAGE.md`, module guide, docs | Injected failures, full CI | |
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
