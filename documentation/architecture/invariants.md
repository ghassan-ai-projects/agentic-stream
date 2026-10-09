# Product invariants

These are release-blocking properties, not aspirations. A feature that weakens
one of them requires a design decision and new evidence before it can be
accepted.

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. A Situation version is immutable after publication.
4. Deterministic state changes are serial per virtual partition.
5. Every episode is bound to one immutable Situation snapshot and finite budget.
6. A model can read evidence and propose typed Intents; it cannot execute
   effects.
7. Policy revalidates every Intent against current state immediately before
   dispatch.
8. Cross-boundary work uses stable identities, inbox/outbox records, and
   idempotency.
9. Replay never performs external effects unless an explicit, separate
   simulation mode is selected.
10. Every admitted, deferred, coalesced, rejected, canceled, and expired
    cognitive opportunity is explainable from durable records.

## How to use the invariants

When changing a boundary, identify which invariant it touches, name the durable
record or test that proves it, and include the failure path. The focused tests
are organized by package, while the design rationale is in
`docs/design/TECHNICAL_DESIGN.md`.

## Snapshot binding under invariant 5

The accepted ADR-013 (`docs/design/DECISIONS.md`)
interprets the binding as one immutable snapshot at any instant. Before an
attempt starts, bounded rebinding may repoint the episode to a validated live
version. It preserves identity, admission evidence, and finite budget; it does
not mutate either published snapshot or a running attempt's request. See
[the episode explanation](../learn/reasoning.md) and
[rebinding tests](../../internal/episodes/internal/app/rebind_test.go).

## Invariant-to-system map

| Invariants | Primary implementation areas |
| --- | --- |
| 1, 6, 7 | `internal/decisions`, `internal/policy`, `internal/actions`, worker/evidence boundary |
| 2, 3, 4 | `internal/eventlog`, `internal/engine`, `internal/operators`, `internal/situations` |
| 5, 10 | `internal/cognition`, `internal/episodes`, `internal/notify`, `internal/storage` |
| 8 | `internal/sources`, `internal/storage`, `internal/evidence`, `internal/actions`, `internal/notify` |
| 9 | `internal/replay`, `internal/runtime`, replay isolation tests |

## Proving tests

Each invariant names the tests that fail when it breaks, grouped by the module
that owns the behavior. A test sits at the lowest layer that owns the rule; the
module's own tests prove the rest. A change that weakens an invariant fails at
least one test listed here, so a rename or deletion of a listed test is a
change to the invariant's evidence and is reviewed as one. The per-module
detail is in the test audit (`docs/test-audit-2026-10-09/modules/README.md`).

### Invariant 1: Raw events are evidence, never executable instructions.

