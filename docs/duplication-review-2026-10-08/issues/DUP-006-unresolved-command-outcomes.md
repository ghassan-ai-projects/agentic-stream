# DUP-006: The "unresolved command outcome" set differs between actions and the soak report

- Status: fixed
- Severity: high (probable bug)
- Verdict (finders): DIVERGED
- Themes: business rules, persistence
- Wave: 1
- Finder sources: R4, P3 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Move the command-state vocabulary and `UnresolvedCommandStatuses` to `actionport`; actions and runartifact build their SQL lists from it. Decide explicitly that the soak gate counts `manual_review` (recommended) and pin it with a test.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R4: "Command outcome still unresolved" has three definitions

- Verdict: DIVERGED
- Shared meaning: the set of command states (and verification states) whose outcome still awaits reconciliation.
- Sites:
  - internal/actions/internal/domain/status.go:54-56 - `UnresolvedCommandStatuses = {outcome_unknown, reconciling, manual_review}` (used by `CountUnresolvedOutcomes`, the authority safety gate: internal/actions/internal/store/unresolved.go:26).
  - internal/actions/internal/store/reconciliation.go:54 and :76 - the same three states re-typed as SQL literals (`AwaitingReconciliation`, close guard).
  - internal/runartifact/internal/store/safety.go:62 and :66 - `c.status IN ('outcome_unknown','reconciling')` plus `v.status = 'awaiting'` for the soak report `unresolved_action_outcomes` (a zero-tolerance gate, domain/soak.go:105).
  - cmd/agentic-stream/commands_command.go:23 - help text lists the three states.
- How they differ / already diverged: runartifact omits `manual_review`. A command parked for manual review is "awaiting reconciliation" for the operator and the authority barrier, but the soak safety report counts it as resolved. No ADR found for this; looks like a latent bug.
- Risk if left: the run artifact can report a clean soak while the reconcile barrier is still closed.
- Proposed canonical owner: `internal/actionport` (rank 5, "approved-command/effect contracts without implementation dependencies", imported by actions, control, device, watch). runartifact (25) is peer to actions (25) so cannot import actions; actionport is importable by both (runartifact needs one new edge). Authority already receives the count via an injected `OutcomeLedger`; that stays.
- Proposed fix: move the command-state consts and `UnresolvedCommandStatuses` to actionport (actions re-exports them), build SQL lists from the slice in actions store; runartifact safety.go builds its `IN (...)` from the same slice (and states `VerificationAwaiting` from the same place). Fix `manual_review` omission deliberately (decide: include).
- Behaviour to preserve: durable status strings; `unresolved_action_outcomes` and `unknown_outcomes` diagnostic names; soak verdict reasons (runartifact rules_test.go:15).
- Verification: runartifact soak_test.go:62 (expects 1), actions reconciliation tests. New: soak test with a `manual_review` command expecting it counted; a test asserting runartifact and `CountUnresolvedOutcomes` agree on every command status.

### Finder report P3: "Unresolved command outcome" status set differs between actions and the soak report

- Verdict: DIVERGED
- Shared meaning: commands whose outcome still needs reconciliation / operator action.
- Sites:
  - internal/actions/internal/domain/status.go:56 `UnresolvedCommandStatuses = {outcome_unknown, reconciling, manual_review}`; used only by internal/actions/internal/store/unresolved.go:26 (the authority reconciliation barrier via `actions.CountUnresolvedOutcomes`, service.go:71).
  - internal/actions/internal/store/reconciliation.go:54 literal `status IN ('reconciling','outcome_unknown','manual_review')` (close), :76 same literal (list awaiting). Both re-spell the domain constant.
  - internal/runartifact/internal/store/safety.go:62 `unknown_outcomes` = `IN ('outcome_unknown','reconciling')`; :66 `unresolved_action_outcomes` = `(c.status IN ('outcome_unknown','reconciling') OR v.status = 'awaiting')`.
  - cmd/agentic-stream/commands_command.go:23 help text lists all three.
- How they differ / already diverged: the soak zero-tolerance gate `unresolved_action_outcomes` (runartifact/internal/domain/soak.go:105, fails the verdict) ignores `manual_review` commands, while the dispatch/authority barrier and the operator list treat `manual_review` as unresolved. A command parked for manual review therefore blocks authority but passes the soak report. Looks like a latent bug (names say "unresolved"), not documented intent.
- Risk if left: a soak run can pass with commands awaiting a human; any new unresolved state must be added in 4 places.
- Proposed canonical owner: `internal/actions` domain constant already exists. runartifact/internal/store is layer 23 and `actions` facade is layer 25, so runartifact cannot import `actions`. Options: (a) pass the status list into runartifact through its constructor from `cmd` (composition root already imports both); (b) move the command-status vocabulary to `internal/actionport` (layer 5, already the shared command contract package, imported by actions, watch, device) and import it from runartifact/store (new edge `internal/runartifact/internal/store -> internal/actionport`). Prefer (b).
- Proposed fix: export `UnresolvedCommandStatuses` from actionport; actions reconciliation.go:54,76 and runartifact safety.go:62,66 build their `IN` lists from it (shared placeholders helper, see C11 notes); decide explicitly whether the soak gate includes `manual_review` (recommended: yes) and update the diagnostic name semantics.
- Behaviour to preserve: `CountUnresolvedOutcomes` result (authority barrier), SoakReport JSON diagnostic keys (`unknown_outcomes`, `unresolved_action_outcomes`), operator list order `updated_at, command_id`.
- Verification: runartifact/internal/app/soak_test.go:62 asserts `unresolved_action_outcomes == 1` for one case; add a case with a `manual_review` command and one test asserting the actionport list equals the actions constant used by the barrier.

