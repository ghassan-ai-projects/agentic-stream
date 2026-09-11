# A-021 · `internal/spec/compiler.go`

LOC: 349 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The context parameter is either honored or absent.
- Lookup results are never discarded, even when a prior check guarantees success.
- Schema validation and reference resolution do not re-walk the same data with nested linear scans when a map exists.
- Compiler state initialization is safe under concurrent use or documented as single-threaded with an enforcing comment.
- No identifier shadows the package's own name in a way that obscures cross-package reading.

## Findings
- **LOW F1. Context accepted and discarded** — `internal/spec/compiler.go:111`. `CompileBytes` takes `context.Context` and immediately discards it (`_ = ctx`). Schema compile/validation and canonicalization are CPU-bound, so nothing is honored today; either drop the parameter from the public surface (`CompileFile` propagates it) or thread it into schema validation if the library supports it.
- **LOW F2. Discarded lookup result behind a guarantee** — `internal/spec/compiler.go:310`. `definition, _ := eventschema.Lookup(in.SchemaRef)` discards `ok`; the earlier loop (lines 252-258) guarantees success, but the invariant lives two loops away and silently breaks if that loop changes. Reuse the input-name → definition map built at lines 246-259 instead of re-looking-up.
- **LOW F3. Redundant nested scans for field validation** — `internal/spec/compiler.go:303-320`. The `op.Field` check is O(operators × inputs × inputs) with an inner linear scan over `spec.Inputs` per input name; the `inputs` map from lines 246-252 maps names to existence only — widen it to carry definitions and index directly.
- **LOW F4. Lazy `init` is not goroutine-safe** — `internal/spec/compiler.go:31-55`. `c.schema` is lazily compiled without synchronization; a shared `Compiler` used from two goroutines races. Current callers are single-threaded (CLI, replay bootstrap), so document the constraint on `Compiler` or guard with `sync.OnceValues`.
- **LOW F5. Variable named `spec` inside package `spec`** — `internal/spec/compiler.go:150,163,172,194,245`. Shadowing the package's own name with a local variable makes cross-repo reading ambiguous next to `spec.` qualified references elsewhere; rename to `compiled`.

## Checked, not an issue
- P1: all errors wrapped with `%w`; YAML duplicate keys rejected pre-decode (lines 60-92) so last-key-wins ambiguity cannot enter the digest; network schema loading denied (lines 94-98) — untrusted refs cannot fetch instructions.
- P2: `apiVersion`/`kind` pinned before validation (lines 130-135); structural schema validation precedes normalization; schema refs cross-checked against the event registry including eventType/schemaVersion (lines 252-258).
- P5/P7: defaults applied in `normalize` (lines 208-240) so semantically equal specs share a digest; digest computed over the normalized struct with `CanonicalJSON`/`Digest` still at zero values — constant preimage, deterministic; embedded schema is the pinned v1 contract.
- P6: `compiler_test.go` compiles all three shipped examples and covers the watch-floor default; CEL expressions validated at compile time (`cel.go:31-77`), so runtime eval failures are not the first line of defense.
