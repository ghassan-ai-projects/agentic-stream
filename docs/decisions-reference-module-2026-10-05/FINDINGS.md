# Findings

These findings describe the package before implementation changes. Production
files, package tests, production callers and relevant architecture gates were
surveyed on 2026-10-05.

## Responsibility and layering

- `internal/decisions/{binding,catalog,intent,intent_parameters,validator}.go`
  mix the public operations with pure decoding and decision rules. They do not
  contain SQL, network, filesystem or clock reads. This is a pure-rules package,
  so a facade plus `internal/domain` is sufficient; app/store layers would add
  no current responsibility.
- The package imports only `canonicaljson` and `contractsv1`, plus the JSON
  Schema dependency. It owns no durable tables and has no SQL mutations.
- `internal/episodes/internal/store/decision.go` writes Decision rows and
  episode-proposed Intent rows. `internal/policy/internal/store` owns policy
  evaluation and governed Intent changes. These are declared separately in
  `durableOwners`; the validator does not query or write either table.

## Public surface and safety

- Production callers use `Validate`, `CompileIntentCatalog`, and the `Input`,
  `Result`, `Intent`, `IntentCatalog`, and `ValidationError` types. Callers read
  validated Intent provenance and canonical bytes, and use the typed rejection
  reason.
- No production caller reads `IntentCatalog.Entries` or names `IntentEntry`.
  Those fields expose mutable risk, schema, preset and approval authority.
- `Result.Document`, `Result.CanonicalJSON`, and `Intent.Document` have no
  production consumer. Decision persistence uses the original bytes in
  `Outcome.DecisionJSON`, while Intent persistence uses `Intent.CanonicalJSON`.
- The package already fails closed when trusted time or the compiled catalog is
  absent. It validates the attempt and snapshot against `Input`, never against
  worker-supplied trusted context. The catalog compiler blocks external schema
  loads.
- `Input` remains a caller-populated value because episode execution and replay
  each bind their own trusted context. There is no runtime dependency that
  belongs in `New(Config)`.

## Vocabulary and untyped contract data

- Worker Decisions and Intents are JSON contract documents validated by
  `contractsv1`; `map[string]any` is currently the parser's schema input.
- Trusted episode identity and authority arrive as a distinct `Input`. Intent
  type catalog authority and the per-episode allowed type subset are separate
  checks.
- A Decision is canonicalized and checked against `DomainDecision`; each Intent
  digest is verified independently before binding and risk checks. Existing
  field paths and reason strings are consumed by episode audit records.
- The package and the storage layer use the same Decision/Intent terms, but
  catalog entries and request allowlists need explicit definitions in the
  canonical glossary.

## Smaller defects and test-only code

- All validation code lives in the root package; moving it behind the facade
  will make architecture enforceable without introducing an I/O layer.
- Catalog authority is mutable from consumers and retains preset maps from the
  caller-owned catalog document. A caller can change those values after
  compilation. The migration will make entries private and copy nested JSON
  authority values on compilation.
- `validInput`, `validDecision`, catalog builders and digest helpers are used
  only by package tests. They remain test fixtures under the domain tests.
- `IntentEntry` is used within production validation but has no production
  consumer outside the package; the public type itself can become private.
- Existing tests include compensating-intent scope, stale attempt before
  snapshot mismatch, target-to-entity binding, preset equality, evidence
  grounding, duplicate JSON keys and digest-before-identity rejection. These
  are behavior contracts, not dead tests.

## Architecture gates

- `packageLayers` currently places `internal/decisions` at level 2. The facade
  will move to level 3 and its domain stays at level 2.
- Episode domain currently imports the decisions facade at level 2. That edge
  requires the episode domain, store, app and facade, then executor/admission
  dependents and runtime composition chain, to move up. Replay domain also
  imports the facade and moves to level 4.
- `allowedImports` needs the new decisions domain package, the root facade's
  domain edge, and removal of root imports now owned by domain. Exact graph
  changes are reported to the parent for the shared gate commit.
- The security path gate must keep `internal/decisions` in the reasoning set;
  no additional forbidden edge is proposed.
- `documentation/architecture/repository-map.md` and public module guidance
  should describe decisions as a pure validator with a thin facade and domain.
