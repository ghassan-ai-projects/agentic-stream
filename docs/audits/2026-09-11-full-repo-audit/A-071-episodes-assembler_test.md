# A-071 · `internal/episodes/assembler_test.go`

LOC: 1140 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Spec/engine/version/scheduler-item setup exists once and is parameterized, not repeated per test.
- The tenant-mismatch test asserts a distinguishable tenant error, not merely `err != nil`.
- The determinism test compares the full request identity, not only the snapshot hash.
- No seeding SQL block appears twice in the file.

## Findings
- **[MED] F1. Six near-identical ~70-line setup blocks** — `internal/episodes/assembler_test.go:95-169,498-570,749-821,861-927,960-1026,1049-1115`. Each repeats storage.Open + `spec.CompiledSpec` literal + `SaveDeployment` + `NewEngine` + version seed + scheduler-item lookup; only executor/intents differ. Extract one fixture helper returning `(db, schedulerItemID)`; cuts ~350 LOC.
- **[MED] F2. Raw seeding SQL duplicated between two tests** — `internal/episodes/assembler_test.go:358-439` and `:655-704`. The `trigger_evaluations` + `scheduler_items` + `episodes` insert blocks are copy-pasted with only values changed. Extract `seedSchedulerItemWithEpisode(t, tx, ...)` variants.
- **[MED] F3. Tenant-mismatch test accepts any error** — `internal/episodes/assembler_test.go:1029-1037`. `Assemble` for tenant `other-tenant` only checks `err == nil`; a broken seed making the item unloadable, or any unrelated error, passes the test, and a future change that rejects with the wrong semantic is indistinguishable. Assert a specific sentinel/classified error (as `TestAssemblerMarksReconsiderationLiveEpisodeConflict:728` does with `ErrLiveEpisodeConflict`).
- **[LOW] F4. Determinism test under-asserts its name** — `internal/episodes/assembler_test.go:847-849`. `TestAssemblerIsDeterministic` compares only `SnapshotSHA256` across two `Assemble` calls; with `ids.Deterministic()` the `EpisodeID`, `AdmissionKey`, and `RequestJSON` are equally comparable. Compare the full request to actually pin assembler determinism.
- **[LOW] F5. WatchConfidenceFloor subtests mutate the parent fixture** — `internal/episodes/assembler_test.go:223-251`. Subtests write `compiled.Actions.WatchConfidenceFloor` through the pointer the assembler captured; they are order-coupled to the parent's default assertion and not `t.Parallel()`-safe. Build a fresh assembler per subtest from a copied spec.
- **[LOW] F6. `ticketSchema` duplicated across the directory's two test packages** — `internal/episodes/assembler_test.go:1138-1140` vs `internal/episodes/decision_input_catalog_test.go:12-14` (identical literal, packages `episodes_test` vs `episodes`). One shared testdata helper would prevent drift.
- **[LOW] F7. Inconsistent context usage** — `t.Context()` appears only at `:618`; all other tests use `context.Background()`.

## Checked, not an issue
- T1: request payload, persisted episode rows, provenance digest lengths, conflict error identity, and non-pending rejection are all asserted with diagnostics; the reconsideration payload check (prior decision/commands/outcomes/invalidates) is exact.
- T2: deterministic — fixed timestamps where timing matters, no sleeps/network; `db.SetMaxOpenConns(1)` avoids SQLite contention.
- T3: no t.Run structure needed except F5's; subtests otherwise independent.
- T4: error paths covered (live-episode conflict, non-pending persist, tenant mismatch); reconsider assembly and persist round-trip covered end-to-end.