- `internal/eventlog`: [`TestQuarantineRawKeepsTheBytesAsDataUnderTheGivenEventID`](../../internal/eventlog/internal/app/quarantine_test.go), [`TestRedriveOfARawQuarantineIsRefusedByEnvelopeValidation`](../../internal/eventlog/internal/app/quarantine_test.go)
- `internal/ingress`: [`TestJSONLReplayQuarantinesMalformedAndSchemaInvalidLines`](../../internal/ingress/internal/app/jsonl_test.go), [`TestJSONLReplayQuarantinesAnEnvelopeOfAnotherTenant`](../../internal/ingress/internal/app/jsonl_test.go), [`TestSimulatorTraceRefusesGrammarViolations`](../../internal/ingress/internal/domain/simulator_trace_test.go)
- `internal/contractsv1`: [`TestDecodeDocumentJSONRefusesWhatALenientReaderAccepts`](../../internal/contractsv1/internal/domain/stored_document_test.go), [`TestEverySchemaAcceptsItsValidDocumentAndRejectsUnknownProperties`](../../internal/contractsv1/internal/domain/schemas_test.go)
- `internal/spec`: [`TestCompileRefusesIntentPoliciesThePolicyPlaneDoesNotEnforce`](../../internal/spec/internal/domain/intent_policy_test.go)
- `internal/decisions`: [`TestValidateKeepsPresetFieldsPresetAuthored`](../../internal/decisions/internal/domain/intent_parameters_test.go), [`TestValidateBindsIdentityParametersToTheEpisode`](../../internal/decisions/internal/domain/intent_parameters_test.go), [`TestValidateRefusesParametersOutsideTheCatalogSchema`](../../internal/decisions/internal/domain/intent_parameters_test.go), [`TestValidateGroundsIntentEvidenceInTheDecisionFacts`](../../internal/decisions/internal/domain/intent_parameters_test.go)
- `internal/evidence`: [`TestDecodeEvidenceGetArgumentsAcceptsOnlyTheClosedV1Schema`](../../internal/evidence/internal/wire/arguments_test.go), [`TestEventLogQueryReadsOnlyWhatTheCallScopeAllows`](../../internal/evidence/internal/transport/event_log_query_test.go)
- `internal/watch`: [`TestValidateExpressionRefusesForbiddenSyntaxAndInvalidCEL`](../../internal/watch/internal/domain/expression_test.go)
- `internal/policy`: [`TestAnIntentWhoseStoredRecordsDoNotVerifyIsDeniedBeforeAnyCommand`](../../internal/policy/internal/app/pending_intent_test.go)

### Invariant 2: Event time, watermark, completeness, and late-data status are explicit.

- `internal/eventlog`: [`TestLateArrivingEventKeepsItsEventTimeAndTakesTheNextLogPosition`](../../internal/eventlog/internal/store/read_test.go), [`TestOutOfOrderEventsAreLoggedInArrivalOrderWithTheirOwnEventTime`](../../internal/eventlog/internal/app/append_test.go)
- `internal/engine`: [`TestWatermarkTrailsEventTimeAndNeverMovesBackwards`](../../internal/engine/internal/domain/watermark_test.go), [`TestLateEventIsAppliedButNeverPullsThePartitionWatermarkBack`](../../internal/engine/internal/app/run_test.go), [`TestMissingHeartbeatMakesTheSituationUncertainUntilTheHeartbeatReturns`](../../internal/engine/internal/app/timers_test.go)
- `internal/operators`: [`TestLatestAndMaximumFollowEventTimeThenEventIDNotArrivalOrder`](../../internal/operators/internal/domain/window_test.go), [`TestStaleBootCannotMutateTheActiveWindow`](../../internal/operators/internal/domain/boot_test.go), [`TestInvalidHeartbeatDoesNotRefreshLiveness`](../../internal/operators/internal/domain/heartbeat_test.go)
- `internal/ingress`: [`TestJSONLReplayAppendsEventsAndResumesFromItsCheckpoint`](../../internal/ingress/internal/app/jsonl_test.go)

### Invariant 3: A Situation version is immutable after publication.

- `internal/situations`: [`TestPublishedSituationVersionNeverChangesAfterLaterFeatures`](../../internal/situations/internal/domain/version_immutability_test.go), [`TestMutatingAPublishedVersionDoesNotReachTheEngine`](../../internal/situations/internal/domain/version_immutability_test.go), [`TestVersionNumbersOnlyGrowAndEachStepNamesItsPredecessor`](../../internal/situations/internal/domain/version_immutability_test.go), [`TestMaterializationKeepsCanonicalEvidenceAndPrivateState`](../../internal/situations/internal/domain/materialize_test.go), [`TestCurrentStateIsACopyWithItsCanonicalStateAndDigest`](../../internal/situations/internal/domain/restore_test.go)
- `internal/engine`: [`TestAPublishedSituationVersionIsNeverRewritten`](../../internal/engine/internal/store/situations_test.go), [`TestPublishedVersionsAreReadableByNumberAndAsTheCurrentOne`](../../internal/engine/internal/app/situation_reads_test.go)

