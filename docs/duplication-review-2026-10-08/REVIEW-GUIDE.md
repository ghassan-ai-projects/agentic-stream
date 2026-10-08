# Review guide for PR #49

The change set is about 425 files. Reading it file by file does not find the
defects that matter, so the review is split by risk, and anything a machine can
prove is proved by a machine.

## 1. What the machine already proves

| Audit | Result against `main` |
| --- | --- |
| Migrations, protobuf, JSON schemas, testdata and golden files | migrations, protobuf, JSON schemas and testdata unchanged. Digests, ids and preimages that embed an instant DID change (nine-digit UTC text instead of trimmed RFC 3339): snapshot and state digests, heartbeat timer ids, rejection ids, outcome and command digests, approval `expires_at` and signing bytes, evaluation event ids, evidence `request_sha256` and token claims. No golden on `main` covered most of them; the pins that existed were edited with the change, and literal pins were added (section 5, R3 F1). The owner accepted the break under the no-backward-compatibility decision. See [import-design.md](import-design.md) "What moved". |
| `make ci-check` (lint, race tests, coverage floor, dead code, vulnerabilities, docs) and `make cross-compile` | pass, the same commands the CI workflow runs |
| Clone gate | 75 tokens, tests included; 0 hits |
| Layering | `allowedImports` additions fall from 49 to 17, each with a reason in [import-design.md](import-design.md); layer table equals `main` plus `kernel` and `storagetest` |
| `kernel` purity | `TestKernelStaysPure` and `TestKernelHasNoSubpackages` |
| Test functions | 1196 on `main`, 1297 now; 17 removed, each listed in section 3 with its successor |
| Failure checks in tests | 389 removed, 315 added; the net loss sits in files where open and setup checks moved into helpers (`storagetest.OpenTemp`, fixtures) |
| New escape hatches | 12 `//nolint:wrapcheck` added (11 removed); the lint config changed only for the clone gate |

## 2. What a person or an independent reviewer must judge

Behaviour changed on purpose in these places. Each cluster lists the paths, the
issues, and the question a reviewer answers. Review them in this order; skip
files that are only renamed calls.

| # | Cluster | Issues | Paths | Question |
| --- | --- | --- | --- | --- |
| R1 | Approval and risk | 001, 013 | `internal/contractsv1` (risk), `internal/policy`, `internal/actions` (authorization), `internal/approvalledger`, `internal/episodes/internal/domain/shadow.go`, `internal/executor/remote/internal/domain/request_fields.go` | Can an intent that needs approval still dispatch without a valid, unexpired approval? Does shadow scoring now say what live policy would do? |
| R2 | Episode lifecycle and dispatch | 004, 005, 014, 016, 017, 018 | `internal/episodeledger`, `internal/episodes`, `internal/runtime/internal/app/admission*`, `internal/replay/internal/store`, `internal/evidence/internal/store`, `internal/control` | Is anything other than an explicit `active` kept out of governance? Do live and replay select the same due items in the same order? Is any state set now narrower or wider than before? |
| R3 | Time | 002, 003, 027 | `internal/kernel`, every `*/internal/store`, `internal/engine`, `internal/eventlog`, `internal/control`, `internal/authority`, `internal/interlock` | Does any lease, timer, debounce or window comparison change result at a second boundary? Does every unreadable stored time fail closed? |
| R4 | Documents, digests and contracts | 006, 008, 019, 020, 021, 022, 025 | `internal/canonicaljson`, `internal/kernel/digest.go`, `internal/cognition` (prior documents), `internal/episodes` (reconsideration), `internal/notify`, `cmd/agentic-stream/effect_profile.go` and `run_shadow.go`, `internal/runartifact` | Do digests, ids and notification payloads stay byte-identical where claimed? Is the physical-actuation rule still enforced before any device connection? |
| R5 | Safety fixes of the last wave | 007, 015, 026 | see their Outcomes | Is the verifier stricter everywhere? Does every write path still fence on the owner? Do all executors classify a deadline the same way? |
| R6 | Tests and infrastructure | test speed, test dedup | `internal/storage/storagetest`, the fixtures in cognition, policy, operators, episodes, device, api, engine, eventlog, ingress, replay, runtime tests | Did each test keep its own distinct assertion? Does any test that reopens a database still use the same path? |
| R7 | Project rules | all | whole diff | `//nolint:wrapcheck` outside the two allowed cases, comments inside module internals, functions over 15 lines |

