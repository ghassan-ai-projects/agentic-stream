# Semantic duplication review (2026-10-08)

Status: 30 issues written; fixes in progress. Token-level clones are already gated by `dupl` at 75
tokens, tests included. This review covers duplicated functionality: the same
business rule, table access, constant, data shape or mechanism written in more
than one place, so that one change needs several edits and can diverge.

## Method

1. Four read-only finders each took one theme: persistence (tables and SQL),
   business rules and constants, contracts and data shapes, and mechanisms.
2. Each candidate cites every site as `path:line`, states the shared meaning,
   and is classified `REAL` (identical meaning, N edits per change) or
   `DIVERGED` (already differs; the difference may be a latent bug).
3. A candidate becomes an issue only after the reviewer re-read its sites.
   Rejected candidates are listed in [rejected.md](rejected.md) so the ground is
   not covered twice.
4. One fixer works one issue. It consolidates onto the proposed canonical owner,
   keeps identities, digests and error text, adds a test that pins the shared
   rule, and records the outcome in the issue.

## Rules for every fix

- Follow [AGENTS.md](../../AGENTS.md): dependency direction, table ownership
  (`durableOwners`), no comments inside module internals, errors wrapped with
  `%w`, Go edits through Go tooling.
- Behaviour does not change unless the issue says the divergence is a bug; then
  the fix names the chosen behaviour and has a regression test for it.
- Add an `allowedImports` edge only when the issue names it; use the AST tool
  pattern for `architecture_test.go` edits.
- `make ci-check` passes. The issue file gets its Status, Outcome and Commit
  filled in; the index below is updated.

## Issue file layout

`issues/DUP-NNN-short-title.md`

| Section | Content |
| --- | --- |
| Header | Status (`open`, `in progress`, `fixed`, `wont-fix`), Verdict (`REAL`, `DIVERGED`), Theme, Severity |
| What is duplicated | The functionality in one paragraph |
| Sites | Every site as `path:line`, one line each |
| Divergence | Where the copies already differ and whether it is intentional |
| Risk | What drifts on the next change |
| Canonical owner | Module, and why the dependency direction allows it |
| Fix | What moves, what is deleted, what callers do |
| Preserve | Identities, digests, error text and ordering that must not change |
| Verification | Existing tests that cover it, and the new test that pins it |
| Outcome | Filled in by the fixer: what changed, commit, anything deferred |

## Index

