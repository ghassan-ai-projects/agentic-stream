# contractsv1

Status: done
Round: 2

## Metrics

| Package | Coverage before | after | Time before (`-short -race`) | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/contractsv1` | 73.3% | 100% | 1.1 s | 1.1 s | 7 / 7 | 9 / 12 |
| `internal/contractsv1/contractstest` | 83.9% | 90.3% | 1.1 s | 1.1 s | 2 / 2 | 5 / 5 |
| `internal/contractsv1/internal/domain` | 87.2% | 97.3% | 1.1 s | 1.2 s | 19 / 57 | 36 / 143 |

## Layout (domain tests, one file per subject)

`schemas_test.go`, `cloud_event_test.go`, `trace_context_test.go`, `envelope_test.go` (envelope, partition ids), `digests_test.go` (intent digest), `risk_test.go`, `fields_test.go`, `version_test.go`, `stored_document_test.go`, `device_schema_test.go` (device wire conformance).

## Findings and changes

### Removed
- `internal/domain/protocol_contract_test.go`: not a domain test (it read `docs/design/contracts/runtime-v1.proto` through a `runtime.Caller` path). Its frozen-fragment checks moved to the facade package next to the SHA-256 pin, `experiment_contract_test.go`, as `TestWorkerProtocolKeepsTheFrozenBoundary`. It stays separate from the pin on purpose: when the pin is legitimately updated, the fragments still say what may not change. (`git rm`.)
- `internal/domain/contracts_test.go`: split into `schemas_test.go`, `cloud_event_test.go`, `trace_context_test.go` (T3, one subject per file). (`git rm`.)
- Facade assertion that `ContractVersion`, `ProtocolVersion` and `TenantID` are "non-empty" (T10): replaced by `TestVersionIdentifiersAreFrozenOnTheWire`, which pins the literals (wire contract).
- The long header comment of `device_schema_test.go` and the `x := x` copies (T11).

### Renamed or moved
- `TestSharedSchemaIDsAndValidation` → `TestEverySchemaAcceptsItsValidDocumentAndRejectsUnknownProperties` + `TestSchemaIDIsTheStableURNOfEveryKnownSchema`.
- `TestIntentDigestExcludesItselfAndVerifies` moved from `envelope_test.go` to `digests_test.go`.
- `TestFacadeValidatesEnvelopesAndKeepsContractVersions` → `TestFacadeValidatesEnvelopes`.

### Improved
- T6: all `contracts_test.go` and `protocol_contract_test.go` tests and subtests lacked `t.Parallel()`.
- T4: facade and trace-context rejections assert the message, not `err != nil`.
- `cloud_event_test.go`: a `sealedCloudEvent` builder replaces inline setup (T9).

### Added
- Schemas: all six non-device schemas (`trigger-evaluation` had no test) accept a valid document, reject an unknown property and a non-object; 30 mutation rows prove the rules the schemas enforce (versions >= 1, enums, digest patterns, date-time format, `risk_class` R0..R4, intent `type` pattern, unique evidence ids, decision `decision_type` conditionals: empty intents need it, `need_more_evidence` forbids intents, 17 intents refused); unknown schema names refused by `SchemaID` and `Validate`; every embedded file has a `SchemaName` and the reverse.
- Golden (digest contracts): `TestCloudEventEnvelopeDigestGolden` (also cross-checked by the independent projection test), `TestIntentDigestGolden` (value recomputed independently with SHA-256 over domain plus canonical JSON), `TestPartitionIDsAreFrozenFNV1aOfTenantNulKey` (recomputed independently with FNV-1a 64; partition ids reshuffle all state if they move).
- `TestCloudEventEnvelopeDigestBindsEveryProjectedAttributeAndTheData`: every projected attribute, the data and the trace context change the digest; the same instant in another zone does not.
- `TestCloudEventValidateRequiresTheCloudEventsAttributes` (13 rows), digest required, trace context accepted.
- `TestParseTraceContextRefusesInvalidContexts` (12 rows), the 512-byte tracestate boundary, and the no-tracing case with a nil link.
- `TestValidConformanceFramesAreByteExactCanonicalJSON`: the conformance README promises canonical JSON plus a trailing newline for consumers that copy the bytes; nothing checked it.
- `TestTheInvalidCorpusExercisesEveryMessageType`, `TestSchemaForMessageTypeKnowsOnlyTheFourDeviceRecords`.
- Facade: `TestFacadeVerifiesAStoredDocumentAgainstItsSchemaAndDigest` (`DecodeDocumentJSON`, `DecodeDocument`, `VerifyDocumentDigest`, `VerifyStoredDocument` and the three sentinels were 0%).
- `contractstest` (test support, previously 0% on two helpers): `AmbiguousKeyJSON`, `RiskClasses` (agrees with the facade constants), invalid frames name a real message type.
- `TestDocumentFieldsProjectWithoutCoercion` extended: fractions truncate, null and nil document project empty.

### Speed
- Nothing slow.

## Production code touched
- none

## Invariants proven here
- 1 (raw events are never instructions) and 7 (intents revalidated against their digest and schema): `TestStoredDocumentRuleHoldsForEverySchemaAndDomain`, `TestIntentDigestFieldMustAgreeWithTheStoredDigest`, `TestDecodeDocumentJSONRefusesWhatALenientReaderAccepts`, `TestEverySchemaAcceptsItsValidDocumentAndRejectsUnknownProperties`.
- 4 (serial per partition) depends on `TestPartitionIDsAreFrozenFNV1aOfTenantNulKey`.
- Experiment compatibility (E7, E8) kept: `TestExperimentThermalCatalogKeepsItsPath`, `TestExperimentWorkerProtocolIsUnchanged` (same names, same pins).

## Open items
- `docs/STREAM_IMPLEMENTATION_AUDIT_2026-08-12.md` (dated archive, left alone) still names `protocol_contract_test.go`.
- `thermal-capability-catalog.json` has only a path pin here; its digest pin lives with the device module (round 13).
- Remaining uncovered statements are the embedded-schema read/compile failure branches in `schemas.go`, reachable only with a corrupt embedded file.
