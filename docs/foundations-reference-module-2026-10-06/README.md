# Foundations as reference modules — October 2026

Working record for rebuilding `internal/canonicaljson` to the
reference-module standard (`internal/ids` was reviewed and deliberately left as it is), following the
[reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md).
Earlier cleanup rounds only removed dead code and duplicates; they did not apply
the architecture. This record does. Each package's vocabulary lives in its own
`UBIQUITOUS_LANGUAGE.md`.

- [Findings](FINDINGS.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

`canonicaljson` does no database or network I/O, so it takes the "pure rules" shape: a thin
facade over `internal/domain`. The facade sits one layer above its domain, so the layer table
was re-derived from the import graph (about 30 entries move up one level). `ids` is not
migrated: it is an 80-line foundation whose generators the deterministic packages may not
import, and a split would not change who can use it.

```
internal/canonicaljson/                 facade: Domain, Marshal, Digest, Verify, DecodeDigest, EncodeDigest, ContentDigest, VerifyStored
internal/canonicaljson/internal/domain/ RFC 8785 encoding, number and string rules, strict validation, digest and stored-document rules
```
