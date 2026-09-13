# A-024 · `internal/notify/notify.go`

LOC: 317 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Exported symbols are documented with comments that describe the actual symbol above them.

## Findings
- **[LOW] F1. Stale doc-comment paragraph describes a nonexistent `ReadAfter`** — `internal/notify/notify.go:130-134`. The comment block above `ReadPage` opens with "ReadAfter returns up to limit events strictly after cursor. It refuses a resume cursor..." — a leftover from a removed symbol — followed by the real `ReadPage` paragraph. Repo-wide grep confirms no `ReadAfter` exists. The surviving text misstates the entry point's name and conflates the cursor-expiry and slow-subscriber behaviors. Fix: delete the stale paragraph.
- **[LOW] F2. Slow-subscriber audit records `oldest.Int64` even when `oldest` is NULL** — `internal/notify/notify.go:157-161`. When the notifications table is empty (`oldest` invalid), the audit row stores 0 as "oldest cursor", producing a misleading audit trail. Pass the same resolved fallback used at 148-151.

## Checked, not an issue
- P1: errors wrapped `%w`; rows closed on every path (180-191); per-record validation is transactional (`recordPoisonAttempt` via `WithTx`); no races (all state in SQLite).
- P2: dedup is content-pinned — same event ID with a different payload is rejected against both live rows and tombstones (73-88, 103-118); poison handling is bounded (3 attempts, then skip + audit, 283-310) so a corrupt row cannot stall or loop a subscriber; `ErrEventExpired` blocks reintroduction after pruning.
- P3: `Page.Skipped` is consumed by tests and documents the skip contract — kept; cursor rollback on duplicate insert is pinned to exact `next_cursor = cursor+1` (111-117) so it cannot rewind another writer's allocation.
- P4: infrastructure plane; callers (cognition, actions) append in their own transactions so the event is atomic with the mutation it announces.
- P5: monotonic per-tenant cursor allocation via `RETURNING next_cursor - 1` (90-95); retention floor of 7 days enforced (244-246); tombstones pruned on the same horizon as the refusal boundary (259-263).
- P6: `notify_test.go` covers duplicate/conflict, poison skip, cursor expiry, pruning.
- P7: `now` is always an injected parameter; no wall-clock reads; cursors are stable identities.
