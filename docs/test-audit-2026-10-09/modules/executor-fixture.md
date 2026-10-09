# executor-fixture

Status: done
Round: 10

`internal/executor/fixture` is the deterministic demo executor. It has one package.

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/executor/fixture` | 92.1% | 92.1% | 1.5 s | 0.5 s | 3 top-level, 2 sub | 4 top-level, 3 sub |

Time is dominated by race-detector start-up and other workers' load; no test takes
more than a few milliseconds. The uncovered 8% are the digest and seal error
branches, unreachable for a well-formed intent.

## Findings and changes

### Removed
- `TestFixtureExecutorConforms`: renamed (below), not dropped.
- `TestFixtureRejectsMalformedRequest`: replaced by `TestFixtureRefusesAMalformedRequest`, which now also asserts the error (`unmarshal request`) and not just `err != nil` (T4).

### Renamed or moved
- `TestFixtureExecutorConforms` → `TestFixtureExecutorConformsToTheExecutorPort` (T3).
- `TestFixtureDecisionPreservesIdentityAndDigest` (table of two cases) → split into `TestFixtureProposesATicketForTheSnapshotPhaseAndTrigger` (what the decision says) and `TestFixtureDecisionPreservesIdentityAndDigest` (what it is bound to).

### Improved
- T4: the old table asserted identity and digest only. It never checked the intent the fixture exists to produce. The new table asserts, for a bound snapshot, a missing snapshot and fields of the wrong JSON type: the summary, the `reason` and `entity_id` parameters, intent type `create_maintenance_ticket`, risk `R1`, `intent_id` prefix, a valid intent digest (`contractsv1.VerifyIntentDigest`), and byte-identical output on a second call.
- T6/T9: parallel, `t.Context()`, one `fixtureRequest` builder; no `bytes.Equal`-then-`Fatalf("%v")` with a nil error.

### Added
- The intent assertions above (behavior the fixture owns and nobody tested).

### Speed
- Nothing slow.

## Production code touched
- none

## Invariants proven here
- 6: the fixture can only return an `episodes.Outcome` holding a Decision and a typed Intent; it has no effect handle. Proven by `TestFixtureProposesATicketForTheSnapshotPhaseAndTrigger` (the proposal is data with a verifiable digest) and by the import gate `TestReasoningAndReplayCannotReachEffectImplementations` (architecture, round 1).
- 5: `TestFixtureDecisionPreservesIdentityAndDigest` (the Decision is bound to the request's snapshot digest, episode, attempt and fence).

## Open items
- `Execute(ctx, nil)` panics (nil dereference of `req.RequestJSON`); the native and remote executors return `episode request is required`. The runner never passes nil, so this is not a test-only concern, but the Executor port does not say which is right. Not changed (production behavior). A `nil request` case belongs in `executorconformance` once the port states it.
- `Execute` ignores its context (`_ = ctx`), so the fixture cannot run `RunCanceled`. It is instantaneous; recorded so the conformance suite's cancellation case is not assumed to cover it.
