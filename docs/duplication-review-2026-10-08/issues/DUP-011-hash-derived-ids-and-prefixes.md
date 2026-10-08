# DUP-011: Hash-derived identities: eleven spellings and prefix literals outside sources

- Status: open
- Severity: medium
- Verdict (finders): DIVERGED
- Themes: contracts and shapes
- Wave: 3
- Finder sources: S8 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Constants first: register the cross-module prefixes in `sources` and replace the literals (`dec_` in native, `dec_baseline_` and `int_baseline_` in replay). Do not change any persisted id scheme. Pin every scheme with an input-to-id table test before touching it. Frozen schemes stay inline with the pin.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S8: Hash-derived identities: one pattern, eleven spellings, one collision-hardened outlier

- Verdict: DIVERGED
- Shared meaning: a stable id is `prefix + hex(sha256(material))` over caller-visible fields.
- Sites (material, separator, length):
  - internal/engine/internal/domain/version_write.go:109-118 - `LineageID`: length-prefixed material, full hex, `lin_` (its doc comment explains why plain concatenation is unsafe for free-text ids)
  - internal/engine/internal/domain/heartbeat.go:94 - `tmr_` + full hex, `\x00`-separated with a `agentic-stream/timer/v1` domain prefix
  - internal/situations/internal/domain/situations.go:221-223 - `sit_`/`occ_` + full hex, `\x00`-separated
  - internal/cognition/internal/domain/evaluation.go:84-88 - `trg_` + first 24 hex, `|` separated; timing.go:67-71 `SchedulerDedupeKey` raw bytes, `|` separated; correction.go:75-79 `ReconsiderationKey` full hex, `|` separated
  - internal/episodeledger/internal/domain/rejection.go:101-105 - `rej_` + full hex, `|` separated and includes raw `details` bytes
  - internal/eventlog/internal/domain/quarantine.go:25-33 - `payload:` + 24 hex and `q_` + 24 hex, `|` separated with the raw payload
  - internal/policy/internal/domain/commands.go:13 - idempotency key over `tenant|intent|type|target`; routing.go:78-81 `ApprovalNonce` over `approvalID|intentID`
  - internal/episodes/internal/domain/assembly.go:100 - `episodeID|schedulerItemID` admission key; internal/executor/native/internal/app/loop_tools.go:52 - `name|args`
  - internal/replay/internal/domain/baseline.go:253-256 - `shortKey` = first 16 hex; shadow_comparison.go:66 - `cmp_` + full hex of an existing digest
  - Prefix literals not in `sources`: `q_`, `rej_`, `lin_`, `tmr_`, `cmp_`, `occ_`, `payload:` ; literals duplicating existing constants: internal/executor/native/internal/domain/deterministic.go:27 `"dec_"` (fixture executor uses `sources.PrefixDecision` for the same id, internal/executor/fixture/executor.go:76-79), internal/replay/internal/domain/baseline.go:120,128,141 `"dec_baseline_"`/`"int_baseline_"`
- How they differ: separators `\x00`, `|`, length prefix; truncation 8/12/16/24 bytes or full; prefix source (`sources.Prefix*` constant vs literal). `LineageID`'s comment documents that `|`-style joins collide for free-text components, yet nine other ids join free-text-capable fields (event ids, details bytes, payload bytes, target) with `|`.
- Risk if left: an id scheme is a durable contract (idempotency keys, quarantine ids, nonces); each module re-decides collision safety, and a prefix typo is invisible to the compiler.
- Proposed canonical owner: `internal/sources` (layer 4, owns id prefixes and generators). Add the missing `Prefix*` constants and `sources.HashID(prefix string, parts ...string) string` (length-prefixed parts, full hex).
- Proposed fix: introduce the helper but do NOT change existing ids: keep each caller's current material/separator behind per-scheme functions in `sources` (or leave frozen schemes inline with a pinned test) and only replace literal prefixes with constants first; use `HashID` for new ids only. A full migration is a durable-identity change and needs a design decision.
- Behaviour to preserve: every id above is persisted or compared across restarts (`TestReconsideration*`, golden replays, `admission_key` uniqueness); output bytes must not change.
- Verification: add a table test per scheme pinning (input -> expected id) before touching anything, plus an architecture test forbidding `"xxx_"` prefix literals outside `internal/sources`.

## Outcome

Not started.
