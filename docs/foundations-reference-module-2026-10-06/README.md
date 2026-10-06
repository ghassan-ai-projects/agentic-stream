# Foundations as reference modules — October 2026

Working record for rebuilding `internal/canonicaljson` and `internal/ids` to the
reference-module standard, following the
[reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md).
Earlier cleanup rounds only removed dead code and duplicates; they did not apply
the architecture. This record does. Each package's vocabulary lives in its own
`UBIQUITOUS_LANGUAGE.md`.

- [Findings](FINDINGS.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

Both are leaf foundations that do no database or network I/O, so they take the
"pure rules" shape: a thin facade over `internal/domain`. `ids` also draws
entropy from the operating system, which is an external source, so that one
generator gets its own adapter package.

```
internal/canonicaljson/                 facade: Domain, Marshal, Digest, Verify, DecodeDigest, EncodeDigest, ContentDigest, VerifyStored
internal/canonicaljson/internal/domain/ RFC 8785 encoding, number and string rules, strict validation, digest and stored-document rules

internal/ids/                           facade: Generator, prefixes, Random, Deterministic
internal/ids/internal/domain/           identity-space prefixes, the Generator contract, the deterministic generator
internal/ids/internal/random/           adapter: crypto/rand generator
```

The facades sit one layer above their domains, so the layer table is re-derived
from the import graph (34 entries move up one level).
