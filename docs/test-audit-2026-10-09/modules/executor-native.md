# executor-native

Status: done
Round: 10

`internal/executor/native` is the in-process Go executor: a bounded model/tool loop
(`internal/app`), pure rules (`internal/domain`), the SQLite evidence tool
(`internal/store`) and the OpenAI-compatible provider (`internal/transport`).

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top / sub) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `executor/native` (facade) | 100.0% | 100.0% | 1.5 s | 1.6 s | 1 / 0 | 2 / 2 |
| `executor/native/internal/app` | 82.1% | 98.7% | 1.6 s | 1.7 s | 16 / 3 | 15 / 26 |
| `executor/native/internal/domain` | 80.0% | 99.1% | 1.5 s | 1.7 s | 9 / 0 | 11 / 34 |
| `executor/native/internal/store` | 81.6% | 84.2% | 2.9 s | 2.0 s | 4 / 0 | 4 / 0 |
| `executor/native/internal/transport` | 87.6% | 95.8% | 1.5 s | 1.5 s | 9 / 4 | 11 / 17 |

Times other than `store` are race start-up under load from the other workers. The slowest test in the module went from 1.41 s
(`TestEvidenceToolByteBoundKeepsTheLongestFittingPrefix`) to 0.34 s (the evidence store tests, which open a migrated database); no other test takes more than 0.03 s. Test-hygiene findings: `app` 15 and `transport` 7 before, 0 after.

## Findings and changes

### Removed
- `TestDeterministicNativeExecutorConforms`: moved to the facade as `TestNativeExecutorConformsToTheExecutorPort` (the port contract is proven where the executor is constructed, not in an internal app test) and extended with `RunCanceled`.
- `TestNativeExecutorRecordsCancellationAsTheRunnerDoes`: now the conformance case `RunCanceled`; the native-specific part (the provider is never called) is `TestExecuteNeverCallsTheProviderOnACanceledContext` (T2).
- `TestNativeExecutorFailsInterruptImmediately`, `TestNativeExecutorEnforcesReportedUsageBudgets` (3 subtests), `TestNativeExecutorRejectsMissingUsageForUsageBound`, `TestNativeExecutorCapsProviderRetries`, `TestNativeExecutorFailsClosedOnAnOversizedResult`: merged into one table, `TestExecuteFailsTheAttemptWithTheReasonOfTheBoundItBreaches` (18 cases). The old oversized-result test asserted only `Status != produced`; the table asserts the reason (T4).
- `TestNativeExecutorRejectsUnboundedRequest`: `TestExecuteRefusesAnUnboundedRequestWithoutCallingTheProvider`.
- `TestOpenAICompatibleProviderRejectsJSONWithoutUsage`, `...RejectsSSEWithoutUsage`, `...RejectsZeroTimeoutClient`: rows of `TestStreamRefusesWhatItCannotAccountFor`.
- `TestEvidenceToolRejectsOutOfScopeArguments` (store): the rules (foreign entity, bad from/until, empty window, bad JSON) are domain rules and now live in `TestEvidenceQueryRejectsArgumentsOutsideTheScope`; the store keeps `TestEvidenceToolRefusesWhatItIsNotScopedFor` (wiring: foreign entity and bad JSON reach the domain rule, unscoped tools answer nothing) (T2).
- `TestFacadeConstructsExecutorAndEvidenceTool`: its `native.ErrInterrupt == nil` assertion proved nothing (T10); the constructor is proven by the conformance test, the evidence tool name by `TestFacadeBuildsTheScopedEvidenceTool`.
- Local copies of provider types (`interruptProvider`, `retryProvider`, `usageProvider`, `countingProvider`, `missingUsageProvider`, `usageThenCancelProvider`, `timedRetryProvider`, `alwaysRetryProvider`, `cancelDuringRetryProvider`): replaced by one `scriptedProvider` driven by a `respondFunc`.

### Renamed or moved
- `native_test.go` (466 lines, one file for everything) → `bounds_test.go`, `loop_test.go`, `cancellation_test.go`, `configuration_test.go`, `fixtures_test.go` (T3, T11).
- `loop_boundary_test.go` → `loop_tools_test.go` (it tests `loop_tools.go`).
- `rules_test.go` (domain) → `budget_test.go`, `request_test.go`, `evidence_query_test.go`, `providers_test.go`.
- `TestOpenAICompatibleProvider*` → `TestStream*`; `openai_internal_test.go` → `openai_client_test.go`.

