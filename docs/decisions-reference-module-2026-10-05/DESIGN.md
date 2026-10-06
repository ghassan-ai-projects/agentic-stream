# Design

## Layer shape

`decisions` is a pure-rules package. The target has two layers:

| Layer | Responsibility | Must not |
| --- | --- | --- |
| `internal/decisions` | Public aliases for supported domain values and short `Validate` / `CompileIntentCatalog` delegation | Parse JSON, inspect fields, compile schemas, decide acceptance, read clocks, touch storage or expose compiled catalog entries |
| `internal/decisions/internal/domain` | Validation vocabulary, catalog compilation, contract parsing, bindings, freshness, risk, parameter and evidence rules | Read clocks, perform I/O, use SQL, own persistence or call policy/actions |

No app, store, transport or wire layer is justified: there is no use-case
orchestration, table ownership, socket, external system or codec boundary here.
Canonical JSON and `contractsv1` schema validation are pure contract
dependencies. The JSON Schema compiler is used only to compile and apply the
provided parameter schemas; external schema loading stays denied.

## Supported public surface

| Public name | Purpose |
| --- | --- |
| `Input` | Trusted episode identity, snapshot, allowlist, risk ceiling, compiled catalog, kind and time |
| `Result` | Validated Decision identity/digest and ordered validated Intents |
| `Intent` | Validated Intent identity, classification, expiry, digest, canonical bytes and policy metadata |
| `IntentCatalog` | Opaque compiled authority created by `CompileIntentCatalog` |
| `ValidationError` | Stable rejection reason and structured field/message details |
| `Validate` | Validate one worker Decision against a trusted `Input` |
| `CompileIntentCatalog` | Compile a non-empty, closed authority view with remote schema references denied |

`IntentEntry` and its map are private. Unused document projections are removed
from `Result` and `Intent`; the original Decision remains available to episode
storage at its source, and canonical Intent bytes remain in `Intent` for the
episode-to-policy handoff.

## Rules every operation follows

`Validate` preserves the existing sequence: require trusted time, require the
compiled catalog, canonicalize and parse the Decision, validate its shared
schema, verify the Decision digest, bind its ID, attempt and fence before
snapshot and Situation identity/version, then check Decision freshness. It
requires explicit `need_more_evidence` for an empty Intent list and allows at
most one actionable Intent. It visits Intents in document order; each Intent's
schema and digest are checked before identity, authority, parameters and
expiry. Within those stages, it preserves current field-check order and
rejection reason/field values.

Intent authority first checks compensation scope or the episode allowlist,
then catalog membership, declared risk equality and the episode ceiling.
Parameters must pass the compiled schema, entity/expiry/target binding, preset
equality and evidence grounding. Expiry is compared with `Input.Now`. No
validator path reads wall time or decides whether a proposal may execute.

Catalog compilation rejects an empty catalog, missing/duplicate types, invalid
risk, missing schemas and schema compiler errors. It keeps format assertion,
local schema compilation and denied external loads. Compiled entries and their
nested preset authority are not mutable by callers after compilation.

## Enforcement and consumer updates

| Concern | Proof |
| --- | --- |
| Facade delegates only | AST test over facade production files |
| Domain remains pure and below facade | `TestDomainPackagesArePure`, `packageLayers`, `allowedImports` |
| Contract bytes and rejection order | Existing Decision/Intent digest and field-precedence tests in the domain package |
| Compiled authority cannot be mutated through source maps or exported entries | Regression test mutates source catalog after compilation and confirms its authority does not change |
| Episode storage still records the source Decision and canonical Intent handoff | Existing episode Decision storage and Intent insertion regressions |
| Replay validates both outputs through the same rules | Existing replay shadow validation tests |

The parent owns shared architecture gates and maps. The decisions extraction
adds only `internal/decisions/internal/domain`, the facade edge to that domain,
and the actual domain imports. No schema, migration, protocol or dependency
change is planned.

## Deliberate behavior/API changes

- Compiled catalog entries become opaque; production callers do not inspect
  them, and this prevents post-compilation mutation of risk, schema, presets
  and approval authority.
- Caller-owned catalog preset/schema JSON containers are copied before the
  compiled catalog retains their authority, so later mutation of the source
  values has no effect.
- Remove `IntentEntry` and `IntentCatalog.Entries` from the public surface.
- Remove `Result.Document`, `Result.CanonicalJSON`, and `Intent.Document`; repo
  production callers do not use them, and the durable Decision path continues
  to use original `Outcome.DecisionJSON` bytes while the Intent handoff keeps
  `Intent.CanonicalJSON`.

These changes do not alter Decision or Intent digest input, digest output,
schema, error ordering, reason strings, trusted clock reads, persistence bytes
or policy ownership.
