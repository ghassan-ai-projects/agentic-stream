# DUP-008: Digest plumbing: Digest-then-Decode, Marshal-then-Digest, raw SHA-256 and hand-built sha256: strings

- Status: open
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

Not started.