## 3. Removed tests and where their property went

| Removed test | Successor to confirm |
| --- | --- |
| `TestAdmissionDefaultsAndConflictReporting`, `TestAdmissionOwnsShadowDefaultAndRejectsConflictingLiveEpisode` | `TestAdmissionStoresDeclaredPolicyAndRejectsConflictingLiveEpisode`, `TestAdmissionRefusesAnUndeclaredDispatchPolicy` (DUP-005) |
| `TestAdmissionReadyRequiresWindowOpenAndUnexpired`, `TestAdmissionWindowAppliesNotBefore`, `TestAdmissionWindowRejectsUnparseableTimes` | the due-items tests in `internal/episodeledger` (DUP-016) |
| `TestContentionClassification`, `TestExpireWaitHonorsCancellation` | `TestWatchExpireHonorsCancellationWhileContended` (DUP-024) |
| `TestLeaseExpiredTreatsUnparseableAndAbsentAsExpired`, `TestOpennessAndTimeEncodings`, `TestStoredTimesParseInColumnOrder`, `TestTimeTextIsFixedWidthUTC`, `TestWatermarkRefusesUnparseableInputsInOrder` | the fail-closed store tests and `TestDurableTimeTextOrdersChronologicallyAndRoundTrips` (DUP-002, 003) |
| `TestOnlyR2IntentsNeedAnUnexpiredApproval` | `TestAuthorizationEpisodeFollowsTheLedgerDecisionPredicate` and the `RouteFor` table (DUP-001) |
| `TestReconsiderationRejectsPriorIdentityBeforeCommand` | `TestPriorDocumentsRejectIdentityBeforeCommand` (DUP-019) |
| `TestSchemaLoaderDeniesNetwork` | `TestCompileSchemaPolicy` (DUP-020) |
| `TestSerialEffectorVerificationRejectsMismatchedFanDuty`, `TestSerialEffectorVerificationRejectsMismatchedIndicatorValue` | `TestSerialEffectorVerificationRejectsMismatchedOutput` (two subtests) |
| `TestEpisodeAndAttemptStateFollowTheLedgerPredicates`, `TestEvidenceFenceVerdictsMatchTheLedgerIdentityRules` | `TestEpisodeStateIsReadFromTheLedgerForEveryLifecycleAndBinding`, `TestEpisodeWithoutAnAttemptOrWithoutARowIsNeverCurrent`, `TestAttemptIsInFlightOnlyWhileDispatchedOrRunning`, `TestAnAttemptThatIsNotTheFencedOneIsRefusedWhenRead` (fixed expectations, DUP-014) |
| `TestAdmitPendingLeavesAnExpiredItemUnadmitted` | `TestAdmitPendingExpiresAnItemPastItsExpiry` (the item now ends `expired`, DUP-016) |

## 4. How findings are handled

A reviewer reports findings with `path:line`, the failing scenario, and a
proposed fix. A fixer applies them in one commit per cluster, with a test that
fails first. Findings and their disposition are appended to the section below.

## 5. Findings

Filled in as clusters are reviewed.

### Review fixes: approval and risk (R1) and policy/actions/approvalledger wrapcheck (R7)