### Improved
- T5: `time.Sleep(p.delay)` in the late-provider test is gone. The provider now blocks on `<-ctx.Done()` and then returns its response, which is exactly the "late response after the wall time" the production comment describes, with no real delay and no race. The `time.After(time.Second)` guards in the two cancellation tests became `awaitSignal` on `t.Context()`. The one remaining real-time assertion is a lower bound: the retry backoff is at least the production 10 ms, which a monotonic timer guarantees.
- T6: every test and subtest is parallel. `openai_internal_test.go` mutated the package-level `defaultOpenAIHTTPClient` (and was therefore not parallel); the default-client rule is now checked through `boundedHTTPClient()` without mutation.
- T4: error tests assert the reason or message; `errors.As` for `RetryableError`; no `err != nil` alone.
- T9: `executorconformance.SetBudget` replaces the local `replaceBudget`; `newExecutor`, `execute`, `toolReturning`, `providerRespondingWith` are the shared builders.
- Comments removed from tests (no-comments rule).
- Store byte-bound test: instead of 700 queries (one per byte limit) it asserts each exact boundary (a limit equal to the encoded size keeps the row, one byte less drops it) for each prefix: same property, 1.35 s → under 0.3 s.

### Added
- `TestExecuteFeedsToolObservationsToTheNextModelCall`: the tool receives the model's arguments; the next model call carries the observation with call id, tool name, result and bytes; the first call has none.
- `TestExecuteReportsAFailedToolAndAnEmptyResultAsObservations`: a failing tool becomes a `tool_failed` observation and the attempt continues; an empty result is `null` counted at the tool's own byte report.
- `TestExecuteAsksOnceForARepairOfAnInvalidDecision`: the repair flag and reason on the second call.
- `TestExecuteRetriesARetryableProviderErrorAfterABackoff`.
- `TestNewRejectsAnIncompleteConfiguration`, `TestExecuteRefusesWhatItCannotRun`, `TestToolFactoryToolsAreBuiltFromTheTrustedRequest` (new, builds tools from the request, ignores nil and unnamed tools).
- Table cases never tested before: unlisted tool, tool arguments not JSON, tool call without arguments, tool-calls budget, model-calls budget, tool result not JSON, tool interrupt, provider failure, retryable error without allowance, no decision, decision rejected after repair.
- domain: `TestRequestBudgetRequiresAFiniteBound` (was 0%), `TestEvidenceQueryNarrowsButNeverWidensTheScope` (wider `max_rows` and `max_bytes` are ignored), `TestToolDefinitions...` (schema, description, `type` fallback, sort).
- store: tenant isolation. The seed now holds an event of the same entity under another tenant; the tool must not return it.
- transport: `TestStreamSendsABoundedStructuredRequest` (system prompt, user content, repair reason, tool declaration, strict `json_schema`, bearer key), `TestStreamSendsNoCredentialWithoutAnAPIKey...`, `TestStreamDecodesAJSONResponse`, `TestStreamMarksOverloadAndServerFailuresRetryable` (429/500/503 retryable, carry the provider message), non-retryable 400, no choices, malformed JSON, malformed SSE event, nil provider, missing endpoint/model.

### Speed
- `store`: byte-bound property test, see above. `app`: nothing sleeps; the slowest test takes 0.03 s.

## Production code touched
- none

## Invariants proven here
- 6: the model cannot execute effects. `TestExecuteFailsTheAttemptWithTheReasonOfTheBoundItBreaches/unlisted_tool` (a tool outside the allow-list fails the attempt with `tool_not_allowed`), `.../tool_arguments_that_are_not_json`, `TestFailedToolCallCannotBeRepeated` (internal), `TestEvidenceToolRefusesWhatItIsNotScopedFor` and `TestEvidenceToolReturnsOnlyTheScopedTenantEntityWindow` (the only tool is read-only and scoped to the request's tenant and entity), `TestValidateDecisionBindsIdentityAndAllowedIntents` (an intent outside `allowed_intent_types` is refused before it can leave the executor), `TestToolFactoryToolsAreBuiltFromTheTrustedRequest`. The provider adapter sends only its own API key header and only when configured: `TestStreamSendsABoundedStructuredRequest`, `TestStreamSendsNoCredentialWithoutAnAPIKey...`.
- 5: `TestRequestBudgetRequiresAFiniteBound`, `TestExecuteRefusesAnUnboundedRequestWithoutCallingTheProvider`, the budget table.

## Open items
- `internal/executor/native/internal/app/loop.go:81` has the comment "classifies a domain.Failed model call", a leftover from an automated rename (it should read "failed"). Production comments in `internal/**` also break the no-comments rule; not touched in a test round.
- The retry backoff (`providerRetryBackoff`, 10 ms) is a constant read from the real clock; a test cannot shorten or virtualize it. Its lower bound is asserted; making it injectable would be a production change.
- `store` is at 84.2%: the remaining branches are `json` encode errors that cannot occur for the values built.
