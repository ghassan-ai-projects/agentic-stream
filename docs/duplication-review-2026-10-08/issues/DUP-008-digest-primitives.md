# DUP-008: Digest plumbing: Digest-then-Decode, Marshal-then-Digest, raw SHA-256 and hand-built sha256: strings

- Status: fixed
- Severity: medium
- Verdict (finders): REAL
- Themes: business rules, contracts and shapes, persistence
- Wave: 2
- Finder sources: S6, S9, R11, P11 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `canonicaljson` (`DigestSum`, `Seal`, `VerifySum`, `Sum`, `HasSumLength`) and use the existing `EncodeDigest` everywhere. Output bytes must not change. Optional: make `DecodeDigest` reject uppercase hex only if no stored value has it.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S6: "Domain digest as bytes", "seal = canonical JSON + digest" and "verify stored sum" repeated around `canonicaljson.Digest`

- Verdict: REAL
- Shared meaning: `canonicaljson.Digest` returns a `sha256:<hex>` string, but storage wants the raw 32 bytes and often also the canonical JSON; each caller re-does `Digest` -> `DecodeDigest` (or `Marshal` + `Digest`, marshalling twice) and wraps it in its own errors.
- Sites (Digest then DecodeDigest to bytes): internal/episodes/internal/domain/snapshot.go:47-52; internal/episodes/internal/domain/decision.go:51-60; internal/cognition/internal/domain/correction.go:49-58; internal/actions/internal/domain/dispatch.go:93-101 (`OutcomeDigest`); internal/policy/internal/domain/commands.go:29-37 (`sealCommand`); internal/replay/internal/domain/shadow_snapshot.go:33-38; internal/replay/internal/domain/shadow_comparison.go:108-116 (`sealComparison`); internal/engine/internal/domain/version_write.go:56 + `DecodeStateDigest` :119-126; internal/engine/internal/domain/situation_state.go:129-137; internal/runartifact/internal/domain/verify.go:112-122; internal/decisions/internal/domain/validator.go:116 (`DecodeDigest` + `Verify`)
- Sites (Marshal then Digest of the same value): internal/executor/native/internal/app/loop.go:154-161; internal/executor/fixture/executor.go:105-115; internal/replay/internal/domain/baseline.go:164-175; internal/policy/internal/domain/commands.go:25-37; internal/replay/internal/domain/shadow_comparison.go:104-116; internal/executor/native/internal/domain/deterministic.go:28
- Sites (compare stored sum to recomputed domain digest): the verifier set in C3 (`bytes.Equal(decoded, persisted)` in snapshot.go:51, shadow_snapshot.go:37, correction.go:58, situation_state.go:133; `string(stored) == string(expected)` in runartifact verify.go:108; `computed != EncodeDigest(digest)` in remote stream.go:142)
- How they differ: three different comparison styles (`bytes.Equal`, string equality of re-encoded, `subtle.ConstantTimeCompare`); error text differs per site and some tests match on it; the double decode/encode hop is purely incidental.
- Risk if left: every new sealed document type (the shadow-comparison and device-catalog digests were added this way) copies the 8-line shape; a change in digest representation (for example storing text instead of BLOB) touches 17 files.
- Proposed canonical owner: `internal/canonicaljson` (layer 8, imported by every one of these packages already; no new allowedImports edge).
- Proposed fix: add `DigestSum(domain, v) ([]byte, error)`, `Seal(domain, v) (json []byte, digest string, err)` and `VerifySum(domain, v, sum []byte) bool` to the facade (`canonicaljson.go`, delegating to `internal/domain`); callers keep their own wrap messages. Fold C3's verifiers onto `VerifySum`.
- Behaviour to preserve: digest values and the `Digest`/`Verify` empty-domain and malformed behaviours; each site's error wrap text (wrapcheck rule in AGENTS.md); constant-time compare where used today.
- Verification: `canonicaljson_test.go` and `ruby_parity_test.go` pin the algorithm; add table tests for the three new functions (round trip, wrong length sum, wrong domain).

### Finder report S9: Raw SHA-256 of stored bytes and its 32-byte guard hand-rolled at ~25 sites

