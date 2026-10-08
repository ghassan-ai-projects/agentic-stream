# DUP-010: The situation state document is written as a map in situations and read as a struct in engine

- Status: open
- Severity: medium
- Verdict (finders): DIVERGED
- Themes: contracts and shapes
- Wave: 3
- Finder sources: S7 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `situations`. Persisted `state_json` and `state_sha256` must stay byte-identical. The `occ-` fallback in engine should be removed if unreachable outside tests; otherwise route it through one `situations.OccurrenceID`.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S7: Situation state document: written as a map in `situations`, read as a struct in `engine`; its digest is recomputed in four places; occurrence id fallback uses a different prefix

- Verdict: DIVERGED
- Shared meaning: the persisted runtime state of a Situation (situation_id, occurrence_id, partition_id, version, facts, evidence, condition_start, traceparent, tracestate) and its `DomainSituationState` digest.
- Sites:
  - internal/situations/internal/domain/materialize.go:158-169 - `stateDocument` writes the map (nine keys, `traceparent`/`tracestate` always present)
  - internal/engine/internal/domain/situation_state.go:14-24 - `SituationState` reads the same keys as a struct (`traceparent,omitempty`)
  - internal/situations/internal/domain/materialize.go:111-127 `persistedState` and internal/situations/internal/domain/situations.go:146-156 `stateDigest`: both do "unmarshal blob to map then `Digest(DomainSituationState)`", while `CurrentState` (situations.go:123-133) calls `stateJSON` then `stateDigest` and `persistedState` does the same in one function
  - internal/engine/internal/domain/version_write.go:46-61 `situationStateDigest` and situation_state.go:122-139 `verifyStateDigest`: the same unmarshal + digest again (second and third decode of the same blob)
  - internal/situations/internal/domain/situations.go:223 - `OccurrenceID: "occ_" + hash`; internal/engine/internal/domain/version_write.go:63-68 - fallback `"occ-" + version.SituationID`
  - internal/engine/internal/store/operator_state.go:131 - the operator-state blob is stored with a raw `sha256.Sum256(json.Marshal(...))` (non-canonical bytes, raw digest), unlike the situation state (canonical bytes, domain digest) beside it
- How they differ / already diverged: `occ_<hash>` vs `occ-<situationID>` (different separator, different derivation); the writer and reader of the state document are separate definitions, so `condition_start` formatting (writer uses `sources.FormatTime`, reader parses with a private `time.Parse` at situation_state.go:73,160) is agreed only by convention; the situation-state digest is domain-separated while the operator-state digest in the same transaction is a raw hash of non-canonical JSON.
- Risk if left: a new state field must be added to the writer map and the reader struct in step (a missing reader field silently drops it on restore, then the digest check fails only if the field is nondeterministic); the `occ-` fallback produces a second occurrence-id scheme for any version built without an occurrence id.
- Proposed canonical owner: `internal/situations` (layer 18; engine already imports it). Export the state document as a typed struct with `Encode()`/`Decode()` and `StateDigest(blob)`; engine's `SituationState` becomes an alias.
- Proposed fix: move `SituationState` into situations/domain, marshal the struct in `stateJSON`, expose `situations.DecodeState` and `situations.StateDigest`; delete engine's `situationStateDigest`, `verifyStateDigest`'s decode half and situations' duplicate `stateDigest`; make `occurrenceIDOf` call a single `situations.OccurrenceID(situationID)` (confirm the `occ-` fallback is only reachable from tests; if so, delete it).
- Behaviour to preserve: persisted `state_json` bytes and `state_sha256` for existing rows (codec version 1), `StateCodecVersion` handling, restore error texts ("requires rebuild", "persisted state digest mismatch").
- Verification: engine restore tests, situations materialize tests, replay golden `versions_hash`. New: encode/decode round trip equals current golden state JSON; digest of encoded struct equals digest of the map form.

## Outcome

Not started.
