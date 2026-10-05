# Plan and round status

## Behavior that must not change

- Decision and Intent JSON schemas, canonicalization, digest domains and digest
  inputs; validated Intent bytes and the original raw Decision persisted by the
  episode owner.
- Fail-closed missing-time/catalog behavior, attempt-before-snapshot rejection
  order, Intent schema/digest-before-identity order, and each documented field
  and reason code.
- One-actionable-Intent rule; episode allowlist, reconsider-only compensation,
  catalog risk equality, risk ceiling, parameter schema, identity/preset/
  evidence checks, and freshness comparisons against the supplied `Now`.
- Episode/policy ownership of storage, replay parity, and no direct worker path
  to policy or action execution.

## Deliberate changes

- Make compiled catalog authority opaque and copy catalog JSON containers that
  are retained as authority so caller mutation cannot change later validation.
- Remove unused public `IntentEntry` / `IntentCatalog.Entries` and unused
  `Result.Document`, `Result.CanonicalJSON`, and `Intent.Document` projections.
  Production consumers continue to use original Decision bytes and canonical
  Intent bytes.
- Split pure rules behind the decisions facade. No I/O layer is introduced.

## Rounds

| Round | Change | Proof | Status |
| --- | --- | --- | --- |
| 0 | Survey, canonical glossary, findings, design and this plan | Dated folder and pre-change package/caller inventory | Complete |
| 1 | Move pure vocabulary, parsing, catalog compilation and validation to `internal/domain`; keep only aliases and delegating operations in root | Focused decisions, episodes and replay tests; facade/domain coverage; lint and diff checks | Complete |
| 2 | Encapsulate compiled catalog authority and remove unused result projections; adapt meaningful tests/callers if any hidden repository references are found | Source-mutation regression; successful digest/canonical-byte parity; full consumer tests | Complete |
| 3 | Record audit and final validation; prove shared import/layer/facade gates with injected violations | Architecture injection proofs; `make ci-check`; uncached non-short race suite | Planned |

Round 0 must be committed before implementation starts. The parent owns the
shared architecture tests, import allowlist, layer levels, module maps and git
commits. The subagent will report exact shared gate edits and wait for the
round-zero commit confirmation before code changes.

## Deferred work

- Profile repeated catalog compilation across attempts before adding any cache;
  caching must remain bound to verified catalog identity and must not weaken
  freshness or isolation.
- Consider narrower typed projections only if contract maps become difficult to
  maintain. Current JSON Schema validation plus retained original bytes is the
  smaller boundary and avoids duplicate representations.

## Status

Round 0 is committed as cef7ceb. Pure extraction and catalog encapsulation
are complete. Independent focused race, lint, architecture and documentation
checks pass; final full-tree gates will run after the integrated migrations.
