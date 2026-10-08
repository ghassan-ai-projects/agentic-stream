# DUP-007: Stored intent/decision/snapshot/command verification is implemented separately in twelve places

- Status: fixed
- Commit: 60c7b98
- Severity: high
- Verdict (finders): DIVERGED
- Themes: contracts and shapes
- Wave: 3
- Finder sources: S3 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Depends on DUP-008 (digest primitives). Owner: `contractsv1` with typed sentinel errors so policy keeps its audited reason codes and actions keeps its messages. Keep the constant-time intent compare everywhere; remove the actions copy of `DocumentString`/`DocumentInt`.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S3: Stored intent/decision/snapshot/command document verification (schema + domain digest + identity) done separately in policy, actions, episodes, replay, cognition, runartifact, decisions, remote

- Verdict: DIVERGED
- Shared meaning: "this stored document is the one its stored 32-byte digest binds": decode, validate against the embedded v1 schema, recompute the domain-separated digest, compare with the stored raw digest, then bind identity fields.
- Sites:
  - internal/policy/internal/domain/documents.go:16-31 - `DocumentDigestMatches(document, digestBytes, domain)`: length check, intent special case via `VerifyIntentDigest`+`IntentDigest` compared with `subtle.ConstantTimeCompare`, else `canonicaljson.Verify(domain, doc, EncodeDigest(d))`
  - internal/policy/internal/domain/governance_documents.go:36-90 - `ParseDecision`/`ParseIntent`: `DecodeDocument` (rules.go:11, schema) -> identity -> digest; failure reasons `schema_invalid`, `identity_mismatch`, `decision_digest_mismatch`, `intent_digest_mismatch`
  - internal/actions/internal/domain/document.go:49-67 - `verifyDigest` and `verifyIntentDigest` (same job as `DocumentDigestMatches`; intent branch uses plain `==` instead of constant time and runs `VerifyIntentDigest` then `IntentDigest` redundantly)
  - internal/actions/internal/domain/authorization.go:46-58,96-125 - `VerifiedCommand`, `CheckIntent`, `CheckDecision`: unmarshal + `contractsv1.Validate(Schema...)` + digest + `ParseIntentDocument`/`ParseDecisionDocument` identity (typed_documents.go:60-88), generic errors; internal/actions/internal/domain/candidate.go:141 verifies command the same way
  - internal/actions/internal/domain/document.go:12-35 - `Document.String/Int/Int64/Object`: private copy of `contractsv1.DocumentString/DocumentInt` (policy and episodes use the contractsv1 ones)
  - internal/episodes/internal/domain/snapshot.go:28-58 - `ValidateSnapshotEvidence`/`bindSnapshotEvidence`: schema + `Digest(DomainSnapshot)` + `DecodeDigest` + `bytes.Equal`
  - internal/replay/internal/domain/shadow_snapshot.go:13-46 - `VerifiedSnapshot`/`verifySnapshotDigest`: canonicalize + schema + same digest-equals-persisted check
  - internal/cognition/internal/domain/correction.go:41-63 - `DecodeCorrection`/`MatchCorrectionDigest`: schema + `Digest(DomainSnapshot)` + `DecodeDigest` + `bytes.Equal`
  - internal/replay/internal/domain/recorded.go:66-86 - `VerifyRecordedDocument`: canonicalize + `SchemaDecision` + `Verify(DomainDecision)`
  - internal/replay/internal/domain/shadow_rules.go:70-95 - `canonicalShadowDecision` + `bindShadowDecisionDigest`
  - internal/decisions/internal/domain/validator.go:104-120 - `parseDecision`: canonicalize + schema + `DecodeDigest`+`Verify(DomainDecision)`
  - internal/executor/remote/internal/domain/stream.go:133-146 - `verifyDecisionDigest` (digest only)
  - internal/runartifact/internal/domain/verify.go:78-130 - `verifyRowDigest`/`expectedDigest` for commands, decisions, snapshots (with base64 BLOB decode)
  - internal/episodes/internal/domain/decision.go:28-70 - `StorageDecisionDigest`/`decisionDigestForStorage`: canonicalize, unmarshal, `Digest(DomainDecision)`, `DecodeDigest`
