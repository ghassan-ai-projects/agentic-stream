# DUP-003: Text-compared timestamp columns mix fixed-width and variable-width encodings

- Status: fixed
- Severity: high (latent bug)
- Verdict (finders): DIVERGED
- Themes: mechanisms, persistence
- Wave: 2b
- Finder sources: P1, M1 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

**Decision (owner, 2026-10-09): one fixed-width UTC layout for every stored timestamp, no migration of old rows and no compatibility reader. `sources.FormatTime` becomes that layout and is used both for compared columns and for digest inputs; regenerate goldens and digest pins. Fix together with DUP-002 in one change. `interlock` sits below `sources`: move the layout constant to a layer that both can import, or have interlock take the text from its caller; pick the smaller change.**

Not dispatched. This changes a stored contract: lease/due/evaluated columns are compared as TEXT in SQL, and `'...:05Z' > '...:05.5Z'` is true in SQLite, so a lease with time left reads as expired when `now` has a zero fraction. The fix needs a decision: fixed-width UTC for compared columns (with a migration or read-compat rule), kept apart from the `FormatTime` text that feeds digests. `interlock` is below `sources`, so it cannot import the helper. Write the decision into this file, then it becomes a fix task.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P1: Durable timestamp text has three encodings, and text-compared columns mix them

- Verdict: DIVERGED
- Shared meaning: "an instant stored as TEXT in SQLite", compared and ordered lexically by SQL (`lease_until > ?`, `due_at <= ?`, `ORDER BY evaluated_at DESC`, `event_time >= ?`).
- Sites:
  - internal/sources/sources.go:110 - `FormatTime`: UTC, `time.RFC3339Nano` (variable width: trailing zeros trimmed, so `...:05Z` and `...:05.5Z`). The "one" helper used by ~40 sites.
  - internal/control/internal/domain/owner.go:55 - `TimeText`: UTC, fixed-width `2006-01-02T15:04:05.000000000Z`; comment says fixed width is required because owner/epoch columns are compared as TEXT. Writes `runtime_owner.lease_until` (control/internal/app/owner.go:52,70,82,99).
  - internal/episodeledger/internal/store/store.go:50 - `AcceptedAtText`: same fixed-width layout, used only for `episodes.accepted_at` (comment: fixed width keeps lexical order chronological); store.go:46 `TimeText` in the same file is `sources.FormatTime` (variable).
  - internal/interlock/interlock.go:78,102 - private `timeLayout` fixed-width.
  - internal/authority/internal/store/reader.go:17-20 - private `storedTimeLayout`/`formatTime` fixed-width, "every stored timestamp ... compare correctly as text".
  - Raw `x.Format(time.RFC3339Nano)` with NO `.UTC()` and no helper (zone offset preserved if the value is not already UTC): cognition/internal/store/evaluations.go:58 (`trigger_evaluations.evaluated_at`), eventlog/internal/store/events.go:41,42,69 (`event_log.event_time/ingested_at/observed_at`), engine/internal/store/situations.go:34,68,69,100, record.go:31, timers.go:50,92,130 (`timers.due_at`, `due_at <= ?` at timers.go:45), episodeledger/internal/store/scheduler.go:53,58 (`not_before`, `expires_at`) and scheduler_queue.go:17,67 (`updated_at`, `not_before <= ?`; the sibling statement at scheduler_queue.go:42 uses `sources.FormatTime`), situations/internal/domain/materialize.go:106-107.
