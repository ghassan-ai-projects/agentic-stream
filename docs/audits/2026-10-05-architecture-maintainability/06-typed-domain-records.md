# 6. Use typed records instead of `map[string]any`

## Problem

Domain records such as decisions, commands, outcomes, reconsiderations,
approval notices, and device evidence are passed around as untyped
`map[string]any` documents. Production code uses `map[string]any` 656 times.
Field names are string literals. The compiler cannot catch a typo, and readers
cannot see the shape of a record.

## Evidence

Files with the most uses:

| File | Uses |
| --- | --- |
| `internal/episodes/reconsideration_request.go` | 20 |
| `internal/device/serial_session_exchange.go` | 19 |
| `internal/replay/baseline.go` | 14 |
| `internal/device/serial_session_safety.go` | 14 |
| `internal/decisions/catalog.go` | 14 |
| `internal/policy/policy_approval_notice.go` | 13 |
| `internal/actions/dispatch_reconcile.go` | 13 |

`reconsideration_request.go` builds a nested document by hand. It calls
`copyDocument`, `setDocumentIdentity`, `bindCommandContext`, and reads
`delta["correction"].(map[string]any)` directly. These are the record's
fields, written as map keys.

## Why it matters

- Renaming or adding a field means searching for strings across packages.
- Type assertions such as `.(map[string]any)` and `.(string)` fail at run time,
  and some of them silently fall back to a zero value.
- Each package that reads a document has to work out its shape again.

## Recommendation

1. For each aggregate, the owning package defines a typed struct with `json`
   tags that match the existing JSON Schema. Examples:
   `episodes.ReconsiderationRequest`, `actions.CommandRecord`,
   `policy.ApprovalNotice`.
2. Convert to `map[string]any` only where canonical JSON or digest code needs
   it, through one `ToDocument()` method. Digest inputs stay byte-for-byte the
   same.
3. Add a round-trip conformance test for each type. Validate the typed value
   against the embedded schema, and check that its canonical bytes equal the
   bytes the current map-based code produces.
4. Migrate one aggregate per PR. Start with reconsideration, which has the most
   uses and a contained blast radius.

Keep `map[string]any` where the data is open on purpose: CEL activation inputs
and raw event payloads. These are evidence, not instructions, so leaving them
untyped is correct.

## Done when

- Each migrated aggregate has a typed struct, a schema round-trip test, and an
  unchanged digest.
- Production `map[string]any` usage shrinks steadily, and a quality test tracks
  the count so it does not grow back.