## Outcome

Status: fixed. Commit: pending (reviewer commits).

Verified (all claims held):
- `actions/internal/domain/status.go` `UnresolvedCommandStatuses` = outcome_unknown, reconciling, manual_review: confirmed; used by the authority barrier (`CountUnresolvedOutcomes`).
- `actions/internal/store/reconciliation.go` close guard and `AwaitingReconciliation` re-spelled the three states as SQL literals: confirmed. A fourth copy was found: `ReconcilableCommand.RequireAwaitingReconciliation` switched over the same three states in domain.
- `runartifact/internal/store/safety.go` omitted `manual_review` in both `unknown_outcomes` and `unresolved_action_outcomes`: confirmed. The `OR v.status = 'awaiting'` arm does not rescue it, because reconciling a command to manual_review sets its verification to `inconclusive`. So a parked command blocked the authority barrier but passed the soak gate. Real bug.
- `cmd/agentic-stream/commands_command.go:23` help text lists the three states: confirmed, correct, left as is (it is user text).

Changed:
- `internal/actionport/internal/domain/status.go` (new): command-state and verification-state vocabulary, `UnresolvedCommandStatuses()` (fresh slice each call), `IsUnresolvedCommandStatus`.
- `internal/actionport/status.go` (new): facade re-export with doc comments.
- `internal/actions/internal/domain/status.go`: removed the Command*/Verification* consts and the `UnresolvedCommandStatuses` var. Every use in actions (domain, tests, store test) now reads `actionport.X`, rewritten with a go/ast offset-rewrite program, no aliases left behind.
- `internal/actions/internal/domain/reconciliation.go`: `RequireAwaitingReconciliation` uses `actionport.IsUnresolvedCommandStatus`.
- `internal/actions/internal/store/unresolved.go`, `reconciliation.go`: the barrier count, the close guard and the awaiting list all build `status IN (...)` from `unresolvedCommandFilter()`, which takes the list from actionport. Local `placeholders` helper deleted.
- `internal/storage/storage.go`: new `storage.InClause(column, values)` (the rejected.md placeholder note says to fold the builder into the fix that generates an IN list); used by actions and runartifact. The authority/events.go and engine/timers.go placeholder builders are untouched (outside this issue).
- `internal/runartifact/internal/store/safety.go`: the diagnostics queries are built from actionport (`UnresolvedCommandStatuses`, `VerificationAwaiting`) instead of literals. The `architecture_test.go` edge `runartifact/internal/store -> internal/actionport` was needed and is present in `allowedImports`.
- `internal/actionport/UBIQUITOUS_LANGUAGE.md`: new row for the unresolved command set.

Decisions:
- The soak gate now counts `manual_review` (deliberate bug fix; behaviour change). Diagnostic keys `unknown_outcomes` and `unresolved_action_outcomes` and the verdict reason text are unchanged.
- `unknown_outcomes` (a diagnostic, not a verdict input) now also counts manual_review: it uses the same single set rather than a second hand-typed list. Its value can rise for runs that have a manual_review command; no consumer reads it.
- Owner is actionport (rank 5, importable by both actions and runartifact), as the reviewer notes proposed. Authority still receives the count through the injected `OutcomeLedger`.
- Outbox/outcome/reconciliation/error-code vocabularies stay in actions (not shared).

Tests pinning the rule:
- `TestUnresolvedCommandStatusesAreExactlyTheStatesAwaitingReconciliation`, `TestUnresolvedCommandStatusesCannotBeMutatedByCallers` (internal/actionport/status_test.go).
- `TestSoakReportCountsEveryCommandStatusLikeTheReconciliationBarrier` (internal/runartifact/internal/app/soak_test.go): every command status; soak count, `actions.CountUnresolvedOutcomes` and the verdict must agree, so the manual_review case fails without the fix.
- `TestInClauseBindsOnePlaceholderPerValue` (internal/storage/storage_test.go).
- Existing `TestReconcilableCommandAcceptsOnlyAwaitingStatuses`, `TestSoakReportFailsUnresolvedActionOutcome` pass unchanged.
