# A-073 · `internal/episodes/rebind_test.go`

LOC: 584 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Episode/situation seeding logic is shared with the other episode fixtures in the package instead of re-implemented per file.
- No literal in the test silently mirrors an unexported production constant without a compile-time or behavioral pin.

## Findings
- **[MED] F1. Seeding helper re-implements the package's existing episode seeder** — `internal/episodes/rebind_test.go:69-168` vs `internal/episodes/p8_freshness_test.go:23-110` (and a third variant, `p8_shadow_test.go:20`). Both do the same `PRAGMA foreign_keys OFF/ON` dance and near-identical `episodes`/`situations`/`situation_versions` insert blocks differing only in parameterization (real vs placeholder snapshots, rebind-count column). One shared `seedAdmittedEpisode(t, db, opts)` in the package's test scope would prevent the three copies drifting when the episodes schema changes.
- **[LOW] F2. Literal 3 pins unexported `maxStaleRebinds`** — `internal/episodes/rebind_test.go:351-357` vs `internal/episodes/executor.go:67`. The comment acknowledges the sync burden; changing the constant silently invalidates the test's premise (it would fail, but without pointing at the constant). Export the bound for tests or derive it (e.g., run re-binds in a loop until abandonment and assert the count matches the constant via one exported accessor).
- **[LOW] F3. `context.Background()` throughout; `t.Fatal` inside deferred PRAGMA restore** — `:71,105-112`. Repo style prefers `t.Context()`; a failing restore inside the deferred call reports against the test but after the body's state is ambiguous. Prefer `t.Cleanup` with error capture.

## Checked, not an issue
- T1: assertions pin exact versions, rebind counts, lifecycle statuses, terminal reasons, decision versions, and validation statuses; `TestRebindOnlyMutatesSnapshotFields` checks mutation in both directions (no non-snapshot key changed, no key added) — no tautologies.
- T2: fully deterministic — fixed ISO timestamps, no sleeps; the "race" test (M7) reproduces the race by ordering writes, not by timing; `recordingExecutor` copies the request to avoid data races.
- T3: helpers take `t.Helper()`, fixtures are per-test TempDir DBs, subtests would be independent.
- T4: covers re-bind dispatch, mutation isolation, budget exhaustion, corrupt-live-snapshot fail-closed with queue drain, and the end-to-end M7 sequence — real edge cases, not padding.
