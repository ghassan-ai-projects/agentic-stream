# Decisions validation

## Focused evidence

| Check | Result |
| --- | --- |
| `go test ./internal/decisions/...` | Pass |
| `go test -race -count=1 ./internal/decisions/...` | Pass |
| `go test -cover ./internal/decisions/...` | Facade 100%; domain 83.3% |
| `go test ./internal/episodes/... ./internal/replay/...` | Pass |
| Focused architecture tests for facade delegation, catalog privacy, import allowlists, layer order, domain purity, package docs and module-map coverage | Pass |
| `make docs-check` | Pass; 68 public pages and volatile surfaces verified |
| Whole-tree `golangci-lint` | Pass; zero issues |
| `make ci-check` and uncached non-short full race suite | Parent owns final repository gates; pending at this record's draft |

The new catalog isolation regression was first run before implementation and
failed: changing a caller-owned preset changed validation and produced
`preset_mismatch` for the original proposal. After making the compiled
authority opaque and copying its JSON containers, the original proposal passes
with the same canonical Intent bytes, and a proposal matching the mutated
source preset fails with `preset_mismatch`.

The regression also mutates the source parameter schema after compilation.
The compiled schema remains bound to the original string parameter contract.
The catalog test confirms remote schema references fail without network access.

## Review ratings

| Dimension | Rating | Evidence and remaining limit |
| --- | ---: | --- |
| Layering | 9/10 | Root facade contains only aliases and two delegations; domain owns pure validation. Shared graph gates pass in the focused run. |
| Domain rules | 9/10 | Trusted bindings, digest, schema, risk, preset, evidence and freshness checks are in one pure layer with precedence tests. Contract input remains JSON-shaped. |
| Fail-closed safety | 9/10 | Missing time/catalog, malformed digest, mismatches, unknown types, remote schemas and stale Intents reject. Full-tree gates remain pending. |
| Ubiquitous language | 9/10 | Canonical glossary, package guide and audit use Decision, Intent, attempt, snapshot and catalog authority consistently. |
| Tests | 9/10 | Existing adversarial/order tests plus compiled-authority and facade success tests; new domain coverage is 83.3%. Full repository suite is pending. |
| Data encapsulation | 8/10 | Compiled authority is opaque and source maps are copied. `Result.Intents`, `Intent.CanonicalJSON` and `ValidationError.Details` remain mutable values within the internal runtime. |
| Type safety | 8/10 | Public values are typed; contract documents and nested action parameters remain `map[string]any` and are constrained by shared JSON Schemas. |
| Simplicity | 9/10 | The package adds only the domain boundary required to isolate pure rules; no app/store layers or caching were added. |

## Remaining work and weaknesses

1. Complete full `make ci-check` and uncached `go test -race -count=1 ./...`
   through the parent integration. Their results may change the ratings.
2. Contract documents and validation details still use JSON-shaped maps. A
   typed projection may help if field access expands, while original Decision
   and Intent bytes remain the digest and audit evidence.
3. Catalog compilation repeats for each episode and replay setup. Profile before
   considering caching; any cache must key on verified catalog identity.

Independent review: whole-root architecture tests pass. Injected facade logic,
domain filesystem access and public catalog authority each failed their named
gate; fixtures were removed. Uncached race coverage is facade 100%, domain 83.3%.
