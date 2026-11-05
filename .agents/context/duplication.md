# Duplication Scan

Duplicated code is a defect to fix in the change that finds it, not to leave
behind or to note for later. Run this scan on every file you add or change,
before handoff, and repeat it after fixing, until nothing is left. This stays a
required step until the repository is clean at the current threshold and the
review checklist (Q8) has stopped finding duplicates.

1. **Token clones.** `make lint` runs `dupl` at the threshold in
   `.golangci.yml`. For a stricter look at your files, run it at a lower
   threshold: copy `.golangci.yml`, set `linters.settings.dupl.threshold` to 60,
   and run `golangci-lint run --config <copy> ./...`; fix what falls in files
   you touched.
2. **Same job, different code.** Before adding a helper, search the repo for the
   job it does (`grep -rn` for the verb and for `func` names such as `nullable`,
   `orPhysical`, `collect`, `format`), and use what exists; the known helpers
   are listed under Go Standards in [AGENTS.md](../../AGENTS.md).
3. **Wrappers.** For every new function whose body is one call into another
   module, ask whether the caller can call the target directly. Keep a wrapper
   only when an architecture gate requires it (a facade delegating to `app`, an
   opaque `store.Tx` method) or it adds something: a fence, a transaction,
   context for the error. Two modules defining the same operation over a third
   module's data (the old `control` interlock copy) is the case to catch.
4. **Same query twice.** Search for the table name in `internal/**/store`; one
   `SELECT` of a row belongs in one place.
5. **Identical bodies.** Find functions whose bodies print the same with a
   throwaway `go/ast` program (group `FuncDecl` bodies by `printer.Fprint`
   output, report groups of 2 or more with at least 4 lines). Per-module
   private types that only look alike (each module's opaque `Join`, each
   module's `Tx`) are not duplicates; helpers with the same body are.

Fix with the tools in [go-style.md](go-style.md), then run `make lint`. The
`dupl` gate covers test files too; repeated test setup moves into a helper
(`storagetest.OpenTemp` opens a migrated temporary database).

## Known duplication to burn down

At threshold 50 (the scan above, with 50 in place of 60) these `dupl` pairs
remain. Each is a structural twin that a shared helper would not shorten, or
whose only shared step would have to return another package's error bare, which
the wrapping rule forbids:

- `cmd/agentic-stream` operator commands: `notifications prune`/`principals apply`
  (flag registration differs per command) and `awaitingTable`/`quarantineTable`
  (a `tableText` call with a per-row format).
- `internal/actions/internal/store`: `scanCommandView`/`scanAwaitingCommand` (distinct
  row types and columns).
- `internal/canonicaljson/internal/domain/digest.go`: `signedInteger`/`unsignedInteger`
  (one type switch per integer family; a type switch cannot span two type sets).
- `internal/episodes/internal/store/assembler.go`: `LoadSchedulerItem`/`LoadEvaluation`
  (`loadRow` already holds the shared step; each call carries its own SQL and columns).
- `internal/episodeledger/internal/store` (`episode_reads.go`/`scheduling_reads.go`) and
  `internal/replay/internal/store/store.go`: a `storage.QueryAll` call followed by its one
  operation-naming wrap; sharing it means returning `storage.QueryAll`'s error bare.
- `internal/notify/internal/domain/lifecycle_identity.go`: the eight lifecycle event
  constructors differ only in the event identity formula; the public facade keeps one
  constructor per payload.
- `internal/policy/internal/domain/rules.go`: `MatchesDecisionIdentity`/`MatchesIntentIdentity`
  compare different fields.
- `internal/policy/internal/store/approval_lookup.go`: `PendingApproval`/`ApprovedApproval`
  each wrap their own `approvalledger` lookup.
- `internal/runtime/internal/app/pipeline_run.go`: `RunLiveSocket`/`RunFeed` pass different
  source callbacks to `runLiveSource`.

Remove an item from this list when you fix it, add one only for a duplicate you
could not fix in the same change and say why in the change.