### Invariant 4: Deterministic state changes are serial per virtual partition.

- `internal/contractsv1`: [`TestPartitionIDsAreFrozenFNV1aOfTenantNulKey`](../../internal/contractsv1/internal/domain/envelope_test.go)
- `internal/engine`: [`TestConcurrentRunsApplyEachEventExactlyOnce`](../../internal/engine/internal/app/run_test.go), [`TestTheSameEvidenceYieldsIdenticalSituationsOnEveryRun`](../../internal/engine/internal/app/run_test.go)
- `internal/runtime`: [`TestEpisodesRunBesideIngestion`](../../internal/runtime/internal/app/episodes_beside_ingestion_test.go)
- `internal/control`: [`TestALostLeaseFencesTheOldEpochsRenewalsAndWrites`](../../internal/control/runtime_owner_test.go), [`TestRecoveryCannotCommitOnceTheOwnerLeaseIsLost`](../../internal/control/owner_recovery_test.go)

### Invariant 5: Every episode is bound to one immutable Situation snapshot and finite budget.

- `internal/episodes`: [`TestRequestAssemblyBindsProvenanceAndEvidence`](../../internal/episodes/internal/domain/assembly_test.go), [`TestAssemblerBindsTheRequestToTheSituationSnapshotAndTheSpec`](../../internal/episodes/internal/app/assembler_test.go), [`TestValidateSnapshotEvidenceRejectsTamperingAndIdentityDrift`](../../internal/episodes/internal/domain/snapshot_test.go), [`TestRebindPreservesAdmissionEvidence`](../../internal/episodes/internal/domain/admission_test.go), [`TestDispatchNeverRewritesTheAdmittedBudget`](../../internal/episodes/internal/app/dispatch_freshness_test.go), [`TestDispatchRefusesADecisionThatArrivesAfterTheWallTimeBudget`](../../internal/episodes/internal/app/dispatch_freshness_test.go), [`TestPersistRefusesAnEpisodeThatExceedsTheCostCeiling`](../../internal/episodes/internal/app/assembler_test.go), [`TestRunnerCancelsTheStreamedAttemptOfASupersededEpisode`](../../internal/episodes/internal/app/cancellation_test.go)
- `internal/control`: [`TestATenantCeilingRefusalRollsBackTheGlobalAccountingOfTheReservation`](../../internal/control/cost_boundaries_test.go), [`TestSettlementRollsBackAllAccountingWhenTheTenantWriteFails`](../../internal/control/cost_boundaries_test.go)
- `internal/executor/native`: [`TestRequestBudgetRequiresAFiniteBound`](../../internal/executor/native/internal/domain/budget_test.go), [`TestExecuteRefusesAnUnboundedRequestWithoutCallingTheProvider`](../../internal/executor/native/internal/app/bounds_test.go)
- `internal/executor/remote`: [`TestExecuteRefusesRequestsItCannotBoundBeforeContactingTheWorker`](../../internal/executor/remote/internal/app/executor_test.go), [`TestExecuteReportsAWorkerBudgetBreachAsATypedError`](../../internal/executor/remote/internal/app/executor_test.go)
- `internal/worker`: [`TestBudgetRequiresAValidPositiveWallTime`](../../internal/worker/internal/domain/budget_test.go)
- `internal/episodeledger`: [`TestASituationHasOneLiveEpisodeAndOnlyAReconsiderationReportsTheConflict`](../../internal/episodeledger/internal/app/admission_test.go), [`TestCoalescedItemsSupersedeTheirLiveEpisodesAndCancelTheirInFlightAttempts`](../../internal/episodeledger/internal/store/supersession_test.go)

### Invariant 6: A model can read evidence and propose typed Intents; it cannot execute effects.