| Finding | Severity | Disposition | Pinned by |
|---|---|---|---|
| R1 F1 tie-break tests insert the larger id first | SHOULD-FIX | FIX, mutation-checked (dropping `approval_id DESC` now fails both) | `TestLatestApprovedOfIntentPicksOneRowAndBreaksDecidedAtTiesByLargerID` (case inserted last), `TestAuthorizationRecordsBindOneApprovedApprovalWhenDecisionsTie` (both orders) |
| R1 F2 shadow scoring under-reports approval for mixed decisions | SHOULD-FIX | FIX: strictest route first, then risk rank, then first position | `TestShadowScoreOfTwoIntentsIsTheStricterLiveRoute`, `TestShadowScoreOfMixedFlagsReportsTheFlaggedIntent`, `TestStrictestIntentFollowsTheRiskOrder` |
| R1 F3 / R7 F2 items 1, 3, 10 bare returns under `//nolint:wrapcheck` (policy `intent_reads.go`, actions `candidates.go`) | SHOULD-FIX | FIX: wrapped returns ("scan intent row", "parse intent expiry", "scan command outbox candidate"); callers already use `errors.Is`; no directive left in these files | `internal/policy/internal/store/intent_reads.go`, `internal/actions/internal/store/candidates.go`; existing store and app tests |
| R1 F4 `CheckApproval` returns nil for a denied or unknown route | NIT | FIX: denied route returns "risk policy denies the intent" | `TestApprovalIsCheckedExactlyWhereThePolicyRoutesToApproval`, `TestCheckApprovalRefusesAnUnrecognizedRiskClass` |
| R1 F5 stale and forbidden comment above `revalidateIn` | NIT | FIX: comment removed | `internal/actions/internal/app/authorize.go` |
| R1 F6 approval `expires_at` wire text changed from `main` | NIT | ACCEPTED and documented: pending approvals across the upgrade fail closed | DUP-001 and DUP-013 Outcome, `import-design.md` |
| R1 F7 unreadable approval expiry blocks evaluation | NIT | FIX in the evaluation path: the approval is expired with reason `approval_expiry_unreadable`; resolving it still refuses. The unreadable intent expiry that wedges the pending queue is R3 F2 and is not changed here | `TestExistingApprovalWithUnreadableExpiryIsExpiredWithAnAuditReason` |
| R6 F2 a corrupt `approvals.expires_at` is unpinned (parse error treated as "never expires" passes) | SHOULD-FIX | FIX, mutation-checked: a parse error as "never expires" in `PendingApprovalExpiry` or `AssertionBinding` now fails the store test and the evaluation test; the expiry boundary is pinned | `TestUnreadableApprovalExpiryRefusesOnlyItsOwnApproval` (corrupt row refused, neighboring approval on another intent reads fine), `TestExistingApprovalWithUnreadableExpiryIsExpiredWithAnAuditReason`, `TestResolvingAnApprovalWithUnreadableExpiryIsRefusedAndLeavesItPending`, `TestExistingApprovalExpiresExactlyAtItsDeadline` (now+1ns live, exactly now expired, now-1ns expired) |

R7 F2 items in `internal/approvalledger`: none present. Items outside policy, actions and approvalledger (eventlog, watch, storage, cognition, episodeledger, episodes) are left to their owners.

### Review fixes: scheduler queue, episode lifecycle and evidence fence (R2, R3 scheduler part, R6-F1)

