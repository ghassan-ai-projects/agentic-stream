# A-080 · `internal/storage/storage_test.go`

LOC: 357 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T1: no assertions that can never fail; idempotence is proven by data survival, not metadata heuristics.
- T5: repeated fixture SQL exists once as a helper.

## Findings
- **[HIGH] F1. Idempotence assertion is a de-facto tautology** — `internal/storage/storage_test.go:350-356`. `TestOpenIsIdempotent` claims "database was recreated on second open" iff `stat2.ModTime().Before(stat.ModTime())`. A recreated file always has a later-or-equal mtime, so `Before()` is false and the test passes even if `Open` deleted and recreated the database (wiping all data). The only observable it checks is one that cannot regress in the direction that matters. Fix: insert a durable row (or rely on migration state) before closing db1, reopen, and assert the row/migration version survived; or compare file content hashes.
- **[MED] F2. 217-line test with copy-pasted fixture SQL** — `internal/storage/storage_test.go:107-323`. `TestLifecycleMigrationMapsEveryFormerEpisodeStatus` inlines ~120 lines of inserts; the situation+situation_versions insert pair appears twice nearly verbatim (lines 158-175 for `sit-legacy` and lines 180-198 inside the per-status loop). Extract a `insertLegacySituationWithVersion(t, db, id, digest)` helper; the loop body then reads as the status matrix it actually tests.
- **[LOW] F3. No shared DB-open helper while siblings have one** — `internal/storage/storage_test.go:23,254,330,343`. Three inline `storage.Open(ctx, filepath.Join(t.TempDir(), …))` + close-handling blocks; `runtime_owner_test.go:99` already defines `openOwnerDB` doing the same job. Consolidate into one `openStorageTestDB(t)` in the package.

## Checked, not an issue
- T1: migration test asserts the full status→lifecycle mapping row by row, decision/intent survival, and schema column presence — behavioral, with per-row messages.
- T2: deterministic — temp dirs, no sleeps, no network, no goroutines; `time.Now()` only fills migration bookkeeping columns in the fixture.
- T3: `t.Run` per pooled connection checks connection-local PRAGMAs independently; table-driven where natural.
- T4: covers migration version parity, WAL/busy_timeout/synchronous/autocheckpoint PRAGMAs on multiple pooled connections, 13 legacy lifecycle statuses, and per-connection pragma propagation — real edge coverage, not padding.
- T5: the `former`/`want` maps keep the status matrix in one place instead of one test per status.
