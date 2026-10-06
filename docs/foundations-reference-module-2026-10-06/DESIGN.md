# Design

| Package | Layer | Responsibility | Must not |
| --- | --- | --- | --- |
| `canonicaljson` | facade | Re-exports `Domain` and its constants; one-line delegations of `Marshal`, `Digest`, `Verify`, `DecodeDigest`, `EncodeDigest`, `ContentDigest`, `VerifyStored` | Hold logic |
| `canonicaljson/internal/domain` | domain | RFC 8785 encoder, number and string rules, strict raw-JSON validation, the digest preimage and reference format, stored-document verification | Do I/O or read a clock |

No behavior, digest, identifier shape or wire format changes. The public API
names stay; callers do not change.