- Verdict: REAL
- Shared meaning: "the content hash of these stored bytes" (BLOB of 32 bytes) and "is this a complete 32-byte digest".
- Sites (compute): internal/notify/internal/domain/seal.go:34; internal/evidence/internal/wire/result.go:12, internal/evidence/internal/store/writes.go:25, internal/evidence/internal/wire/fingerprint.go:19; internal/authority/internal/domain/state.go:32, reconciliation.go:145, internal/authority/internal/store/events.go:75-83 (`canonicalDocument`, which is `Marshal`+raw sum); internal/spec/internal/store/event_schema_store.go:22; internal/engine/internal/domain/version_write.go:99, internal/engine/internal/store/operator_state.go:131; internal/eventlog/internal/domain/event.go:25, quarantine.go:26; internal/runartifact/internal/domain/files.go:72,139, verify.go:129; internal/executor/remote/internal/domain/request.go:160-161; two private helpers `sha256Sum` (internal/situations/internal/domain/situations.go:230, internal/episodes/internal/domain/intent_catalog.go:152)
- Sites (verify / guard): internal/notify/internal/domain/read.go:70; internal/evidence/internal/domain/reservation.go:47-52; internal/canonicaljson/internal/domain/stored.go:19-35 (`ContentDigest`, `VerifyStored` - the owner already exists but returns the text form and is used by only authority/device documents); `len(x) != sha256.Size` at actions/internal/domain/reconciliation.go:33, document.go:50,59, engine/internal/domain/situation_state.go:118, policy/internal/domain/routing.go:92, documents.go:17, runartifact/internal/domain/verify.go:139
- How they differ: canonicaljson exposes `ContentDigest` (string) and `VerifyStored` (bytes, with a canonical-form check) but everything else wants `[]byte`; some stored bytes are canonical JSON (authority, notify) and some are plain `json.Marshal` (eventlog payload, operator state, lineage references), so "verify stored" semantics differ. Intentional per row, but the hashing primitive is the same.
- Risk if left: swapping the hash (the stored column is named `*_sha256`, so low odds) or adding a length guard is a 25-site edit; two private helpers with the same body and different argument types.
- Proposed canonical owner: `internal/canonicaljson` (it already owns `ContentDigest`/`VerifyStored`; every listed package imports or may import it).
- Proposed fix: add `canonicaljson.Sum(data []byte) []byte` and `canonicaljson.HasSumLength(b []byte) bool`; replace the inline `sha256.Sum256` + `[:]` sequences and the two `sha256Sum` helpers; keep HMAC use in evidence token.go untouched.
- Behaviour to preserve: byte-identical digests for every column; do not canonicalise the payload hashes that are over raw `json.Marshal` bytes (eventlog, operator state) - those must stay "hash of stored bytes".
- Verification: eventlog/engine/evidence store tests and migrations constraints (`length(...) = 32`); add a unit test for `Sum` equal to `sha256.Sum256`.

### Finder report R11: Hand-formatted digests: `"sha256:" + hex.EncodeToString(...)` next to `canonicaljson.EncodeDigest`

- Verdict: REAL
- Shared meaning: the textual form of a stored 32-byte digest is `sha256:<lowercase hex>`; `canonicaljson.EncodeDigest` is the inverse of `DecodeDigest` and owns the prefix.
- Sites (all hand-format): internal/cognition/internal/store/evaluation_reads.go:65; internal/episodes/internal/store/decision_reads.go:31; internal/replay/internal/store/store.go:55 and :101; internal/replay/internal/store/recorded_ledger.go:60; internal/engine/internal/store/situation_reads.go:68; internal/episodeledger/internal/store/episode_reads.go:27.
- Sites (already correct): the 20+ callers listed by `grep EncodeDigest` (episodes/app/runner_claim.go:88-90, remote/domain/stream.go:142, actions/domain/document.go:53, etc.).
- How they differ: none today; the prefix literal appears in 7 stores while the owner has the const `digestPrefix` (canonicaljson/internal/domain/digest.go:40). `DecodeDigest` accepts uppercase hex whereas every JSON schema pattern requires `^sha256:[0-9a-f]{64}$` (14 occurrences); uppercase round-trips to a different string.
- Risk if left: if the digest format changes (new algorithm prefix, or case rule), these 7 sites keep producing the old form that `DecodeDigest` then rejects.
- Proposed canonical owner: `internal/canonicaljson.EncodeDigest` (rank 8). Stores in cognition/engine/replay/episodeledger need one added edge each to canonicaljson in architecture_test.go (cognition and episodes stores already have it; engine/store, episodeledger/store, replay/store do not). Alternative with no new edges: have the stores return raw bytes and let their domain mappers format.
- Proposed fix: replace the 7 expressions with `canonicaljson.EncodeDigest(x)`; optionally make `DecodeDigest` reject uppercase.
- Behaviour to preserve: identical output string, stored bytes unchanged.
- Verification: existing store read tests and experiment inspect tests. New: none beyond a grep-based architecture test forbidding the `"sha256:"+hex` pattern outside canonicaljson.