- `internal/architecture`: [`TestReasoningAndReplayCannotReachEffectImplementations`](../../internal/architecture/imports_test.go), [`TestDecisionCatalogKeepsAuthorityPrivate`](../../internal/architecture/module_specific_test.go), [`TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability`](../../internal/architecture/imports_test.go), [`TestEpisodeLifecycleImportsNoExecutorTransport`](../../internal/architecture/imports_test.go)
- `internal/decisions`: [`TestValidateRefusesIntentTypesTheEpisodeDoesNotAllow`](../../internal/decisions/internal/domain/intent_authority_test.go), [`TestValidateRefusesAllowedTypesTheCatalogDoesNotDeclare`](../../internal/decisions/internal/domain/intent_authority_test.go), [`TestValidateHoldsProposedRiskToTheCatalogAndTheCeiling`](../../internal/decisions/internal/domain/intent_authority_test.go)
- `internal/evidence`: [`TestCallRefusesEveryRequestOutsideTheCapabilityBeforeReserving`](../../internal/evidence/internal/app/call_scope_test.go), [`TestAuthorizeScopeRefusesEveryDimensionOutsideTheCapability`](../../internal/evidence/internal/domain/authorization_test.go)
- `internal/executor/native`: [`TestExecuteFailsTheAttemptWithTheReasonOfTheBoundItBreaches`](../../internal/executor/native/internal/app/bounds_test.go), [`TestFailedToolCallCannotBeRepeated`](../../internal/executor/native/internal/app/loop_tools_test.go)
- `internal/executor/remote`: [`TestExecuteGivesTheWorkerNoEvidenceAccessUnlessConfigured`](../../internal/executor/remote/internal/app/evidence_capability_test.go), [`TestExecuteIssuesAFreshScopedCapabilityPerDispatch`](../../internal/executor/remote/internal/app/evidence_capability_test.go)
- `internal/worker`: [`TestEvidenceSocketIsPrivateAndCleansUp`](../../internal/worker/internal/transport/uds_test.go)
- `internal/device`: [`TestEveryEffectorChecksAuthorizationBeforeDispatch`](../../internal/device/internal/app/gateway_effector_authorization_test.go), [`TestMaterializeIgnoresModelSuppliedParameters`](../../internal/device/internal/domain/materialize_test.go), [`TestMaterializeFailsClosed`](../../internal/device/internal/domain/materialize_test.go)
- `internal/runtime`: [`TestDeviceRoutesNeverReachTheFallback`](../../internal/runtime/internal/app/effect_routing_test.go), [`TestARouteWithoutItsEffectorFailsClosedInsteadOfFallingBack`](../../internal/runtime/internal/app/effect_routing_test.go)
- `internal/spec`: [`TestCompileRejectsASpecThatBreaksARule`](../../internal/spec/internal/domain/compiler_test.go)

### Invariant 7: Policy revalidates every Intent against current state immediately before dispatch.

