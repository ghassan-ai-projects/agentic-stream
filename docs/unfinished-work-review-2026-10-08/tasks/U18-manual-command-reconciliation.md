# U18 — Manual command reconciliation

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U13

## Finding

A command whose outcome is unknown (timeout after send) moves to
`outcome_unknown`/`reconciling`, and some end in `manual_review`. Device
commands are reconciled automatically by the device session from typed
state/feedback evidence (`device/internal/app/session_reconcile.go`). For any
other effector there is no production path: `actions.reconcileUnknown` exists,
is tested (including rollback at every write boundary), and is reachable only
through `export_test.go`. The actions facade exposes only `DispatchOnce`.

TECHNICAL_DESIGN §14.4 step 9 and Gate C ("unknown outcomes enter
reconciliation and are never blindly retried") assume a human can close them.
Today such a command stays open forever.

## Decision and reasoning

Complete it. The never-blindly-retry rule is only safe if a human can close the
case with evidence; otherwise it turns into "never resolved".

```text
agentic-stream commands list    --db <db> --status outcome_unknown|reconciling|manual_review [--json]
agentic-stream commands resolve --db <db> <command-id> --status succeeded|failed --evidence <file.json> --reason <text>
```

- `resolve` calls a new facade operation `actions.Service.Reconcile` that wraps
  the existing `reconcileUnknown`. Evidence is validated by the existing typed
  `domain.Evidence` parsing; the reconciliation notice is appended as today.
- Owner-fenced (U13).
- Device commands are refused with "reconciled by the device session". The
  authority barrier owns them, and a second path would bypass it.

## Done when

- Test: a ticket command forced to `outcome_unknown` is listed, resolved with
  evidence, closes its verification, and emits the reconciliation notification;
  a second resolve is refused; a device command is refused.
- `export_test.go` no longer needs `ReconcileUnknown`.
- The limitations page drops "unknown-outcome reconciliation" from the
  internal-only list.
