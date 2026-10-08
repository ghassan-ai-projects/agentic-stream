# DUP-007: Stored intent/decision/snapshot/command verification is implemented separately in twelve places

- Status: open
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

Not started.