- `internal/policy`: [`TestEvaluationRevalidatesTheIntentAgainstCurrentState`](../../internal/policy/internal/app/evaluation_test.go), [`TestResolvingAnApprovalRevalidatesTheIntentAgainstCurrentState`](../../internal/policy/internal/app/approval_resolution_test.go), [`TestApprovalStalenessPrecedesExpiryAndAuthorization`](../../internal/policy/internal/app/approval_resolution_test.go), [`TestAnApprovalExpiresExactlyAtItsDeadline`](../../internal/policy/internal/app/approval_expiry_test.go), [`TestRiskRouteForEveryRiskClassAndCatalogApproval`](../../internal/policy/internal/domain/routing_test.go), [`TestACommittedApprovalDispatchesWithoutNewSensorInput`](../../internal/policy/internal/app/approval_dispatch_test.go)
- `internal/actions`: [`TestAnIntentIsRevalidatedAgainstCurrentStateJustBeforeTheEffector`](../../internal/actions/internal/app/authorization_test.go), [`TestTheInterlockIsCheckedAgainAtTheMomentTheEffectorAccepts`](../../internal/actions/internal/app/authorization_test.go), [`TestATrippedInterlockFailsTheCommandBeforeTheEffector`](../../internal/actions/internal/app/authorization_test.go), [`TestAuthorizationRefusesEachStaleOrAlteredRecord`](../../internal/actions/internal/domain/authorization_test.go)
- `internal/control`: [`TestDispatchGateRefusesOnceTheInterlockTripsAndNeverWritesTheInterlock`](../../internal/control/dispatch_gate_test.go), [`TestAssertInterlockReadsTheDurableInterlockInOneTransaction`](../../internal/control/internal/store/dispatch_test.go)
- `internal/interlock`: [`TestTripBlocksAndClearReopensTheActionPlane`](../../internal/interlock/interlock_test.go), [`TestClearIsRefusedWithoutTheFenceAndWritesNothing`](../../internal/interlock/interlock_test.go), [`TestRequireReadyFailsClosedWithTheStoredReason`](../../internal/interlock/internal/domain/interlock_test.go)
- `internal/watch`: [`TestATrippedInterlockRefusesBothInstallingAndFiring`](../../internal/watch/internal/app/install_test.go), [`TestAuthorizedDispatchRefusesWithoutAPassingFinalCheckAndInstallsNothing`](../../internal/watch/internal/app/install_test.go)
- `internal/api`: [`TestApprovalsAreRejectedBeforeAnyCallbackRuns`](../../internal/api/internal/transport/approvals_test.go), [`TestApprovalsPresentAndResolveBoundToTheRegisteredRelay`](../../internal/api/internal/transport/approvals_test.go)
- `internal/device`: [`TestGatewayEffectorRefusesAnUnauthorizedCommandBeforeMaterializingOrSending`](../../internal/device/internal/app/gateway_effector_dispatch_test.go)

### Invariant 8: Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.

