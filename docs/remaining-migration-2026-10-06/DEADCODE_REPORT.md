# Dead, unreachable and test-only code (2026-10-06)

> **Superseded** by [the unfinished work review (2026-10-08)](../unfinished-work-review-2026-10-08/README.md):
> every item here was completed, deleted or moved to test support, and
> `make deadcode` now fails on any production function reachable only from tests.

Method: `deadcode ./...` (reachability from `main`, production only) and `deadcode -test ./...` (reachability including tests), plus a token scan for exported types, constants and variables. `golangci-lint` `unused` reports nothing (no unused unexported code).

## Headline

- `deadcode -test ./...` reports **0** unreachable functions: nothing is dead in the strict sense. Every function that production cannot reach is called by a test.
- **189** functions and methods are reachable only from tests (listed below by module).
- **4** exported types, constants or variables are referenced only by tests; **6** are referenced nowhere.

## Functions and methods reachable only from tests

Class: **lever** = a designed capability with no production caller (wire or remove); **support** = test or reference support; **leftover** = wrapper or helper nobody needs.

| Module | Count | Class | Symbols |
| --- | --- | --- | --- |
| `replay` | 93 | lever (shadow, recorded, counterfactual, baseline modes; `RunMode`, `RunNTimes`) | `AdmitCapabilities`, `AdmitSimulatedCommand`, `AllHashesEqual`, `BaselinePolicy.ExecuteBaseline`, `BaselinePolicy.selectIntent`, `BuildComparison`, `Capabilities.Validate`, `Capabilities.requireShadowExecutors`, `DecodeRecordedDecision`, `EpisodeKey`, `EpisodeViews`, `IndexRecordedEntries`, `MatchRecordedMetadata`, `NewBaselinePolicy`, `NewDeterministicBaseline`, `ParseParameterSchema`, `ReplayEpisode.Keyed`, `RequireEmptyRecordedLedger`, `RequireExpectedRecordedKeys`, `RunMode`, `RunNTimes`, `ShadowEntityID`, `ShadowInput.Clone`, `ShadowRules.ValidateOutput`, `ShadowRules.ValidatePair`, `ShadowRules.bindOutput`, `ShadowRules.validateDecision`, `Store.EpisodeWorklist`, `Store.RecordShadowComparison`, `Store.RecordedSnapshotDigest`, `Store.ShadowSnapshot`, `ValidateRecordedAttempt`, `ValidateRecordedSnapshot`, `VerifiedSnapshot`, `VerifyRecordedDocument`, `VerifyRecordedEntry`, `WithRunDirectory`, `WorkerAwareMode`, `applyCapabilities`, `applyCounterfactual`, `applyPairedShadow`, `applyRecorded`, `assembleComparison`, `baselineEnumValue`, `baselineIntents`, `baselineManifestDigest`, `baselineParameters`, `bindShadowDecisionDigest`, `bindShadowManifest`, `canonicalShadowDecision`, `collectReplayEpisodes`, `columnValues`, `compareShadowEpisode`, `comparisonDocument`, `comparisonRecord`, `compileShadowCatalog`, `compileShadowRules`, `decodeBaselineSnapshot`, `enumValue`, `evaluateShadowPair`, `fillBaselineEnums`, `finalizeBaselineOutput`, `indexedRecordedEntries`, `inputDigests`, `loadShadowInput`, `matchRecordedEpisode`, `matchRecordedEpisodes`, `newBaselineDecision`, `newBaselineIntent`, `newShadowInput`, `outputDifferences`, `outputProvenance`, `parseParameterProperty`, `propertyEnum`, `recordShadowComparison`, `recordedEntries`, `repeatReplay`, `requireReplayCapability`, `requiredBaselineParametersPresent`, `runCapabilityMode`, `runDeterministicMode`, `sealComparison`, `setBaselineIntents`, `shadowAllowedTypes`, `shortKey`, `simulateCommand`, `simulateCommands`, `validateRecordedDecision`, `verifySnapshotDigest` |
| `worker` | 35 | support (reference `Server`, validators registered only in tests) | `DialEpisodeWorkerSocket`, `DialEvidenceSocket`, `Limits.Resolved`, `NewHandshakeResponse`, `NewStreamValidator`, `RequireFeatures`, `Server.Execute`, `Server.Handshake`, `Server.concludeStream`, `Server.executeHandler`, `Server.executeStream`, `Server.limits`, `Server.now`, `StartedEvent`, `StreamValidator.Check`, `StreamValidator.Record`, `StreamValidator.Terminated`, `StreamValidator.checkOrder`, `StreamValidator.checkPayload`, `StreamValidator.checkSize`, `ValidateHandshake`, `ValidateRequest`, `WireError`, `WireErrorf`, `applyWallBudget`, `boundedExecutionContext`, `guardedStream.emit`, `validateDeadline`, `validateEvidenceEndpoint`, `validateRequestContext`, `validateRequestIdentity`, `validateRequestProtocol`, `validateRequestShape` |
| `eventlog` | 14 | lever (quarantine release and redrive, `RecordGap`) | `EventLog.Quarantine`, `EventLog.RecordGap`, `EventLog.RedriveQuarantine`, `EventLog.ReleaseQuarantine`, `Gap.Valid`, `Service.RecordGap`, `Service.RedriveQuarantine`, `Service.ReleaseQuarantine`, `Service.redrive`, `Store.RecordGap`, `Unit.MarkRedriven`, `Unit.ReleaseQuarantined`, `Unit.ReleasedEnvelope`, `ValidRelease` |
| `executor/native` | 11 | support / leftover (batch runner, memory artifact store) | `MemoryArtifactStore.Get`, `MemoryArtifactStore.Put`, `NewMemoryArtifactStore`, `RunBatch`, `RunBatchJSON`, `executeBatchCell`, `producedBatchCell`, `settleBatchCell` |
| `contractsv1` | 7 | support (conformance fixtures) | `ConformanceInvalidFrames`, `ConformanceValidFrame`, `ConformanceValidMessageTypes`, `invalidFrame`, `loadConformanceFrame`, `schemaForInvalidName` |
| `notify` | 7 | lever (`Prune` retention never scheduled) | `CheckRetention`, `RetentionCutoff`, `Service.Prune`, `Tx.DeleteExpiredTombstones`, `Tx.DeleteRetiredNotifications`, `Tx.RetireNotifications` |
| `testsupport` | 6 | support | `FixtureRequest`, `Run`, `checkProducedOutcome`, `fixtureRequestJSON`, `mustDocument`, `ticketSchema` |
| `engine` | 5 | leftover (per-partition run path) | `Service.applyPartitionRecords`, `Service.drainPartition`, `Service.readPartitionRecords`, `Service.run`, `Service.runBatch` |
| `interlock` | 3 | lever (`Set`: nothing can trip the interlock) | `Set`, `ValidateChange` |
| `api` | 2 | support (facade wrappers used by tests; production composes `NewRuntimeHandler`) | `NewHealthHandler`, `NewSSEHandler` |
| `authority` | 2 | leftover (facade wrappers) | `ParseReconciliationEvidence`, `PhysicalEvidenceComplete` |
| `spec` | 2 | support (event-schema seeding helpers used by `eventlog` and `ingress` tests) | `EventSchemaJSON`, `RegisterEventSchema` |
| `control` | 1 | support (`SetCostLimit`) | `SetCostLimit` |
| `episodes` | 1 | leftover/lever (`CompileIntentCatalog`, used by replay shadow when wired) | `CompileIntentCatalog` |