- How they differ / already diverged: failure surfaces differ (policy returns stable reason codes that are audited; actions returns generic strings; replay/cognition return formatted errors); intent compare is constant-time in policy and not in actions; the "canonicalize raw JSON, then unmarshal" preamble appears in 5 of them and is skipped in others (policy/actions unmarshal raw bytes directly, so a non-canonical-but-equal document is accepted there and rejected in decisions/replay); length-32 guards are explicit in some and implicit (via `DecodeDigest`) in others. This looks accidental, not a defence-in-depth decision (policy and actions both claim to re-validate; no ADR calls for different semantics).
- Risk if left: tightening the check (for example rejecting non-canonical stored bytes, or changing the intent self-digest exclusion) must be repeated in ~12 places; one missed site becomes a fail-open verifier on the dispatch path.
- Proposed canonical owner: `internal/contractsv1` (domain layer 9, facade 12; already owns `IntentDigest`, `VerifyIntentDigest`, `Validate`, `DocumentString`). policy, actions, replay, episodes, cognition, decisions and runartifact already import contractsv1 or may (policy/actions domain are layer 13 > 12; runartifact/internal/domain is layer 9 and imports only canonicaljson, so it would use the canonicaljson part only).
- Proposed fix: in canonicaljson add `DigestBytes(domain, v) ([]byte, error)` and `VerifySum(domain, v, sum []byte) bool` (see C6); in contractsv1 add `VerifyStoredDocument(schema SchemaName, domain Domain, raw, sum []byte) (map[string]any, error)` returning typed sentinel errors (`ErrDocumentSchema`, `ErrDocumentDigest`) so policy can map to its reason codes and actions to its messages; intent special-case lives there once. Delete actions `verifyDigest`/`verifyIntentDigest` and `Document.String/Int/Int64`'s duplicate of `DocumentString/DocumentInt` (keep `Document.Object` and `Digest`).
- Behaviour to preserve: policy reason codes and their precedence (decision before intent; `ParseGovernanceDocuments`), actions error strings asserted in `authorization_test.go`, digest bytes, constant-time compare for the intent path, `Verify` returning false for malformed.
- Verification: policy governance tests, `internal/actions/internal/domain/authorization_test.go`, replay recorded/shadow tests, `runartifact` verify tests. New: one contractsv1 table test per (schema, domain) covering tampered digest, tampered body, schema-invalid, non-canonical bytes; a test that policy and actions return the same accept/reject verdict for the same rows.

## Outcome

Verified (finders read, did not run):
- Confirmed: eight modules had their own copy of "decode, validate, digest" (policy, actions, episodes snapshot, replay snapshot/recorded/shadow, cognition, decisions, remote, runartifact).
- Confirmed: actions compared the intent digest with `==` (policy used constant time) and ran `VerifyIntentDigest` then `IntentDigest` redundantly; actions also kept a private `Document.String/Int/Int64/Object/Digest` copy of `contractsv1.DocumentString/DocumentInt` (`Object` and `Digest` had no remaining use besides one call and a test).
- Partly wrong: the finders said policy and actions accept "non-canonical but equal" bytes while decisions and replay reject them. All of them accept whitespace and key-order differences (`canonicaljson.Marshal(json.RawMessage)` re-canonicalizes); only the replay shadow-output and recorded-decode paths require byte-canonical input, and that is an intake rule for worker output, kept as is. Stored decision `raw_json` is the worker's raw bytes, so a byte-canonical rule would be wrong for stored documents.
- Real looseness found (the strictest-rule choice): policy, actions (command, intent, decision), episodes snapshot, cognition correction and the remote stream decoded with plain `json.Unmarshal`. That reader keeps the last of duplicate keys, replaces invalid UTF-8 and lone surrogates, and rounds inexact integers, so a stored document could decode to different content than its bytes show while the digest (computed over the decoded map) still matched. Decisions, replay snapshot/recorded and shadow used `canonicaljson.Marshal(json.RawMessage)` first, which refuses all of those. The remote stream (digest-only check) was the loosest and also compared digests with a plain string compare.
- Not a verifier: `episodes` `StorageDecisionDigest` / `decisionDigestForStorage` computes a digest for storage and is left unchanged.

The one rule (owner `contractsv1`, files `internal/contractsv1/internal/domain/stored_document.go` and the facade `contractsv1.go`):
- `DecodeDocumentJSON(raw)`: strict decode (`canonicaljson.Marshal(json.RawMessage)` then unmarshal; duplicate keys, bad Unicode, several values, inexact numbers, non-objects and `null` are refused with `ErrDocumentJSON`).
- `DecodeDocument(raw, schema)`: strict decode then schema validation (`ErrDocumentSchema`).
- `VerifyDocumentDigest(domain, document, sum)`: 32-byte length check, constant-time compare; the intent is bound without its own digest field and must also carry that digest.
- `VerifyStoredDocument(schema, domain, raw, sum)`: decode, schema, digest (`ErrDocumentDigest`). The three sentinels are typed so each caller keeps its own surface: policy keeps `schema_invalid` / `*_digest_mismatch` and the decision-before-intent precedence (identity is checked between decode and digest, so policy and the episodes snapshot use the two primitives); actions keeps its exact messages and maps `ErrDocumentSchema` / other to `FailureCommandSchemaInvalid` / `FailureCommandJSONInvalid`; decisions keeps its `canonical_json` / `decision_schema` / `decision_digest` fields.
- `DigestDomain` is a facade alias of `canonicaljson.Domain` so the contractsv1 facade needs no new `canonicaljson` edge. No `allowedImports` or `packageLayers` edit was made.

