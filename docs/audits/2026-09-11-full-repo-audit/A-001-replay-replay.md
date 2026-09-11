# A-001 · `internal/replay/replay.go`

LOC: 1003 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- No unused exported symbols or dead code paths (P3) — FAIL: entire Mode/Shadow/Recorded/Counterfactual machinery is unreachable from production.
- Replay admission logic is not a divergent duplicate of the production pipeline (P3/P7) — FAIL.
- No dead helpers (P3) — FAIL: `validateDigest` has zero callers repo-wide.
- Schema-guard logic is not duplicated across packages (P3/P5) — FAIL.
- Hash framing is unambiguous (P5/P7) — FAIL.
- Errors wrapped %w, none swallowed (P1) — FAIL: one discarded digest error.
- Contexts honored, no data races (P1) — pass.
- Replay performs no external effects; untrusted ledger/trace input validated before use; effectors unreachable (P2) — pass.
- Package boundaries per design §23, canonical JSON for domain digests (P4/P5) — pass.

## Findings
- **MED F1. Mode/shadow/counterfactual machinery is dead production code** — `internal/replay/replay.go:33-302,551-617,619-874`. `RunMode`, `Capabilities`, `ShadowExecutor`, `BaselineExecutor`, `Simulator`, `RecordedLedger(ForReplay)`, `ShadowInput/Output`, `SimulatedCommand`, `applyPairedShadow`, `applyCapabilities`, `materializeReplayEpisodes`, `loadReplayItems`, `loadShadowInput`, `validateRecordedDecision`, and `replayEpisodeKey` have zero callers outside this package (grep confirmed; `internal/` cannot be imported externally, and `cmd/agentic-stream/main.go:621` calls only `replay.Run`). The production shadow path lives in `internal/runtime` (`pipeline.go:141` `WithShadowStore`), so this is a parallel, test-only second implementation of shadow mode (~450 LOC). `cognitionEnabled=true` in `run` (line 207) is reachable only via `RunMode`, so episode materialization is dead too. Fix: wire the modes into the CLI (`replay --mode …`) or delete the machinery until a caller exists; do not keep two shadow implementations.
- **MED F2. Divergent duplicate of production episode admission** — `internal/replay/replay.go:582-598,608` vs `internal/runtime/pipeline.go:468-473,496`. Replay admits `admitAt = max(created, not_before)`, skips `expires <= admitAt`, orders by `scheduler_item_id`, and persists with `admitAt`; production admits `not_before <= now` ordered by `(not_before, created_at, scheduler_item_id)` and persists `now`. Over the same evidence, replay can materialize a different episode set and different episode timestamps than a live run — a fidelity gap against the golden-replay correctness contract. Fix: extract one shared admission predicate/ordered query used by both call sites.
- **MED F3. Dead helper `validateDigest`** — `internal/replay/replay.go:865-870`. Zero callers repo-wide (grep confirmed). Delete.
- **LOW F4. Duplicated schema-guard logic and double deployment save** — `internal/replay/replay.go:462-488` duplicates `engine.configureSchemaValidation` (`internal/engine/engine_construction.go:49-59`) and `spec.SaveDeployment` runs twice per replay (line 462 and `engine_construction.go:32`). The two copies of the "all inputs have SchemaRef" predicate can silently diverge. Extract one spec helper and reuse it.
- **LOW F5. Ambiguous aggregate hash framing** — `internal/replay/replay.go:958-961`. `fmt.Fprintf(h, "%s%d", situationID, version)` concatenates without a separator: `(id="a", v=12)` and `(id="a1", v=2)` produce the same bytes, so `VersionsHash` is not a faithful identity of the version set. Also not canonical JSON (P5). Use a length prefix or separator (e.g. `%s\x00%d`).
- **LOW F6. Underlying error discarded** — `internal/replay/replay.go:636-638`. `if err != nil || provenance != entry.AttemptProvenanceSHA256` drops the digest error from the returned message. Wrap it (`%w`) so the failure cause is diagnosable.
- **LOW F7. Minor dead weight and odd ordering** — `internal/replay/replay.go:197-198` re-assign zero values `WorkerInvoked`/`EffectsAllowed`; `:710-712` checks the empty-commands error after the loop that can never run; `runAllPartitions` (`:543-549`) is a single-caller wrapper around one `RunGlobal` call whose name promises partition fan-out it does not do. Inline/rename.

## Checked, not an issue
- P1: all other errors `%w`-wrapped with context; ctx threaded through every DB/file call; single-threaded, no data races.
- P2: no external effects anywhere; shadow persist is a local report row (tests assert zero intents/commands/outbox); untrusted ledger entries re-validated against schema+digest before trust; capability interfaces are effect-free by construction; effectors unreachable.
- P4: imports flow downward per design §23; no transport leakage; no per-domain branches.
- P5: exported symbols documented; canonical JSON used for all domain digests; no logging needed.
- "Tamoz" naming is design-sanctioned (`docs/plans/real-world-sensor-hil/`), not a domain leak.
- Determinism of the live `Run` path: fresh DB, virtual clock epoch derived before ingestion, serial `RunGlobal`, stable deployment digest — pass.
