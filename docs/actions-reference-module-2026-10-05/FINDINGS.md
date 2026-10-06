# Actions migration findings

## Mixed responsibilities

- [`dispatcher.go`](../../internal/actions/dispatcher.go) combines constructor defaults, dispatch ordering, effector invocation, acceptance authorization, timeout classification, device verification, and runtime-owner SQL checks.
- [`dispatch_lease.go`](../../internal/actions/dispatch_lease.go) mixes lease SQL, command JSON decoding/schema/digest validation, lifecycle decisions, and typed command materialization.
- [`dispatch_authorization.go`](../../internal/actions/dispatch_authorization.go) reads through command, intent, decision, episode, Situation, approval, and policy rows while also deciding approval, freshness, identity, digest, and lease validity.
- [`dispatch_finalize.go`](../../internal/actions/dispatch_finalize.go) combines outcome classification and digest creation with outcome/command/outbox/verification SQL and lifecycle notification writes.
- [`dispatch_reconcile.go`](../../internal/actions/dispatch_reconcile.go) combines reconciliation evidence validation, command-state decisions, authority verification, SQL persistence, and notifications.
- [`dispatch_ledger.go`](../../internal/actions/dispatch_ledger.go) contains shared SQL and JSON/digest mechanics used by unrelated lease and outcome paths.

## Public surface and observed consumers

| Current symbol | Production use | Tests or other observations | Migration decision |
| --- | --- | --- | --- |
| `NewDispatcher` / `Dispatcher` | `internal/runtime/internal/composition/planes.go` | action tests | Replace with `New(Config)` and `Service`. |
| `(*Dispatcher).DispatchOnce` | `internal/runtime/internal/app/pipeline_execute.go` | action tests | Keep as a one-line configured Service operation. |
| `WithRuntimeOwner`, `WithInterlock`, `WithTelemetry` | Runtime composition | action tests use interlock setter | Replace mutable post-construction setup with constructor dependencies. |
| `(*Dispatcher).ReconcileUnknown` | Called by the dispatch flow after independent device verification | Direct external-package test calls; no other production caller | Keep the use case private to `internal/app`; retain a direct public operation only if a manual reconciliation entrypoint is added. |
| `CountUnresolvedOutcomes` | `cmd/agentic-stream/effect_profile.go` supplies it to device authority | Authority/device tests and action test | Keep its exact caller-owned `*sql.Tx` callback signature as a one-line facade delegate; move query SQL into the store. |

`actionport.Command`, `Effect`, `Effector`, and authorization values remain owned by `internal/actionport`; no action-specific command type is imported by consumers.

## Optional safety dependencies

`NewDispatcher` silently permits a nil database or effector and fills missing owner, clock, generator, and lease values with defaults. Runtime ownership and the interlock are separately attached through optional setters; both checks are bypassed when omitted. A configured `New(Config)` should require the database, effector, runtime owner and nonempty epoch, plus the interlock that protects effect acceptance. Clock, ID generator, and lease duration defaults may remain if they do not bypass authorization or change identity semantics.

## Cross-module access and transaction ownership

- Actions owns mutations of `commands`, `outbox`, `outcomes`, and `verifications`. Policy alone inserts prepared commands and outbox rows; its narrow command deletion handoff remains unchanged.
- Dispatch candidate and authorization queries read `intents`, `decisions`, `episodes`, `situations`, `approvals`, and `policy_evaluations`, owned by policy, episodes/episodeledger, engine, approvalledger, and policy respectively. Keep them as read-only projections in this migration; consider owner-provided read ports separately.
- Runtime owner and interlock checks, device-command evidence verification, and lifecycle notification appends currently run on the same open SQL transaction as actions mutations. Preserve that transaction identity and error order as store plumbing; do not open a second transaction around those calls.

## Vocabulary and untyped records

- Stored command, intent, decision, provider result, observed effect, notification, and reconciliation documents use `map[string]any`; their schemas/digests and field-to-column identity bindings are repeated across lease and authorization paths.
- Status strings span the command, outbox, outcome, and verification tables. They are related state machines and should be expressed as explicit domain values without changing stored spellings.
- Evidence validation is ordered: allowed final status, evidence presence, source, evidence type, device feedback fields, then the first present digest. Preserve this precedence.
- See the canonical [`actions ubiquitous language`](../../internal/actions/UBIQUITOUS_LANGUAGE.md) for code, column, and event names.

## Smaller defects and audit decisions

- `loadReconcilableCommand` rejects command status while loading it, and `leaseHeld` reports SQL-read state as `held/live` booleans. Move both decisions into domain/app over loaded values; SQL should project the records only.
- `markLeaseFailure` is a receiver method even though it does not use its dispatcher receiver; make it a store operation or plain helper owned by the package layer that calls it.
- `ReconcileUnknown` accepts `manual_review`, while the command update matches only `reconciling` and `outcome_unknown`; it still records the reconciliation outcome and notification and ignores the zero-row update. Preserve this behavior for this migration and track transition semantics as follow-up work.
- `CountUnresolvedOutcomes` is a real production transaction callback, not dead code. `ReconcileUnknown` is production-used inside dispatch but its exported receiver method has no production caller outside the package. Keep the use case, narrow its public exposure.
- Test-only helpers in `dispatcher_test.go` (`recordingEffector`, `verifyingEffector`, `tripBeforeAcceptEffector`, `deadlineEffector`, and ledger readers) are fixtures, not production candidates. Move behavior tests to the app/store package that owns each rule; retain independent caller/API tests at the facade.
