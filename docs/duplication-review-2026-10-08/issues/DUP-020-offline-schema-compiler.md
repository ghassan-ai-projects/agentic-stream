# DUP-020: The offline JSON-Schema compiler is copied in four modules

- Status: fixed
- Severity: medium
- Verdict (finders): REAL
- Themes: contracts and shapes
- Wave: 1
- Finder sources: S10 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Owner: `canonicaljson` (the spec module is below contractsv1). Keep the loader error text that spec tests assert. Add a test that an http `$ref` is refused and that formats are asserted.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report S10: Offline JSON-Schema compiler (`AssertFormat` + deny-network loader) and lazy compile cache copied in four modules

- Verdict: REAL
- Shared meaning: compile an embedded or in-memory JSON Schema with format assertion on and external loading denied.
- Sites:
  - internal/contractsv1/internal/domain/schemas.go:59-130 - `loadSchema` (mutex + map cache), `compileEmbeddedSchema`, `offlineCompiler`, `denyNetworkLoader` ("external schema load denied")
  - internal/notify/internal/domain/lifecycle_schema.go:51-101 - `loadSchema` (mutex + single cache), `compileContractSchema`, `contractDocument`, its own `denyNetworkLoader` ("external notification schema load denied")
  - internal/spec/internal/domain/compiler_schema.go:28-52 - `compileSpecSchema` with its own `denyNetworkLoader` and the same four lines
  - internal/decisions/internal/domain/catalog.go:25,35-37,180 - catalog compiler with its own `denyNetworkLoader`
- How they differ: only the loader error text and the cache shape (per-name map vs single value vs per-compiler). Nothing else; the two embedded-resource ones also repeat read -> `json.Unmarshal` -> `AddResource` -> `Compile` with near-identical wrap messages.
- Risk if left: a change in the validation policy (for example disallowing `$ref` to anything but `urn:`, or turning off `AssertFormat`) must be made 4 times; one forgotten copy is a silent policy hole in a module that validates untrusted model output (decisions) or operator input (spec).
- Proposed canonical owner: spec is layer 11 (< contractsv1 12), so contractsv1 is not importable from spec. Put the helper in `internal/canonicaljson` (layer 8; "contract and digest primitives") as `canonicaljson.CompileSchema(id string, document any) (*jsonschema.Schema, error)`, or in a new layer-0 package if the maintainers dislike a JSON-Schema dependency in canonicaljson. Edges needed if canonicaljson: spec/internal/domain already imports it; decisions/internal/domain and notify/internal/domain already import it; contractsv1/internal/domain already imports it.
- Proposed fix: one `CompileSchema` (offline loader, `AssertFormat`) plus a `sync.OnceValues`-style cache helper; contractsv1, notify, spec, decisions call it; delete four `denyNetworkLoader` types.
- Behaviour to preserve: no network access (add a test that a `$ref` to http fails), format assertions, error strings checked in spec compile tests (`external schema load denied`).
- Verification: contractsv1 schema tests, spec compiler tests, notify `lifecycle_contract_test.go`, decisions catalog tests. New: one test of the shared compiler for denied external load and format assertion.

## Outcome

Verified: all four sites confirmed. Each built `jsonschema.NewCompiler()` + `AssertFormat()` + a private `denyNetworkLoader`; contractsv1, notify and spec also repeated read/unmarshal/AddResource/Compile. The only differences were the loader text (notify said "external notification schema load denied") and the cache shape. One extra difference found: decisions shared a single compiler across all catalog entries.

Changed:
- `internal/canonicaljson/internal/domain/schema.go` (new): `CompileSchema(id, document)` and `CompileSchemaJSON(id, data)` over one private `offlineCompiler` (AssertFormat, deny-network loader, "external schema load denied: <url>"). Facade delegations added in `internal/canonicaljson/canonicaljson.go`; README rows updated. No allowedImports edit was needed (every consumer already imports canonicaljson).
- contractsv1 `schemas.go`, notify `lifecycle_schema.go`, spec `compiler_schema.go`, decisions `catalog.go`: call the owner; the four `denyNetworkLoader` types, `offlineCompiler`, `embeddedSchemaDocument`, `contractDocument` and `compileSpecSchema` are deleted. notify's mutex cache became `sync.OnceValues`. spec's per-Compiler cache and contractsv1's per-name map are kept (different shapes, one use each). spec's `prepareSchema` now wraps the error.
- `contractsv1/internal/domain/envelope_test.go`: `TestSchemaLoaderDeniesNetwork` removed (it tested the deleted private type); the rule is now pinned at the owner.

Decisions: decisions now compiles each catalog entry with a fresh compiler instead of one shared compiler. A parameter schema could previously `$ref` an earlier entry's URN; that now fails closed. No catalog uses `$ref`. Error text changed only in wrapping (notify's loader text now matches the others; compile errors gain the schema id); no test asserted the old strings.

Pinned by: `TestCompileSchemaPolicy` (http and file `$ref` refused with "external schema load denied", date-time and email formats asserted, malformed and non-JSON schemas) and `TestCompileSchemaAcceptsDecodedDocument` in `internal/canonicaljson/internal/domain/schema_test.go`; existing consumer tests (decisions `$ref https://example.invalid` catalog test, spec/contractsv1/notify suites) still pass.