- `internal/architecture`: [`TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase`](../../internal/architecture/ownership_test.go)
- `internal/eventlog`: [`TestInsertEventKeepsTheFirstDeliveryOfAnEventIDPerTenant`](../../internal/eventlog/internal/store/events_test.go), [`TestAppendReportsDuplicatesAsMinusOne`](../../internal/eventlog/internal/app/append_test.go), [`TestQuarantineRejectsTheEleventhDeliveryOfThePayload`](../../internal/eventlog/internal/store/quarantine_test.go), [`TestReleasedEnvelopeIsRedrivenIntoTheLogExactlyOnce`](../../internal/eventlog/internal/app/quarantine_test.go)
- `internal/engine`: [`TestRecordedEventsAreIdempotentAndAdvanceTheCheckpoint`](../../internal/engine/internal/store/checkpoint_test.go), [`TestAnEventAlreadyInTheInboxChangesNoState`](../../internal/engine/internal/app/run_test.go), [`TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery`](../../internal/engine/internal/app/rollback_test.go)
- `internal/episodeledger`: [`TestTwoAttemptsCannotShareAnIdOrAFenceOfOneEpisode`](../../internal/episodeledger/internal/store/attempt_test.go), [`TestALateOutputOfAnEarlierAttemptIsRefusedAndAuditedAfterARetry`](../../internal/episodeledger/fencing_test.go), [`TestRecoveryIsIdempotent`](../../internal/episodeledger/internal/app/recovery_test.go)
- `internal/policy`: [`TestAnAutomaticCommandIsPreparedOnceAndReplayedWithoutDuplicates`](../../internal/policy/internal/app/evaluation_test.go), [`TestACommandIsStoredOnceAndAReplayLearnsTheWinner`](../../internal/policy/internal/store/commands_test.go), [`TestConcurrentHTTPRepliesCreateOneCommand`](../../internal/policy/internal/app/approval_http_test.go)
- `internal/actions`: [`TestACrashBetweenTheEffectAndItsOutcomeIsNeverDispatchedAgain`](../../internal/actions/internal/app/crash_recovery_test.go), [`TestADispatchRecordsItsOutcomeAndTheEffectIsNeverRepeated`](../../internal/actions/internal/app/dispatch_outcome_test.go), [`TestAnOutboxRowReplayedForAFinishedCommandIsClosedWithoutCallingTheEffector`](../../internal/actions/internal/app/dispatch_outcome_test.go), [`TestALeaseIsExclusiveUntilItExpires`](../../internal/actions/internal/store/lease_test.go), [`TestAnUnknownOutcomeIsReconciledOnlyByTypedIndependentEvidence`](../../internal/actions/internal/app/reconciliation_test.go)
- `internal/actionport`: [`TestUnknownOutcomePreservesCauseAndRequiresReconciliation`](../../internal/actionport/internal/domain/effect_test.go)
- `internal/authority`: [`TestBindCommandIsIdempotentAndRefusesConflicts`](../../internal/authority/internal/app/commands_test.go), [`TestRebootRequiresReconciliationAcrossRestartAndManualReview`](../../internal/authority/internal/app/reconciliation_test.go)
- `internal/device`: [`TestSessionAnswersADuplicateIdempotencyKeyFromItsReceiptCache`](../../internal/device/internal/app/session_exchange_test.go), [`TestAMalformedReceiptAfterSendingIsAnUnknownOutcomeThatSurvivesRestart`](../../internal/device/internal/app/gateway_effector_unknown_outcome_test.go)
- `internal/watch`: [`TestAWatchFiresOncePerEventAndOnlyWithinItsAllowance`](../../internal/watch/internal/app/fire_test.go), [`TestReinstallingTheSameWatchIsIdempotentAndAConflictingOneIsRefused`](../../internal/watch/internal/app/install_test.go)
- `internal/notify`: [`TestAppendingTheSameEventAgainReturnsItsCursorAndADifferentPayloadIsRefused`](../../internal/notify/internal/app/append_test.go), [`TestCursorsAreGaplessAcrossReleasedAllocations`](../../internal/notify/internal/store/append_test.go)
- `internal/approvalledger`: [`TestOnlyAPendingApprovalCanTransition`](../../internal/approvalledger/internal/store/transition_test.go), [`TestAnIntentHasOnePendingApprovalAtATimeAndApprovalIdsAreUnique`](../../internal/approvalledger/internal/store/transition_test.go)

### Invariant 9: Replay never performs external effects unless an explicit, separate simulation mode is selected.

- `internal/architecture`: [`TestReasoningAndReplayCannotReachEffectImplementations`](../../internal/architecture/imports_test.go)
- `internal/replay`: [`TestDeterministicModeEqualsRunAndNeverInvokesCognition`](../../internal/replay/internal/app/run_test.go), [`TestRunModeFailsClosedBeforeAnyWorkWithoutCapabilities`](../../internal/replay/internal/app/run_test.go), [`TestShadowPhasePersistsOnlyComparisonsAndRepeatsByteForByte`](../../internal/replay/internal/app/shadow_test.go), [`TestRecordedPhaseValidatesACompleteLedger`](../../internal/replay/internal/app/recorded_test.go), [`TestGoldenTracesAreDeterministic`](../../internal/replay/golden_replay_test.go), [`TestRunNTimesRepeatsDeterministicallyInSeparateDatabases`](../../internal/replay/internal/app/run_test.go), [`TestThermalChamberReplayIsDeterministic`](../../internal/replay/thermal_chamber_test.go)
- `internal/storage`: [`TestOpenFreshReservesPathUntilClose`](../../internal/storage/internal/store/fresh_database_test.go), [`TestOpenFreshRejectsCollisionsWithoutRemovingExistingFiles`](../../internal/storage/internal/store/fresh_database_test.go)
- `internal/device`: [`TestEffectProfileValidationReadsTheConfigurationItIsGiven`](../../internal/device/profile_test.go), [`TestCheckEffectProfile`](../../internal/device/internal/domain/profile_test.go)
- `internal/sources`: [`TestDeterministicSequenceIsReproducible`](../../internal/sources/internal/domain/ids_test.go)
- `cmd/agentic-stream`: [`TestExperimentShadowReplayComparesTheCandidate`](../../cmd/agentic-stream/experiment_shadow_test.go)