## Exported types, constants and variables used only by tests

| Symbol | Defined in |
| --- | --- |
| `CommandDispatching` | `internal/actions/internal/domain/status.go` |
| `CommandPending` | `internal/actions/internal/domain/status.go` |
| `OutboxPending` | `internal/actions/internal/domain/status.go` |
| `StatusDenied` | `internal/approvalledger/internal/domain/approval.go` |

Caveat: the scan matches identifiers, so enum values that production only writes as string literals (for example SQL status strings) can look unused. Verify before deleting.

## Exported types, constants and variables referenced nowhere

| Symbol | Defined in |
| --- | --- |
| `ClassificationConfidential` | `internal/contractsv1/internal/domain/envelope.go` |
| `ClassificationPublic` | `internal/contractsv1/internal/domain/envelope.go` |
| `ClassificationRestricted` | `internal/contractsv1/internal/domain/envelope.go` |
| `StatusActive` | `internal/watch/internal/domain/condition.go` |
| `StatusDisabled` | `internal/watch/internal/domain/condition.go` |
| `StatusPending` | `internal/approvalledger/internal/domain/approval.go` |

Also see [EXPOSURE.md](EXPOSURE.md) for exported symbols that no other package uses (kept inside each module) and [DEADCODE.md](DEADCODE.md) for the earlier decisions per area.

Nothing was deleted; the owner decides per row.
