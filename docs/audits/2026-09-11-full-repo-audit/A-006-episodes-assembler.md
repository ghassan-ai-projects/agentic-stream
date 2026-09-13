# A-006 · `internal/episodes/assembler.go`

LOC: 764 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every struct field is read after construction.
- No always-true predicate or filtering logic that cannot filter.
- Constraint classification does not depend on matching a driver-formatted error string.
- `RequestJSON` budget fields are decoded in one shared, validated place.

## Findings
- **MED F1. `snapshotEvidence.json` field is written and never read** — `internal/episodes/assembler.go:417-424, 458-461`. `loadValidatedSnapshot` stores the raw snapshot bytes in the `json` field, but neither `Assemble` (uses `document`/`entityID`/`digest`/`traceparent`/`tracestate`) nor `Rebind` ever reads it; `snapshotEntityID(snapshotJSON)` re-derives the entity from the local variable instead. Grep across the repo confirms zero reads. Dead state in the type that binds episodes to snapshots. Fix: drop the field and pass `snapshotJSON` locally.
- **MED F2. `allowedIntentTypes`/`allowedIntentTypeList` filtering is always true** — `internal/episodes/assembler.go:476-498`. `allowedIntentTypes` builds the set from `a.spec.Actions.Intents`, and `allowedIntentTypeList` iterates the same slice checking membership in that set — the `allowed[intent.Type]` test can never fail, so the helper pair only performs deduplication (and duplicates are already a hard `CompileIntentCatalog` error at 216-219). Speculative abstraction that suggests a filter that does not exist. Fix: replace both with a single dedup loop over `a.spec.Actions.Intents`.
- **LOW F3. Live-episode conflict detected by string-matching the driver's error text** — `internal/episodes/assembler.go:371-377`. `isLiveEpisodeConstraint` requires `strings.Contains(err.Error(), "UNIQUE constraint failed: episodes.situation_id")`. Verified working against SQLite's current column-format message for the partial index (`migrations/003_lifecycle_fencing.sql:101-103`), but the message format is not a driver contract; if modernc.org/sqlite ever reports the index name, every reconsider conflict degrades to a generic admission failure. Fix: match on the index name, or pre-check liveness with a SELECT inside the tx and map the INSERT failure positionally.
- **LOW F4. Budget decode duplicated in `Persist`** — `internal/episodes/assembler.go:322-331`. `cost_microunits` is decoded here from `RequestJSON` with its own error semantics, one of four independent partial decodes of the same budget document (see A-003 F3). Fix: shared typed budget accessor on `Request`.

## Checked, not an issue
- P2: `Assemble` and `Rebind` share `loadValidatedSnapshot` — schema validation, situation/tenant/version identity, entity extraction, and persisted-digest equality are enforced before any snapshot reaches a request; `Rebind` preserves entity identity and rejects entity drift (393-395).
- P2: empty `DispatchPolicy` defaults to shadow at the only write site (302-309); nothing enters governance unless the spec declared active.
- P4: no per-domain branches in Go; dependency direction is downward (canonicaljson, contractsv1, costcontrol, ids, spec).
- P5: exported symbols documented; canonical JSON digests for prompt/objective/catalog/snapshot; errors wrapped with `%w`.
- P6: assembler tests (40KB) plus rebind tests cover admission, reconsideration evidence, digest mismatch, and re-bind entity drift.