### Invariant 10: Every admitted, deferred, coalesced, rejected, canceled, and expired cognitive opportunity is explainable from durable records.

- `internal/cognition`: [`TestEveryCognitiveOpportunityIsExplainableFromDurableRecords`](../../internal/cognition/internal/app/explainability_test.go), [`TestEachTriggerGateRecordsItsOutcomeAndReasonDurably`](../../internal/cognition/internal/app/trigger_evaluation_test.go), [`TestCostRefusalIsAddedToTheEvaluationInTheCallersTransactionOnly`](../../internal/cognition/internal/app/evaluation_reasons_test.go), [`TestSchedulerExpiryIsAddedToTheEvaluationWithoutTouchingTheQueue`](../../internal/cognition/internal/app/evaluation_reasons_test.go)
- `internal/episodeledger`: [`TestAnOpportunityIsExplainableFromDurableRecordsWhetherCoalescedExpiredOrRefused`](../../internal/episodeledger/internal/app/reads_test.go), [`TestSchedulingExplainsWhatBecameOfAnItemItsEpisodeAndTheRefusedResults`](../../internal/episodeledger/internal/store/scheduling_test.go), [`TestARejectionOfAKnownEpisodeIsAuditedWithItsAttemptAndFence`](../../internal/episodeledger/internal/app/rejection_test.go), [`TestAnEpisodeIsAdmittedRunAndExplainedThroughTheFacade`](../../internal/episodeledger/episode_lifecycle_test.go)
- `internal/approvalledger`: [`TestEveryApprovalOfAnIntentIsExplainableThroughTheFacade`](../../internal/approvalledger/lifecycle_test.go), [`TestEachTransitionRecordsItsOwnColumnsAndStableReason`](../../internal/approvalledger/internal/store/transition_test.go)
- `internal/policy`: [`TestAnInterlockDenialKeepsTheStableReasonAndAuditsItsCause`](../../internal/policy/internal/app/interlock_test.go), [`TestAnApprovalWithUnreadableExpiryIsExpiredWithAnAuditReason`](../../internal/policy/internal/app/approval_expiry_test.go)
- `internal/runtime`: [`TestAdmitPendingAdmitsOrSkipsTheDueItemAccordingToTheScenario`](../../internal/runtime/internal/app/admission_test.go), [`TestAdmissionStepsRecordWhyAnItemLeftTheQueue`](../../internal/runtime/internal/store/admission_test.go)
- `internal/notify`: [`TestRefusalsAreAuditedWithTheRequestedAndOldestCursors`](../../internal/notify/internal/app/read_test.go), [`TestNotificationsAreCursorResumableAndAuditExpiredCursor`](../../internal/notify/reading_test.go)
- `internal/api`: [`TestSSEStreamsDurableEventsInFramesAndResumesAfterTheCursor`](../../internal/api/internal/transport/sse_test.go), [`TestSSEAnswersARefusedResumeWithAProblemBeforeAnyStream`](../../internal/api/internal/transport/sse_test.go)

## Review questions

- Can untrusted content cross the boundary as an executable parameter?
- Can out-of-order or duplicate input change a published version silently?
- Can a stale worker result become accepted after cancellation or restart?
- Can a policy decision become stale between validation and effect acceptance?
- Can a crash cause duplicate or unacknowledged external work without a durable
  reconciliation state?
- Can replay construct or reach a real effector?
- Can an operator explain why an opportunity was admitted or refused?

## Next reads

- [Security model](security-model.md)
- [Replay and shadow design](../design/replay-and-shadow.md)
- [Quality gates](../governance/quality.md)