| Issue | Title | Severity | Status | Wave |
| --- | --- | --- | --- | --- |
| [DUP-001](issues/DUP-001-risk-class-and-approval-requirement.md) | Risk class order, routes and "needs approval" are decided in eight places | high (probable bug) | fixed | 1 |
| [DUP-002](issues/DUP-002-timestamp-parse-format-and-expiry.md) | Durable timestamps: one writer, one parser, one expiry rule (layout unchanged) | medium | open | 2 |
| [DUP-003](issues/DUP-003-ordered-timestamp-columns.md) | Text-compared timestamp columns mix fixed-width and variable-width encodings | high (latent bug) | needs-decision | - |
| [DUP-004](issues/DUP-004-episode-lifecycle-and-attempt-state-sets.md) | Episode lifecycle and attempt state sets are re-spelled as literals and SQL lists | high | fixed | 2 |
| [DUP-005](issues/DUP-005-dispatch-policy-and-episode-vocabulary.md) | Dispatch policy (shadow/active), kind and lane vocabularies; unset must mean shadow everywhere | high (fail-open polarity) | fixed | 2 |
| [DUP-006](issues/DUP-006-unresolved-command-outcomes.md) | The "unresolved command outcome" set differs between actions and the soak report | high (probable bug) | fixed | 1 |
| [DUP-007](issues/DUP-007-stored-document-verification.md) | Stored intent/decision/snapshot/command verification is implemented separately in twelve places | high | open | 3 |
| [DUP-008](issues/DUP-008-digest-primitives.md) | Digest plumbing: Digest-then-Decode, Marshal-then-Digest, raw SHA-256 and hand-built sha256: strings | medium | open | 2 |
| [DUP-009](issues/DUP-009-episode-request-document.md) | The episode request_json document is written as a map and decoded by seven readers; the budget is declared four times | high | open | 3 |
| [DUP-010](issues/DUP-010-situation-state-document.md) | The situation state document is written as a map in situations and read as a struct in engine | medium | open | 3 |
| [DUP-011](issues/DUP-011-hash-derived-ids-and-prefixes.md) | Hash-derived identities: eleven spellings and prefix literals outside sources | medium | open | 3 |
| [DUP-012](issues/DUP-012-vocabulary-constants.md) | Approval and completeness vocabularies re-typed as literals although the owner exports constants | low | open | 3 |
| [DUP-013](issues/DUP-013-approvals-lookups.md) | pending and latest-approved approval lookups are written in policy and actions | medium | fixed | 2 |
| [DUP-014](issues/DUP-014-episode-fence-and-attempt-reads.md) | Episode fence and attempt-status point reads are re-written in evidence and episodes | medium | open | 2 |
| [DUP-015](issues/DUP-015-runtime-owner-and-ownership-fence.md) | The runtime_owner lease check is re-implemented in episodeledger and the ownership fence port is declared about ten times | medium | open | 3 |
| [DUP-016](issues/DUP-016-due-scheduler-items-live-vs-replay.md) | Which pending scheduler items are due is written twice, with different rules, for live and replay | high | needs-decision | - |
| [DUP-017](issues/DUP-017-coalesce-predicate.md) | The trigger open-item predicate is selected by cognition and updated by episodeledger as two statements | low | open | 2 |
| [DUP-018](issues/DUP-018-episode-admission-record.md) | The episode admission columns have four parallel struct and column-list views | medium | open | 3 |
| [DUP-019](issues/DUP-019-prior-outcome-document.md) | The prior-outcome document and invalidated-command chain are built in cognition and again in episodes | medium | needs-decision | - |
| [DUP-020](issues/DUP-020-offline-schema-compiler.md) | The offline JSON-Schema compiler is copied in four modules | medium | fixed | 1 |
| [DUP-021](issues/DUP-021-sqlite-error-classification.md) | SQLite constraint classification lives in episodeledger and depends on message text | low | fixed | 1 |
| [DUP-022](issues/DUP-022-cmd-reimplements-owner-rules.md) | cmd re-derives rules and shapes owned by device, watch and replay | medium | fixed | 1 |
| [DUP-023](issues/DUP-023-numeric-defaults.md) | One-minute lease, capability TTL, evidence read budget and tenant default stated in several modules | low | fixed | 2 |
| [DUP-024](issues/DUP-024-detached-context-and-busy-retry.md) | Detached 5 s persistence idiom (five sites) and watch retry loop that copies storage retry | low | fixed | 2 |
| [DUP-025](issues/DUP-025-notification-identity.md) | approval.withdrawn is built in two places with different payloads; notification ids and sources drift | medium | fixed | 1 |
| [DUP-026](issues/DUP-026-deadline-and-cancel-classification.md) | Execution deadline and cancel are classified three ways across executors | medium | open | 3 |
| [DUP-027](issues/DUP-027-clock-defaulting.md) | Nil-clock defaulting and wall-clock reads that bypass the injected clock | low | open | 2 |
| [DUP-028](issues/DUP-028-rows-affected-checks.md) | The exactly-one-row check after a fenced write has four error policies | low | open | 3 |
| [DUP-029](issues/DUP-029-periodic-loops.md) | Periodic loop and shutdown-predicate copies across runtime, episodes, cmd and api | low | open | - |
| [DUP-030](issues/DUP-030-idempotent-insert-or-compare.md) | Insert-or-compare idempotent append repeated in notify, policy and watch | low | open | - |

Statuses: `open` (ready for a fixer), `needs-decision` (a stored contract or
product choice is needed first), `in progress`, `fixed`, `wont-fix`.

Waves group issues that touch different modules so fixers can run side by side.
A fixer in a later wave starts only after the earlier wave is merged and green.
Issues not scheduled in a wave are either waiting on a decision or low value.
