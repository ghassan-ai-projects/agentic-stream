# Review guide for PR #49

The change set is about 425 files. Reading it file by file does not find the
defects that matter, so the review is split by risk, and anything a machine can
prove is proved by a machine.

## 1. What the machine already proves

| Audit | Result against `main` |
| --- | --- |
| Migrations, protobuf, JSON schemas, testdata and golden files | unchanged; no digest pin or fixture was regenerated |
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

## 4. How findings are handled

A reviewer reports findings with `path:line`, the failing scenario, and a
proposed fix. A fixer applies them in one commit per cluster, with a test that
fails first. Findings and their disposition are appended to the section below.

## 5. Findings

Filled in as clusters are reviewed.
