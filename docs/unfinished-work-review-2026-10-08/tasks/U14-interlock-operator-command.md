# U14 — Interlock trip/clear command

Status: todo · Decision: **complete** · Priority: P1 · Size: S · Depends on: U13

## Finding

`runtime_interlock` is the global action-plane stop. Policy, watch and actions
assert it before creating a command and again before delivering an effect
(`interlock.DurableReader`, `control.AuthorizeDispatch`). Migration 013 seeds it
`ready`, and the only writer, `interlock.Set`, has no production caller. The
final dispatch gate therefore always passes, and there is no way to stop effects
except killing the process.

`/control/kill` is a different control: it stops one runtime epoch's work. The
interlock is durable, survives restarts and blocks dispatch from any epoch.

## Decision and reasoning

Complete it. A safety stop with no trigger gives false assurance, and the code,
the version monotonicity rule (`ValidateChange`) and the readers already exist.

```text
agentic-stream interlock status --db <db>
agentic-stream interlock trip   --db <db> --reason <text>
agentic-stream interlock clear  --db <db> --reason <text>
```

- **Trip takes no owner lease.** Tripping only blocks effects, and an
  emergency stop must work when the runtime is hung or holds the lease. The
  version check still rejects stale writes.
- **Clear is owner-fenced** (U13 lease), as the existing doc comment on `Set`
  requires: reopening the action plane must not race a runtime that is
  mid-dispatch. *Owner confirmation needed* (see README).
- Both record the reason and actor in the interlock row and append an audit
  notification.

Check whether `/health/ready` should report a tripped interlock (proposal:
yes, as a non-failing detail, because ingestion stays healthy).

## Done when

- End-to-end test: trip while `serve` runs → the next pending command is refused
  with `ErrTripped` and nothing is delivered; clear (after stopping serve) →
  dispatch resumes.
- `interlock.Set` and `ValidateChange` are reachable from `main`.
- `documentation/operations/` has a short emergency-stop runbook.
