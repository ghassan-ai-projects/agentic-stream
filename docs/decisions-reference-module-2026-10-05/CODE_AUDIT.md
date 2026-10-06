# Decisions code audit

The audit distinguishes production callers from package tests and records the
disposition of every public shape removed or retained during the migration.

| Candidate | Evidence | Decision |
| --- | --- | --- |
| `IntentCatalog.Entries` | No production caller outside `internal/decisions` reads the map; it held compiled risk, schema, presets and policy flags by mutable pointer. | Removed from the public surface. `IntentCatalog` has a private entries map and private entry type. |
| `IntentEntry` | Only internal catalog compilation and validation used the type. No production caller named it. | Made private as `intentEntry`. |
| Caller-owned schema/preset maps | `catalogPresets` previously retained nested maps from the input. Mutating the source after compilation changed the authority used by validation. | Deep-copy JSON maps and arrays before retaining preset values; compile from a copied schema map. A regression mutates nested source maps after facade compilation, checks the original proposal still passes and checks a proposal matching the mutated preset is rejected. |
| `Result.Document` | No production consumer read it. Episode storage persists `Outcome.DecisionJSON`, the original worker bytes. | Removed. Parsed JSON remains local to validation. |
| `Result.CanonicalJSON` | No production consumer read it. Decision persistence uses source bytes and the verified digest. | Removed. Decision digest remains in `Result.DecisionDigest`. |
| `Intent.Document` | No production consumer read it. | Removed. Canonical Intent bytes and digest remain available for the episode-to-policy handoff. |
| Test fixture helpers (`validInput`, `validDecision`, `testCatalog`, digest helpers) | Referenced only from decision validator tests. | Kept under `internal/domain/*_test.go`; they are not production APIs. |
| `denyNetworkLoader.Load` | Used by catalog compilation as the `jsonschema.Loader` implementation. | Retained; it enforces the no-remote-schema rule. |
| `Input`, `Result`, `Intent`, `IntentCatalog`, `ValidationError` | Production consumers use these values in episode and replay flows; episode persistence needs Intent digest/canonical bytes, and audit processing needs the typed rejection reason. | Retained as facade aliases to domain vocabulary. The catalog alias exposes no authority fields. |

## Consumer evidence

- Episodes constructs `Input`, calls the public compiler and validator, reads
  `Result.DecisionID` / `DecisionDigest` / `Intents`, and persists Intent ID,
  type, risk, digest, canonical JSON, expiry, rate limit and approval flag.
- Replay compiles the same catalog, validates baseline and candidate outputs
  through `Validate`, and retains the validated result for shadow comparison.
- Episode rejection recording uses `errors.As` to inspect
  `ValidationError.Reason`; its fallback behavior is unchanged.
- The test-only construction of `decisions.Result` in episode shadow scoring
  remains possible through the public typed result fields. It does not require
  the removed projections.
- There are no decisions-owned tables or SQL. Decision records remain episode
  owned and governed Intent mutations remain policy owned.

No dead production function or code path was identified after moving all pure
helpers together. The domain exports `Validate` and `CompileIntentCatalog` only
because the sibling facade delegates to them; import restrictions prevent
other modules from bypassing the facade.
