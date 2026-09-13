# A-069 · `internal/ids/ids.go`

LOC: 114 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported type, constructor, and constant has at least one production consumer.
- One deterministic ID scheme exists, not two divergent ones.
- The package has tests covering uniqueness, format, and prefix handling.
- Identity prefixes used by other packages come from this registry, not local string literals.

## Findings
- **MED F1. `PrefixSequence` and its `Reset` are dead exported code** — `internal/ids/ids.go:75-98`. Zero references repo-wide, including tests. It also duplicates `deterministicGenerator` (lines 63-73) — the same "mutex + counter + `%016x`" logic with per-prefix vs global scoping that can diverge. Delete `PrefixSequence`, or replace `Deterministic` with it if per-prefix sequences are the actual requirement.
- **MED F2. No tests for the package** — `internal/ids/` contains only `ids.go`. Uniqueness under concurrency (`randomGenerator`, `Sequence`), format stability (`%016x` width), and prefix concatenation are untested; this package's output feeds durable records, which is exactly where format changes bite. Add a table-driven test (`t.Parallel` where safe).
- **LOW F3. Four prefix constants unused; callers hard-code prefixes instead** — `internal/ids/ids.go:15,21,29,30`. `PrefixSituation`, `PrefixIntent`, `PrefixArtifact`, `PrefixReplay` have zero references, while `internal/situations/situations.go:223,232` hard-codes `"sit_"`/`"occ_"` (the latter matching no constant at all) and `internal/ingress/live_socket.go:67` hard-codes `"live_uds_"`. Route all identity spaces through this registry or shrink it to the used set.
- **LOW F4. `Sequence` is test-only in production terms** — `internal/ids/ids.go:100-114`. Its only consumer is `internal/operators/operators_test.go` (test-fixture IDs). Keep only if the engine tests standardize on it; otherwise fold into `Deterministic`.

## Checked, not an issue
- P1: `crypto/rand` failure panics with context (line 51) — acceptable for a catastrophic-only failure mode; mutexes correct; `Sequence` uses `atomic.Uint64`.
- P2/P7: `Random` IDs are 12 bytes of CSPRNG, base64url — collision-safe and URL-safe; `Deterministic`/`Sequence` output is bit-for-bit reproducible for identical call sequences, satisfying replay.
- P5: all exported symbols documented; interface (`Generator`, line 35) has many production implementations and consumers.
