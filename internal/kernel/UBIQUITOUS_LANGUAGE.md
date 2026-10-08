# Kernel ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Kernel | The shared pure vocabulary: representation rules every module spells the same way. Standard library only; no database, file, network, random source or clock read. Any production package may import it without declaring the edge. | `kernel` | — |
| Durable time text | The text of an instant in a stored column, a digest preimage or a document: UTC with a fixed nine-digit fraction, so text order is chronological order. | `FormatTime`, `ParseTime` | `2026-01-01T00:00:00.000000000Z` |
| Digest text | The text of a 32-byte SHA-256 sum in contracts, documents and schemas: `sha256:` followed by 64 lowercase hex characters. | `EncodeDigest`, `DecodeDigest` | `sha256:ab…` |

## Admission rule

A symbol belongs here only when two or more modules need it and it is a stable
representation rule, not business behaviour. Reading the clock, generating ids
and anything that touches storage belong to `sources` and `storage`, which
other packages must declare as imports.

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| `sources.FormatTime`, `sources.ParseTime` | `kernel.FormatTime`, `kernel.ParseTime` | `sources` provides injected clocks and ids; time text is a representation rule. |
| `canonicaljson.Sum` for stored bytes in packages without that edge | stdlib `crypto/sha256` | A one-line standard library call needs no owner. |
