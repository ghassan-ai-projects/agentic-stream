# Canonical JSON module

Canonical JSON is the foundation of contract identity: every digest the runtime binds (snapshots,
Decisions, intents, commands, policy, catalogs) is computed over its canonical encoding in a named
domain, and the same vectors are checked on the Ruby side.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | The public API: `Marshal`, `Digest`, `DigestSum`, `Seal`, `Verify`, `VerifySum`, `DecodeDigest`, `EncodeDigest`, `ContentDigest`, `Sum`, `HasSumLength`, `VerifyStored`, `CompileSchema`, `CompileSchemaJSON`, and the `Domain` constants; one-line delegations only |
| Domain | The RFC 8785 encoder, number formatting (exact integers, shortest doubles), string escaping and UTF-16 key ordering, strict raw-JSON validation, the digest preimage and reference format, stored-document verification, the offline JSON-Schema compiler (format assertions on, every external `$ref` refused) |

The module does no I/O (the offline schema compiler never loads external documents), so it has no app or store layer. Its domain package is a layer-0 foundation: the foundation gate lets it import only other foundations (it reads the shared `sha256:<hex>` text rules from `internal/kernel`). The facade sits at layer 8, like every other module facade. Cross-repository vectors (`contractsv1/internal/domain/testdata`) and the frozen
Ruby digests are the contract tests; a change that moves a digest breaks both repositories.

Migration record (`docs/foundations-reference-module-2026-10-06/README.md`).
