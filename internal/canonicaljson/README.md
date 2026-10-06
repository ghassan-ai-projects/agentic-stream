# Canonical JSON module

Canonical JSON is the foundation of contract identity: every digest the runtime binds (snapshots,
Decisions, intents, commands, policy, catalogs) is computed over its canonical encoding in a named
domain, and the same vectors are checked on the Ruby side.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | The public API: `Marshal`, `Digest`, `Verify`, `DecodeDigest`, `EncodeDigest`, `ContentDigest`, `VerifyStored`, and the `Domain` constants; one-line delegations only |
| Domain | The RFC 8785 encoder, number formatting (exact integers, shortest doubles), string escaping and UTF-16 key ordering, strict raw-JSON validation, the digest preimage and reference format, stored-document verification |

The module does no I/O, so it has no app or store layer. It is a layer-0 foundation: the foundation gate
lets it import only other foundations. Cross-repository vectors (`contractsv1/testdata`) and the frozen
Ruby digests are the contract tests; a change that moves a digest breaks both repositories.

[Migration record](../../docs/foundations-reference-module-2026-10-06/README.md).