- How they differ / already diverged: `'2026-01-01T00:00:00Z' > '2026-01-01T00:00:00.5Z'` is TRUE in SQLite (checked), i.e. any comparison where one side is whole-second/trimmed and the other is not orders wrongly. Concrete mixed comparison: `runtime_owner.lease_until` is written fixed-width by control, but episodeledger/internal/store/fence.go:93-95 (`OwnerHoldsLease`) compares it with `store.TimeText(now)` = variable-width. `ORDER BY evaluated_at DESC LIMIT 1` (cognition/internal/store/queue.go:29, drives debounce/cooldown `LatestAdmittedTime`) and actions/internal/store/authorization.go:47 order variable-width text. The repo's own comments (control, episodeledger, authority) state fixed width is a correctness requirement, yet the session consolidation (commit 44fa270) standardised on the variable-width form. Looks like an unnoticed latent bug, not intent; effects show only with whole-second or trailing-zero instants (virtual clocks, replay, 1 Hz sensors mixed with ms ones).
- Risk if left: lease expiry, timer due, debounce "latest admitted" and event-window reads silently mis-order at second boundaries; fixes must be made in 4 private copies plus ~40 helper callers; the deterministic-replay invariant depends on these comparisons.
- Proposed canonical owner: `internal/sources` (layer 4, already imported by every store). Add `sources.FormatTime` as fixed-width (or add `sources.FormatSortableTime`) and make `control`, `interlock`, `authority`, `episodeledger.AcceptedAtText` delete their private copies and call it; replace the raw `.Format(time.RFC3339Nano)` sites with it. No new allowedImports edges (interlock is layer 2, below sources layer 4: interlock cannot import sources, so interlock keeps its own constant or the helper moves to a layer-0 package; flag this).
- Proposed fix: decide one stored layout. Fixed-width UTC is the only layout for which SQL text comparison is chronological. Parsing already uses `time.Parse(time.RFC3339Nano, ...)` which accepts fixed width, so readers are unaffected. Existing rows written variable-width need a one-time migration (UPDATE ... to rewrite lease/due/evaluated columns) or a read-compat note; this is a stored-contract change and needs an ADR/migration decision. Pin the choice in `sources_test.go` with a table test over whole-second, fractional and non-UTC inputs asserting `a<b` iff `Format(a)<Format(b)`.
- Behaviour to preserve: digests that include timestamps (evidence/wire/fingerprint.go:14 and token.go:35-40 hash `sources.FormatTime` output; contractsv1 cloud_event.go:111; actions dispatch.go:77 outcome document; situations materialize.go:142,153 facts) - changing the layout changes those digests and golden replay fixtures. Either keep `FormatTime` for digest/document use and add a separate storage-column formatter, or regenerate goldens deliberately. Do not change this silently.
- Verification: add the ordering property test above; extend an integration test that writes a lease at `T+0.5s` and checks `OwnerHoldsLease` at whole-second `T`; existing golden replay and `TestAllBuiltinsLoadFromData`-style pins must stay green or be consciously regenerated.

### Finder report M1: Durable timestamp text has two encodings, and ~20 SQL predicates order it as text

- Verdict: DIVERGED
- Shared meaning: a stored instant is TEXT that SQL compares with `<`, `<=`, `>` (lease expiry, timer due, not-before, retention). That only works if the text sorts chronologically, i.e. fixed-width UTC.
- Sites (encoders):
  - internal/sources/sources.go:110 - `FormatTime`: UTC, `time.RFC3339Nano` (trailing zeros trimmed, so width varies; `...05Z` vs `...05.5Z`).
  - internal/control/internal/domain/owner.go:55 - `TimeText`: fixed-width `2006-01-02T15:04:05.000000000Z` (comment says why: TEXT comparison).
  - internal/interlock/interlock.go:102 (used :78) - same fixed-width layout, own constant.
  - internal/authority/internal/store/reader.go:17,19 - `storedTimeLayout`/`formatTime`: same fixed-width layout, own constant (parse at :23 uses RFC3339Nano).
  - internal/episodeledger/internal/store/store.go:46 - `TimeText` = `sources.FormatTime` (variable width); :50 `AcceptedAtText` = fixed-width, same module, different columns.
  - internal/policy/internal/domain/documents.go:60 - `policy.FormatTime`, a wrapper of `sources.FormatTime`.
  - Raw `.Format(time.RFC3339Nano)` with no `.UTC()` (zone kept, so an offset event time is stored as `+02:00`): internal/engine/internal/store/timers.go:48,92,130; record.go:31; situations.go:34,68,69,100; internal/eventlog/internal/store/events.go:41,42,69; internal/cognition/internal/store/evaluations.go:58; internal/episodeledger/internal/store/scheduler.go:53,58, scheduler_queue.go:17,67; internal/storage/internal/store/storage.go:166.