### Finder report P11: `"sha256:" + hex(bytes)` is hand-built in seven stores although `canonicaljson.EncodeDigest` exists

- Verdict: REAL
- Shared meaning: render a 32-byte BLOB digest column as the canonical `sha256:<hex>` text.
- Sites:
  - internal/cognition/internal/store/evaluation_reads.go:65 (policy_sha256)
  - internal/episodes/internal/store/decision_reads.go:31 (decision_sha256)
  - internal/replay/internal/store/store.go:55 and :101 (snapshot_sha256), recorded_ledger.go:60 (decision_sha256)
  - internal/engine/internal/store/situation_reads.go:68 (snapshot_sha256)
  - internal/episodeledger/internal/store/episode_reads.go:27 (snapshot_sha256)
  - Canonical: internal/canonicaljson/canonicaljson.go:51 `EncodeDigest` (used by ~19 domain/app/store call sites already, e.g. authority/internal/store/bindings.go:32, actions/internal/store/notifications.go:49,74) and the inverse `DecodeDigest` (39 call sites).
- How they differ: identical today; the prefix is also a private constant `digestPrefix` in canonicaljson/internal/domain/digest.go:40.
- Risk if left: a prefix/algorithm change (the domain-separated digest format is a versioned contract) would miss these seven stores and produce documents whose digest strings no longer verify.
- Proposed canonical owner: `internal/canonicaljson` (foundation, layer 8; every store may import it; check `internal/replay/internal/store`, `internal/engine/internal/store`, `internal/episodeledger/internal/store` allowedImports - they lack `internal/canonicaljson` and need that edge, which is a foundation package).
- Proposed fix: replace each concatenation with `canonicaljson.EncodeDigest(bytes)` (mechanical, gofmt/goimports).
- Behaviour to preserve: exact output string (lower-case hex, `sha256:` prefix); no digest input changes.
- Verification: any golden containing these strings (replay goldens, API snapshot tests); add one test in canonicaljson asserting `EncodeDigest(DecodeDigest(x)) == x`.

## Outcome

Status: fixed. Commit: 7268242.

Verified (all four finder reports held; the sites were re-opened and read):
- S6 held. Every listed `Digest` then `DecodeDigest` hop (episodes snapshot and decision, cognition correction, actions `OutcomeDigest`, policy `sealCommand`, replay shadow snapshot and `sealComparison`, engine `verifyStateDigest`, runartifact `expectedDigest`) was a pure round trip through text. The Marshal-then-Digest pairs (native loop, fixture executor, replay baseline, policy command, replay comparison) marshalled the same value twice; situations `snapshot` and spec `sealSpec` had the same shape and are fixed too.
- Partly wrong: `internal/executor/native/internal/domain/deterministic.go:28` only canonicalizes, it never digests, so it is not a clone and is untouched. `internal/decisions` `DecodeDigest` before `Verify` was redundant (Verify already fails for any non-canonical digest text) and is dropped. engine `DecodeStateDigest` and `DecodeSnapshotDigest`, and `situations.Version`, take digest strings by contract (public type, inbound text), so those decodes stay.
- S9 held for the stored-bytes hashes. Not touched on purpose: identity-derivation hashes (cognition reconsideration key, episodes admission key, replay `shortKey`, native `loop_tools`, episodeledger rejection, engine heartbeat, policy approval nonce), the streaming `sha256.New()` users, and HMAC in evidence `token.go`. They derive ids, they do not hash stored content, and the finder list excluded them.
- R11 held: seven stores hand-built `"sha256:" + hex`.
- Uppercase hex: confirmed that no stored value has it. Every digest column is a 32-byte BLOB (`*_sha256`), reads re-encode with `EncodeDigest` (always lowercase), every JSON schema pattern is `^sha256:[0-9a-f]{64}$`, and a repo-wide search (code, fixtures, testdata, docs) found uppercase digest text in exactly one place, the authority test constant `digestUpper`. So `DecodeDigest` now rejects uppercase hex.