| Finding | Severity | Disposition | Pinned by |
|---|---|---|---|
| R2 F1 / R3 F4 expired or never-due scheduler items stay `pending` forever and can exhaust `globalCapacity` | SHOULD-FIX | FIX: `PollSchedulerQueue` reports items with `expires_at <= now`; the admitter expires them (pending to `expired`, written by the ledger owner, reason recorded on the trigger evaluation) in an owner-fenced transaction. Replay does not write the transition | `TestAdmitPendingExpiresAnItemPastItsExpiry`, `TestExpiredItemsNoLongerFillGlobalCapacityOrTheNextPoll` (101 stale rows leave `pending`), `TestExpiredSchedulerItemsLeaveThePendingQueue` (second poll no longer reads them), `TestOnlyPendingItemsCountTowardGlobalCapacity` |
| R2 F2 debounce or cooldown at or above `expiresAfter` silently never runs | SHOULD-FIX | FIX, but not by rejecting at compile time: the canonical predictive-maintenance spec declares `cooldown: 30m` with `expiresAfter: 15m`, so a compile error would reject the MVP example and its pinned digests. `expiresAfter` now means the useful life after the item may start (`expires_at = max(now, not_before) + expiresAfter`), which keeps main's behaviour for these specs. Debounced items expire `debounce` later than before | `TestTimingKeepsAnItemUsefulForExpiresAfterOnceItMayStart` ("cooldown above expiry" is the example's shape), `TestPollExpiresAnItemAtItsExpiryInstantAndNotOneNanosecondBefore` |
| R2 F3 / R3 F1 (rejection part) the claim that rejection ids are unchanged is false | NIT | FIX the text: ids changed for whole-second instants (main hashed the trimmed RFC 3339 text); `import-design.md` corrected; both forms pinned | `TestRejectionIDIsPinnedForWholeSecondAndFractionalInstants` |
| R2 F4 hand-built specs and replay lose the implicit R1 risk ceiling | NIT | DOCUMENTED in the DUP-005 Outcome (fails closed; the compiler is the single place that defaults it) | DUP-005 Outcome |
| R2 F5 evidence fence tests compare the implementation with itself | NIT | FIX: fixed expectations per lifecycle, binding and attempt status, including higher fence, missing attempt row and unknown episode at completion | `TestEpisodeStateIsReadFromTheLedgerForEveryLifecycleAndBinding`, `TestEpisodeWithoutAnAttemptOrWithoutARowIsNeverCurrent`, `TestAttemptIsInFlightOnlyWhileDispatchedOrRunning`, `TestAnAttemptThatIsNotTheFencedOneIsRefusedWhenRead` |
| R2 F6 missing rows surface as different errors in two evidence/ledger reads | NIT | DOCUMENTED in the DUP-014 Outcome (all still refuse; asserted now) | `TestEpisodeWithoutAnAttemptOrWithoutARowIsNeverCurrent`, `TestAnAttemptThatIsNotTheFencedOneIsRefusedWhenRead` |
| R2 F7 comments inside module internals | NIT | FIX in episodeledger, cognition, runtime admission, evidence store and `episodes/internal/store/dispatch.go`: comments added since `main` stripped by an AST program (tool directives and package comments kept). The comments that `main` already had stay | `gofmt`, `golangci-lint`, `go test` |
| R3 F2 (scheduler part) one unreadable time fails admission for the whole tenant | SHOULD-FIX | FIX: the unreadable row is listed apart, expired with reason `unreadable <column>` and the tenant keeps admitting. Replay skips such a row | `TestUnreadableSchedulerTimeIsolatesOnlyThatRow` (created_at, not_before, expires_at), `TestAnUnreadableSchedulerTimeExpiresThatItemAndAdmissionContinues`, `TestPollSeparatesTheAdmittableItemFromThoseThatCanNeverRun` |
| R3 F5 (episodeledger queue part) queue parse unpinned | SHOULD-FIX | FIX: see R6-F1 | same tests |
| R6-F1 unreadable `scheduler_items` times unpinned (mutating the parse to accept anything passes) | SHOULD-FIX | FIX, mutation-checked: accepting the text of any one of the three columns fails `TestUnreadableSchedulerTimeIsolatesOnlyThatRow`; the boundary "expires exactly at now is expired, one nanosecond later is live" is pinned at domain and ledger level | `TestUnreadableSchedulerTimeIsolatesOnlyThatRow`, `TestSchedulerItemExpiresExactlyAtItsExpiryInstant`, `TestPollExpiresAnItemAtItsExpiryInstantAndNotOneNanosecondBefore` |

### Review fixes: time and documents (R3, R4)