Behaviour changes on purpose (strictness only; nothing is looser than before):
- policy, actions, episodes snapshot, cognition and the remote stream now refuse ambiguous stored bytes (duplicate keys, invalid Unicode, trailing values, inexact numbers) with the same failure surface as other malformed JSON (`schema_invalid`, "intent authorization is invalid", `FailureCommandJSONInvalid`, `unmarshal snapshot`, "worker decision is not valid JSON").
- The remote stream digest check is constant time and shares the intent-free path (digest only, no schema, so a schema-invalid worker decision is still recorded as a rejected decision downstream).
- replay `VerifyRecordedDocument` and `VerifiedSnapshot` error texts now read `recorded ledger decision "<key>": <sentinel>: ...` and `verify shadow snapshot: ...` (no test asserted the old text). `VerifiedSnapshot` returns `canonicaljson.Marshal` of the verified document, which is the digest preimage.
- Order inside cognition: decode, then read the persisted digest, then compare, as before. Episodes: decode, identity, entity, digest, as before.

Changed files:
- New: `internal/contractsv1/internal/domain/stored_document.go`, `internal/contractsv1/internal/domain/stored_document_test.go`, `internal/decisions/internal/domain/parse_decision_test.go`, `internal/executor/remote/internal/domain/stream_digest_test.go`.
- contractsv1 facade and test support: `internal/contractsv1/contractsv1.go`, `internal/contractsv1/contractstest/contractstest.go` (new `AmbiguousKeyJSON`).
- Consumers: `internal/policy/internal/domain/{documents,rules,governance_documents}.go` (`DocumentDigestMatches` deleted); `internal/actions/internal/domain/{authorization,candidate,document,typed_documents}.go` (`verifyDigest`, `verifyIntentDigest` and `Document.String/Int/Int64/Object/Digest` deleted, `Parse*Document` take `map[string]any`); `internal/episodes/internal/domain/snapshot.go`; `internal/cognition/internal/domain/correction.go`, `internal/cognition/internal/app/correction.go` (`DecodeCorrection` returns two values, `MatchCorrectionDigest` takes the decoded correction); `internal/replay/internal/domain/{shadow_snapshot,recorded,shadow_rules}.go`; `internal/decisions/internal/domain/validator.go`; `internal/executor/remote/internal/domain/stream.go`; `internal/runartifact/internal/domain/verify.go` (domain digest compared with `VerifySum`, which is constant time; the document is already decoded and canonical-checked there, and the package stays at layer 9 with no contractsv1 import).
- Tests changed: policy `rules_test.go`, `routing_test.go`; actions `authorization_test.go` (removed `TestDocumentAccessorsTolerateMissingFields`, the accessors are gone), `candidate_test.go`; episodes `rules_test.go`; replay `shadow_snapshot_test.go`, `recorded_test.go`; cognition `boundaries_test.go` (new signatures).

Pinning tests (all fail when the strict decode is swapped for a plain `json.Unmarshal`, checked by mutation):
- `TestStoredDocumentRuleHoldsForEverySchemaAndDomain` (contractsv1; command, intent, decision, snapshot x bound, indented-but-equal, tampered body, tampered/short/missing digest, schema-invalid, not JSON, null, trailing value, duplicate key at top level and mid-document), `TestIntentDigestFieldMustAgreeWithTheStoredDigest`, `TestDecodeDocumentJSONRefusesWhatALenientReaderAccepts`.
- Per consumer: policy `TestDecisionValidationPrecedenceAndBinding/ambiguous_bytes`, `TestTypedIntentRetainsDigestInput`; actions `TestAuthorizationRefusesEachStaleOrAlteredRecord` (ambiguous command/intent/decision bytes), `TestAdmitDecidesEachCandidateKind` (ambiguous document); episodes `TestValidateSnapshotEvidenceRejectsTamperingAndIdentityDrift/ambiguous_bytes`; replay `TestVerifiedSnapshotRejectsTampering`, `TestVerifyRecordedEntryRejectsIncomplete`; cognition `TestReconsiderationEvidenceAndDigestRefusal`; decisions `TestParseDecisionReportsEachStoredDocumentFailureField`; remote `TestWorkerDecisionDigestRefusesAmbiguousBytesAndMismatches`.

Not done: no cross-module "policy and actions give the same verdict" test (the module `internal` boundary forbids one module's test importing another's domain); both consume the same contractsv1 functions and each has the same ambiguous-bytes case. Goldens and digests: none regenerated. The shadow-output canonical-bytes check (`canonicalShadowDecision`) and the recorded-ledger `DecodeRecordedDecision` canonical-bytes check remain replay-specific intake rules.
