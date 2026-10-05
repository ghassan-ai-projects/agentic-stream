# Actions migration plan

## Rounds

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, canonical vocabulary, target design, ordered implementation plan | Review and commit these five dated planning files and `internal/actions/UBIQUITOUS_LANGUAGE.md` before code changes | Complete; 5b7bc88 |
| 1 | Extract pure domain decisions and document checks (wire merged into domain); move/assign tests with the rules they prove | Focused domain tests; preserve command, intent, decision and outcome schema/digest behavior | Complete |
| 2 | Move every owned SQL statement into an opaque store; keep owner, interlock, authority evidence, and notification checks on the same original transaction | Store tests for ownership, lease CAS, rollback, authorization projections and unresolved counts | Complete |
| 3 | Move use-case sequencing into app and replace root `Dispatcher` with `New(Config)`/`Service`; update runtime composition and `DispatchOnce` caller | Facade/app tests; constructor refusal tests; full focused action and runtime test sets | Complete |
| 4 | Add architecture gates and map/ownership entries; prove gates by injection; write code audit and final validation | Generic and module-specific gates, injected failures, lint, coverage, full CI | Complete |
| 5 | Record unresolved ownership/read-port and transition work without broadening this migration | Future-work report and final review | Complete (see Deferred work) |

Rounds 1-3 landed in one commit because the ownership gates would otherwise see
two writers of the command and outbox ledgers, and the old and new dispatcher
cannot coexist behind one runtime composition.

Each implementation round is a separate reviewed commit owned by the repository
integrator. Before the first code change, the integrator commits round 0.

## Behavior that must not change

- Policy remains the only producer of prepared commands and command outbox rows; actions only updates dispatch/status fields allowed by the existing handoff.
- The dispatcher never imports concrete device or watch implementations; effects cross only `actionport`.
- Command idempotency and command/intent/decision digest input bytes, schema validation, field-to-column bindings, and error precedence remain stable.
- Runtime-owner and interlock checks remain fail closed and use the same transaction as the lifecycle writes. Device evidence verification and notifications also keep the original open transaction.
- Outbox candidate order, lease acquisition and refresh predicates, dispatch timeout bound, context cancellation, clocks, generated identities, and terminal-state handling remain unchanged.
- Unknown results, provider deadlines, expired in-flight leases, and verification errors never cause blind retries. Provider receipts alone do not become physical success.
- Outcome, verification, command, outbox, and lifecycle notification writes remain atomic.
- Reconciliation evidence precedence and command/device binding checks remain unchanged. Keep the accepted `manual_review` behavior until a separate behavior change is planned.

## Deliberate changes

- Replace the mutable `NewDispatcher` plus optional owner/interlock setters with `New(Config)`, requiring DB, effector, runtime owner/epoch, and interlock during construction. This removes silent safety-check bypasses; missing configuration becomes a constructor error.
- Move the production-used `ReconcileUnknown` use case into private app code. There is no non-test caller of its exported receiver operation; a human/operator API needs a concrete authorized caller before being exposed.
- Keep `CountUnresolvedOutcomes` as the public transactional callback because `cmd` supplies it to the device-authority constructor; its SQL moves behind the action store.
- Keep existing clock, ID generator, telemetry, and positive lease defaults unless review shows they alter safety or identity behavior.
- The interlock is now required, so the "dispatcher has no interlock" branch (plain `Dispatch`) is gone: every command runs through `actionport.AuthorizedEffector.DispatchAuthorized`, and an effector that is not authorized fails the command closed. Production composition already set the interlock, so only test effectors change.
- The "no effector configured" finalization is replaced by a constructor error.
- Composition without a runtime owner passes an explicit always-pass ownership check (the policy precedent) instead of omitting the check, so the check can no longer be skipped by forgetting a setter.
- Error text drift without behavior change: lease-inactive and reconciliation-validation errors no longer embed SQL no-rows text or an extra wrapper; reconciled-notification errors carry one wrapper, not two.

## Deferred work

- Replace read-only joins into policy, episode, engine, and approval tables with owner-provided transactional projection ports if a concrete ownership review requires it. Keep the current joins read-only and on the same transaction in this migration.
- Clarify whether reconciling a `manual_review` command should leave it in `manual_review`, move it to a terminal status, or reject that final status. Current code appends reconciliation evidence while the conditional status update affects no row; preserve and test this behavior now.
- Add typed JSON records for command/effect/provider/evidence documents at the wire boundary, retaining the exact fields and canonical digest inputs; do not widen schema in this refactor.
- Add targeted persistence/notification fault-injection tests for each write boundary if the moved store code reveals gaps in existing rollback coverage.

## Round status

| Gate | Evidence | Status |
| --- | --- | --- |
| Planning folder and canonical language committed before code | Commit reference to be added by integrator | Pending |
| Focused package tests per implementation round | Paths and results recorded after each round | Pending |
| New package coverage at least 60% | `go test -short -cover` results recorded after extraction | Pending |
| Architecture gates and violation injection | Test names and expected failures recorded after gate round | Pending |
| `golangci-lint run ./...` | Final output recorded after implementation | Pending |
| `make ci-check` | Final output recorded after implementation | Pending |
| Uncached full race suite | Final output recorded after implementation | Pending |
| Dead/test-only audit | Search and pinned deadcode results recorded after implementation | Pending |