| Finding | Severity | Disposition | Pinned by |
|---|---|---|---|
| R3 F1 / R4 F1 digests, ids and a signed wire form that embed an instant changed while the docs said they did not | BLOCKER | FIX: the claims in DUP-002, DUP-003, DUP-008 Outcomes, `import-design.md` (Groups 1 to 3 and a new "What moved" section) and section 1 of this guide now list what moved, say that no pinned golden covered most of it, and record the owner's acceptance under the no-backward-compatibility decision. The trimmed form was not restored. Literal pins added so any future change is deliberate | `TestMaterializationPinsTheDigestsThatEmbedInstants` (snapshot and state digest), `TestHeartbeatTimerPinsItsIdentityAndDueInstantText` and `TestTimerFiringPinsItsFiredInstantText` (timer id, `due_at`, `timer_fired_at`), `TestOutcomeDigestBindsSchemaValidDocuments` (outcome digest), `TestCommandDigestPinsTheCreatedAtText`, `TestApprovalAssertionSigningBytesPinTheExpiryText`, `TestApprovalNotificationKeepsItsSealedJSONShape`, `TestEvaluationEventPinsItsIdentityAndInstantText`, `TestTokenEncodingAndIntegrity` (claims), `TestEvidenceFingerprintEncodingIsStable` (request_sha256), `TestRejectionIDIsPinnedForWholeSecondAndFractionalInstants` (pass B) |
| R3 F2 a queue head with an unreadable time halts the stage (policy intents, actions outbox) | SHOULD-FIX | FIX: a pending intent whose expiry is unreadable is denied with reason `intent_expiry_unreadable` and leaves the queue; an outbox row whose lease is unreadable is selected as an expired lease and abandoned as an unknown outcome (logged with its reason) instead of blocking dispatch; `LoadOutboxLease` no longer errors on a foreign row. Single-item authorization (command authorization, approval resolution) still refuses. The scheduler part is pass B | `TestUnreadableIntentExpiryDeniesThatIntentAndLeavesTheQueue`, `TestFreshnessAndApprovalPrecedence` (unreadable case), `TestUnreadableLeaseExpiryIsExpiredAndNeverBlocksTheQueue`, `TestDispatcherAbandonsUnreadableLeaseAsUnknownOutcomeWithoutError`, `TestAdmitNamesWhyAnInFlightLeaseWasAbandoned` |
| R3 F2 engine checkpoint watermark, engine and cognition situation state, eventlog reads | SHOULD-FIX | DEFER: these are the state of one partition or one situation, not queue rows, and replay depends on them, so skipping one would change results silently. They refuse that entity (pinned below). Trigger: a partition quarantine feature | `TestUnreadableStoredTimesRefuseTheNextVersionOfThatSituationOnly`, `TestCorruptStoredTimesRefuseTheReadWithTheirColumn`, `TestCorruptCheckpointWatermarkRefusesTheRead` |
| R3 F3 SQL text comparisons of lease and expiry columns treat unreadable text as unexpired | SHOULD-FIX | FIX without a migration: every database opened through `storage` carries the SQL function `stored_time_ok(text)`, true only for text `kernel.FormatTime` writes (it calls `kernel.ParseStoredTime`, so Go and SQL agree). Liveness predicates (`lease_until > ?`, `expires_at > ?`) require it and reclaim predicates (`lease_until <= ?`, `expires_at <= ?`) also accept its negation, so an unreadable value is expired, never live. Sites: `runtime_owner` (claim, renew, hold), `outbox` (candidate, acquire, live, refresh), `watch_conditions` (expire, load, fire, target), `evidence_call_ledger` (complete, fail, reclaim). Limit: other SQL comparisons of time columns (`timers.due_at`, retention cutoffs, the event window) are unchanged; an unreadable `due_at` is never pre-expired | `TestStoredTimeFunctionAcceptsOnlyDurableTimeText`, `TestStoredTimeFunctionRefusesNullAndBlobs`, `TestParseStoredTimeAcceptsOnlyTheTextFormatTimeWrites`, `TestUnreadableLeaseTextIsNeverHeldOrRenewedAndCanBeClaimed`, `TestUnreadableStoredExpiryIsExpiredNotActive`, `TestUnreadableLeaseTextIsNeverOwnedAndIsReclaimed` |
| R3 F5 fail-closed behaviour is unpinned for most stores | SHOULD-FIX | FIX: tests now write unreadable text into the parsed columns and assert the resolution for actions (lease, authorization intent and approval expiry), policy (intent expiry), cognition (version times, last admission time), evidence (every token time), watch, control. Policy approval expiries are pinned by pass A and the scheduler queue by pass B | `TestAuthorizationRecordsRefuseUnreadableExpiryText`, `TestUnreadableStoredTimesRefuseTheNextVersionOfThatSituationOnly`, `TestDecodeScopeRefusesEveryUnreadableClaimTime`, plus the F2 and F3 tests above |
| R3 F4 stale scheduler items never leave `pending` | SHOULD-FIX | Pass B (see the R2 F1 / R3 F4 row above) | pass B |
| R3 F6 authority negative claim lease now defaults to one minute | NIT | REJECT: `Config.validate` refuses a negative `ClaimLease` (`internal/authority/service.go`), so `sources.OrLease` only ever sees zero | `internal/authority` config validation tests |
| R3 F7 fixtures write non-codec time text (`datetime('now')`, `...Z` without a fraction) | NIT | DEFER: those columns are never read through a parse in the tests that use them. Trigger: reusing such a fixture with a real store read | none |
| R3 F8 eventlog `event_time` now loses the producer's offset | NIT | FIX the text: listed in the DUP-002 and DUP-003 "What moved" paragraph | `TestDurableTimeTextOrdersChronologicallyAndRoundTrips` |
| R3 F9 `TransitionAttempt` identity check reads the wall clock | NIT | DEFER as in DUP-027: live composition configures the physical clock for both; trigger: a composition that runs the live pipeline on a virtual clock | none |
| R4 F2 prior documents are parsed inside the Situation-version transaction | SHOULD-FIX | FIX: an invalidated command whose stored decision, command or provider result cannot be read is recorded as a `rejected` evaluation of `prior_action_invalidated` (reason `prior documents unavailable: ...`, announced like any evaluation) and skipped; the corrected version, its other reconsiderations and `markVersion` proceed | `TestUnreadablePriorDocumentsRejectOnlyThatReconsideration` |
| R4 F3 the `awaiting` arm and the argument order of the rewritten soak queries are unpinned | SHOULD-FIX | FIX: a succeeded command with an `awaiting` verification counts in `awaiting_verification` and `unresolved_action_outcomes` and fails the verdict; `reconciled` and `observed` count zero; another tenant's commands count zero (tenant binds before the status arguments) | `TestSoakReportCountsOnlyAwaitingVerificationsOfSucceededCommands`, `TestSoakReportBindsTenantBeforeTheStatusArguments` |
| R4 F4 reconsider approval notices carry the full prior decision and command | NIT | ACCEPTED: approvers see what they are asked to approve again; the notice size has no contract limit. Noted in the DUP-019 Outcome | none |
| R4 F5 assembly no longer cross-checks the reconsideration row | NIT | FIX: `TakeReconsideration` refuses a delta whose `correction_version` differs from the scheduler item's version. The earlier test items now carry the version | `TestTakeReconsiderationRefusesADeltaForAnotherCorrectionVersion` |
| R4 F6 `storage.InClause` takes the column as raw SQL text | NIT | FIX: doc line on the exported symbol: the column must be a literal identifier, never data | `internal/storage/storage.go` |
| R4 F7 new comments inside module internals | NIT | DEFER to the owner: the files named sit beside older comments of the same kind and no gate enforces the rule; trigger: the owner wants the rule applied to existing code | none |
