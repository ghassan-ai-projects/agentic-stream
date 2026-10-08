# U17 — Notification retention command

Status: done · Decision: **complete** · Priority: P1 · Size: S · Depends on: U13

## Finding

`notify.Service.Prune` (7 symbols with its domain and store steps) retires
notifications older than a retention period (minimum seven days), deletes
retired rows and expires tombstones. Nothing calls it, so `notifications` grows
without bound, and `ErrCursorExpired` (the audited resnapshot path that Gate D
requires) can never happen in production. FOLLOW_UPS #1 and #2 track this; the
notify migration decided to keep `Prune` "until an operator chooses a
retention".

`Prune` runs as three autocommit statements; a crash in between leaves
inconsistent (though harmless) state (FOLLOW_UPS #2).

## Decision and reasoning

Complete it as an operator command, not a background job:

```text
agentic-stream notifications prune --db <db> --retention 720h [--dry-run]
```

- An explicit command lets the operator choose the retention, which was the
  open decision, and schedule it with whatever runs the host (cron, systemd
  timer). A built-in scheduler would need retention configuration in the spec,
  which was deliberately removed (limitations page).
- `--dry-run` reports the counts it would retire, delete and expire, as
  TECHNICAL_DESIGN §14.5 requires ("durable job with dry-run").
- Owner-fenced (U13), because retiring rows under a live SSE subscriber must not
  race its cursor reads.
- Fix FOLLOW_UPS #2 in the same task: one transaction, plus a test that injects
  a failure between the steps.

The §14.5 "legal-hold check" is removed from the plan
([PLAN_CHANGES](../PLAN_CHANGES.md) P03): there is no legal-hold model and no
requirement source for it in v1.

## Done when

- Test: prune with a retention under seven days is refused; dry-run changes
  nothing; a real prune makes an old cursor get `ErrCursorExpired` and an
  audited resnapshot.
- FOLLOW_UPS #1 and #2 are marked done; the notifications contract page
  documents the command.
