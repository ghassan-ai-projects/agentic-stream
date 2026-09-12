# A-062 · `internal/storage/epoch_control.go`

LOC: 149 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Kill is atomic: the epoch record and the supersession of its in-flight episodes commit together or not at all.
- Misconfiguration fails closed on the decision path.
- Kill is terminal and can never be downgraded to draining.
- Time sources are injectable like the sibling lease types.

## Findings
- **[MED] F1. Kill performs two independent writes without a transaction** — `internal/storage/epoch_control.go:35-49`. `set(killed)` and the `UPDATE episodes ... lifecycle_status = 'superseded'` are separate autocommit statements. A crash between them leaves the epoch killed while its episodes stay `admitted`/`running`: `watchSupersession` never cancels the provider calls (the documented mechanism), and the `one_live_episode_per_situation` unique index (migration 003) blocks any new episode for those situations until the next process restart runs attempt recovery. Wrap both statements in `db.WithTx`; nothing else changes.
- **[MED] F2. State/AssertDecision fail open on misconfiguration** — `internal/storage/epoch_control.go:59-73` and `internal/storage/epoch_control.go:78-87`. A nil receiver, nil `DB`, or empty epoch returns `("", nil)`, so `AssertDecision` allows the decision. `AssertOrdinaryTx` on the same type fails closed with "epoch control is not configured" (`internal/storage/epoch_control.go:93-95`). The decision-boundary gate — the safety-critical one — is the path that fails open. Return an error for nil/empty like `AssertOrdinaryTx`.
- **[LOW] F3. Wall-clock time not injectable** — `internal/storage/epoch_control.go:42` and `internal/storage/epoch_control.go:132`. `Kill` and `set` call `time.Now()` directly while `RuntimeOwner`/`TargetAuthority` take an injectable `Now`; tests of kill-time supersession timestamps cannot be made deterministic.
- **[LOW] F4. set() composes SQL via fmt.Sprintf branches** — `internal/storage/epoch_control.go:135-143`. The killed/draining `ON CONFLICT` WHERE clauses are near-identical string templates differing by `state IS NULL OR`; two plain statement constants would remove the branch and the sprintf entirely (input is not injectable today, but the pattern invites it).

## Checked, not an issue
- P1: statements that are single writes (`set`, `State`, asserts) are atomic; errors wrapped `%w`; `ErrEpochKilled`/`ErrEpochDraining` are `errors.Is`-compatible and consumed in `internal/episodes/executor.go`, `internal/policy/policy_evaluate.go`, `internal/runtime/pipeline.go`, and `storage.assertOrdinaryTx`.
- P2: kill terminality is enforced (`WHERE epoch_control.state <> 'killed'`); decision gate checks the episode's RECORDED epoch, so a hostile worker cannot resurrect outcomes.
- P4: matches `migrations/025_epoch_control.sql` (CHECK-in states, STRICT); `lifecycle_status` values written by Kill match the 003 CHECK.
- P6: epoch behavior is exercised indirectly via `authority_test.go` (draining/killed refusal through `assertOrdinaryTx`) and policy/episode suites; direct `Kill` supersession has no dedicated test — add one when F1 is fixed.

## Resolution (2026-09-12) — FIXED

- **F1** fixed: `Kill` records the killed epoch and supersedes admitted/running episodes in one transaction. `TestEpochControlKillIsAtomicWithEpisodeSupersession` asserts the shared timestamp and terminal state, and validates the killed decision gate on the same transaction.
- **F2** fixed: nil, unconfigured, and empty-epoch `State`, `AssertDecision`, and transaction-scoped decision checks return an error, so the safety boundary fails closed.
- **F3** fixed: `EpochControl.Now` is injectable, with UTC normalization and a physical-clock fallback.
- **F4** fixed: killed and draining upserts are separate static SQL statements; runtime SQL composition was removed.
- Verified: `go test -race ./internal/storage` passes.
