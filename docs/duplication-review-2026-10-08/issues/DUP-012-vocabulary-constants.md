# DUP-012: Approval and completeness vocabularies re-typed as literals although the owner exports constants

- Status: open
- Severity: low
- Verdict (finders): REAL
- Themes: business rules
- Wave: 3
- Finder sources: R12 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Re-export the approvalledger constants through its facade and the operators completeness constants through their facade, then replace the literals in policy, cognition and actions. Spellings must not change.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report R12: Stringly vocabularies whose owner already exports constants (approval, completeness, id prefixes)

- Verdict: REAL
- Shared meaning: approval statuses and reason codes, completeness states, deterministic-id prefixes.
- Sites:
  - Approval: owner internal/approvalledger/internal/domain/approval.go:5-16 (`StatusExpired`, `StatusDenied`, `ReasonExpired = "approval_expired"`, `ReasonWithdrawn`, `WithdrawalConflict = "situation_version_conflict"`) - NOT exported through the approvalledger facade (grep of approvalledger.go: only `Withdrawal` types). Copies: internal/policy/internal/app/approval.go:50,57 (`"denied"`, `"situation_version_conflict"`), :63,67 and internal/policy/internal/app/evaluate.go:111,114 (`"expired"`, `"approval_expired"`), approval.go:38-40 (`"stale"`, `"expired"`), internal/policy/internal/app/approval_presentation.go:21 (`"pending"`), internal/cognition/internal/store/tx.go:52 (`Reason: "situation_version_conflict"`), policy store SQL (approval_lookup.go:15,27,46, pending_approval.go:12) and actions store authorization.go:17-18 (`a.status = 'approved'`).
  - Completeness: owner internal/operators/internal/domain/types.go:84-92 (typed consts, partly re-exported operators.go:29-33). Copies: internal/policy/internal/domain/rules.go:52 (`"provisional"`, `"uncertain"`), internal/cognition/internal/domain/correction.go:37 (`"corrected"`).
  - Id prefixes: owner internal/sources/internal/domain/ids.go:10-26 (`PrefixDecision` etc., 24 uses elsewhere). Copies: internal/executor/native/internal/domain/deterministic.go:27 (`"dec_" + EpisodeID`, while fixture executor.go:99 uses `sources.PrefixDecision + EpisodeID`), internal/replay/internal/domain/baseline.go:120,128,141 (`"dec_baseline_"`, `"int_baseline_"`), plus unregistered prefixes defined locally: `occ_` (situations/domain/situations.go:223), `cmp_` (replay shadow_comparison.go:66), `q_` (eventlog/domain/quarantine.go:33), `lin_` (engine/domain/version_write.go:117), `tmr_` (engine/domain/heartbeat.go:96), `rej_` (episodeledger/domain/rejection.go:104).
- How they differ: copies agree today; the same reason string is a literal in the writer (approvalledger writes `ReasonExpired` into the approval row) and in the notification/outcome built by policy, so a rename of the code in one place splits the audit trail from the notification.
- Risk if left: renaming a reason or adding an approval status needs a repo-wide grep; a typo is not caught by the compiler.
- Proposed canonical owner: approvalledger facade (policy/store already imports it; policy/app would need an edge; cognition store already imports it); operators for completeness (policy domain would need an edge; alternatively expose through `situations`, which cognition already imports); `sources` for prefixes (native and replay domains need the edge; replay/domain already imports sources, native deterministic provider needs one).
- Proposed fix: re-export the constants from the facades and replace literals; register the six local prefixes in sources/ids.go only if they are cross-module (they are not: leave them).
- Behaviour to preserve: all spellings; deterministic ids (`dec_baseline_<key>` etc.) feed digests in replay tests.
- Verification: policy approval tests, cognition supersession tests, native deterministic provider tests, replay baseline tests. New: none needed beyond compile-time use.

## Outcome

Not started.