Changed:
- `internal/canonicaljson` (facade, domain, tests, README, UBIQUITOUS_LANGUAGE): added `DigestSum`, `Seal` (canonical JSON plus raw domain sum, canonicalizes once), `VerifySum` (constant time, false for wrong length), `Sum`, `HasSumLength`; `Digest` now builds on `DigestSum`/`Seal`; `DecodeDigest` rejects uppercase hex; `ContentDigest`/`VerifyStored` use `Sum`/`HasSumLength`. Digest bytes are unchanged (golden and Ruby parity tests pass untouched).
- Digest-then-decode and double-marshal sites now call `DigestSum`/`Seal`: episodes `snapshot.go`, `decision.go`; cognition `correction.go` (`DecodeCorrection` returns the raw sum, `MatchCorrectionDigest` takes it; app caller renamed) ; actions `dispatch.go`; policy `commands.go` (`sealCommand` takes `[]byte`); replay `shadow_snapshot.go`, `shadow_comparison.go` (`sealComparison` deleted), `baseline.go`; engine `situation_state.go`; runartifact `verify.go`; decisions `validator.go`; executor native `app/loop.go`, fixture `executor.go`; situations `materialize.go`; spec `compiler.go`.
- Raw hashes and guards: `Sum`/`HasSumLength` replace inline `sha256.Sum256` and `len == sha256.Size` in notify (`seal.go`, `read.go`), evidence (`wire/fingerprint.go`, `wire/result.go`, `store/writes.go`, `domain/reservation.go`, new `QueryResult.SHA256()` so wire, store and the reservation check share one rule), authority (`state.go`, `reconciliation.go`, `store/events.go`), spec `store/event_schema_store.go`, engine (`version_write.go`, `situation_state.go`, `store/operator_state.go`), eventlog (`event.go`, `quarantine.go`), runartifact (`files.go`, `verify.go`), executor remote `request.go` (arrays became slices), actions (`document.go`, `reconciliation.go`), policy (`routing.go`, `documents.go`, which also folds onto `VerifySum`). The two private `sha256Sum` helpers (situations, episodes) are deleted.
- R11: the seven hand-built digests now call `canonicaljson.EncodeDigest` (cognition `evaluation_reads.go`, episodes `decision_reads.go`, replay `store.go` x2 and `recorded_ledger.go`, engine `situation_reads.go`, episodeledger `episode_reads.go`).
- Gates: `architecture_test.go` allowedImports gained `internal/canonicaljson` for engine/store, episodeledger/store, replay/store, evidence/domain, evidence/wire, eventlog/domain. `architecture_flow_test.go`: `internal/canonicaljson` moved from layer 8 to 1 (it imports only its layer-0 domain; layer 8 made the new edges from layer-8 packages sideways imports).

Decisions:
- `Seal` returns the raw sum, not the text digest; callers needing text call `EncodeDigest`. This avoids a re-decode and matches the BLOB columns.
- Error text: the paired "marshal X"/"digest X" wraps became one "seal X" wrap (policy command, replay comparison and baseline, fixture executor, situations snapshot, spec); no test asserted the old text. The native loop's two failure codes `decision_digest_failed` and `decision_canonicalization_failed` collapsed to `decision_canonicalization_failed` (no other reference existed).
- Intentional behaviour change: `DecodeDigest` rejects uppercase hex. The authority test row "digest compares by value" (`digestUpper`) now expects a conflict and was renamed "uppercase digest is not a canonical digest"; `sameDigest` comment updated.
- Not folded onto `VerifySum`: runartifact `verifyRowDigest` (it mixes a domain digest with a plain content hash and has two distinct error messages) and episodes/replay/engine/cognition verifiers (they need the digest or the error from the same computation, so they compare the sum from `DigestSum` with `bytes.Equal`, as before).

Deferred: test helpers that still do `Digest` then `DecodeDigest` (cognition, policy, executor remote tests); other fixers edit those files and the dupl gate does not flag them.

Pinned by: `TestDecodeDigestRejectsMalformedReferences`, `TestSealCanonicalizesOnceAndMatchesDigestSum`, `TestSealAndDigestSumRefuseEmptyDomainAndUnencodableValues`, `TestVerifySumBindsSumToValueAndDomain`, `TestSumIsTheRawSHA256OfTheBytes`, `TestHasSumLengthRequiresExactlyThirtyTwoBytes` (canonicaljson domain), `TestFacadeRawSumOperationsAgreeWithTextForm` (facade), `TestDigestTextHasOneOwner` (root: no production package outside canonicaljson concatenates the bare `"sha256:"` prefix), `TestDecideBinding` (uppercase case).

Not regenerated: no golden, pin or fixture changed; all digest values are byte-identical.