- Sites (text-ordered SQL predicates): evidence/internal/store/writes.go:27,39,51 (`lease_until > ? / <= ?`); actions/internal/store/leases.go:33,42, candidates.go:25,26,33; watch/internal/store/watches.go:53,63,77,99; engine/internal/store/timers.go:48; control/internal/store/owner.go:96,104; episodeledger/internal/store/scheduler_queue.go:65 and fence.go:95; notify/internal/store/retention.go:12,19,31,39.
- How they differ / already diverged:
  1. Cross-module mismatch: control writes `runtime_owner.lease_until` fixed-width; episodeledger/internal/store/fence.go:95 compares it with `store.TimeText(now)` (= variable width, identity.go:52). `...05Z` sorts AFTER `...05.5Z` ('Z' > '.'), so a lease with 0.5 s left is read as expired whenever `now` has a zero fraction (common with the virtual clock). (known: #3b)
  2. Same bug inside single modules: actions writes `lease_until` with `sources.FormatTime` and compares with `lease_until > ?` in SQL (leases.go:33,42), but decides the same predicate in Go by parsing (`OutboxLease.LeaseStanding`, domain/dispatch.go:23). The two can disagree for the same row.
  3. engine timers compare `due_at <= ?` with a raw non-UTC `Format`, so a non-UTC `now` or `DueAt` changes the text.
  4. `sources.FormatTime` was consolidated this session onto RFC3339Nano, the layout four other modules deliberately avoided; it is also an input to digests (evidence/internal/wire/fingerprint.go:14 request fingerprint, token.go:40), so it cannot simply be switched.
- Risk if left: lease/fence/timer/retention predicates silently wrong at sub-second boundaries; replay with a whole-second virtual clock can order differently than a physical clock; any new module picks one of three formats.
- Proposed canonical owner: `internal/sources` (leaf; already imported by the actions, evidence, engine, watch, episodeledger, cognition, notify and eventlog stores; control/internal/domain imports nothing, so control would need `internal/control/internal/store -> internal/sources` or keep one thin wrapper).
- Proposed fix: (a) add `sources.FormatOrdered(at)` (fixed-width UTC, = the existing layout) next to `FormatTime`; keep `FormatTime` for digest inputs and external JSON. (b) Every column that SQL compares (lease_until, due_at, not_before, expires_at where `<`/`>` is used, notify retention cutoff, scheduler not_before) is written and bound with `FormatOrdered`. (c) Delete control `TimeText`, interlock `timeLayout` (interlock domain/store allow no imports today: add `internal/interlock -> internal/sources`), authority `storedTimeLayout/formatTime`, episodeledger `TimeText/AcceptedAtText`, `policy.FormatTime`; new edge `internal/control/internal/store -> internal/sources` (control domain stays import-free). (d) Migration/compat: existing rows hold trimmed text; either accept a one-off normalising migration or leave pre-release data. Do C2 first (it removes the cross-module case without any encoding change).
- Behaviour to preserve: digests built from `sources.FormatTime` (evidence fingerprint, watch `ExpiresAt` in condition.go:71, token payload); accepted_at ordering of episodes; golden replay output; notification `created_at`.
- Verification: control runtime_owner_test.go / recovery_fence_test.go, actions lease tests, engine timer tests. New test: table-driven "lease live at boundary" with `now` whole-second vs lease fractional, in control, actions and evidence stores; one test asserting `FormatOrdered` is monotone over random instants.

## Outcome

Done together with DUP-002 through the import-design rework (see import-design.md, group results).

- Verified: durable timestamps were written with three encodings and read with about 28 hand-written parses; text-compared columns mixed fixed-width and variable-width forms.
- Changed: one fixed-width UTC layout, `kernel.FormatTime` and `kernel.ParseTime`, replaces every private copy. Domain records now carry `time.Time`; stores parse once and fail closed with a wrapped error; deadline rules use plain comparisons. `interlock` and `authority` use the kernel layout.
- Pinned: `TestDurableTimeTextOrdersChronologicallyAndRoundTrips` (kernel), `TestTimestampTextHasOneOwner` (root gate: no other file formats or parses with RFC3339Nano, except the CloudEvent wire form), and the fail-closed store tests named in the group results.
- Decisions: the CloudEvent digest keeps its trimmed RFC 3339 wire form in `contractsv1` because downstream consumers pin it. No stored rows are migrated (no backward compatibility, per the owner).
- Commit: see the git history of the import-design rework.
