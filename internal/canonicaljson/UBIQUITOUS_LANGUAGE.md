# Ubiquitous language — canonical JSON

The words of contract identity. Code, storage and the Ruby side of the contract use the same
names.

| Term | Meaning | Code name | Storage / wire name |
| --- | --- | --- | --- |
| Canonical JSON | The one byte encoding of a JSON value (RFC 8785 / JCS): sorted keys, shortest numbers, minimal escapes | `Marshal` | `*_json` columns |
| Strict profile | The runtime's producer rules on top of JCS: no duplicate keys after unescaping, no lone surrogates, exact integers within ±(2^53−1), no non-finite or negative-zero numbers | `validate*` | — |
| Domain | The contract namespace mixed into a digest so two kinds of document never share a digest | `Domain`, `DomainDecision`, … | preimage prefix (ends in a newline) |
| Digest | `sha256:` plus the hex SHA-256 of domain + canonical bytes | `Digest` | `sha256:<hex>` |
| Reference | The text form of a digest; the `sha256:<hex>` rule itself is owned by `internal/kernel` | `EncodeDigest` | `sha256:<hex>` |
| Storage form | The 32 raw bytes of a digest, as stored in BLOB columns; `DigestSum` and `Seal` produce it directly | `DecodeDigest`, `DigestSum`, `Seal` | `*_sha256` columns |
| Content digest | The raw SHA-256 of stored bytes, without a domain; `Sum` is its raw form | `ContentDigest`, `Sum` | `*_sha256` columns |
| Stored document | Canonical bytes read back from storage; it must be canonical before its digest is compared | `VerifyStored` | `*_json` + `*_sha256` |
| Verify | Recompute a domain digest and compare it in constant time | `Verify` | — |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `MarshalString` | `string(Marshal(v))` | Test-only helper |
| `DomainTest` | a local test constant | Test-only value in the public constants |
| `"sha256:" + hex.EncodeToString(sum)` | `EncodeDigest(sum)` | One reference formatter |
