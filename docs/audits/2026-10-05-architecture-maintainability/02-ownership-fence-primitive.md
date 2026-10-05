# 2. One ownership fence and one fenced transaction

## Problem

Runtime ownership is checked by hand in many places. Each component stores its
own `owner *control.RuntimeOwner` and `ownerEpoch string`. Each one opens a
transaction, calls `owner.Assert(ctx, tx, epoch)`, wraps the error in its own
way, and decides on its own whether a missing owner means "skip".

## Evidence

- Nine packages have `owner`/`ownerEpoch` fields: `admission`, `episodes`,
  `runtime`, `watch`, `engine`, `episodeledger` (3 files), and `policy`.
- `owner.Assert` is called in 11 places with slightly different code:
  `evidence/ledger.go:156`, `admission/admission.go:161`,
  `runtime/pipeline.go:191`, `runtime/pipeline_cost.go:33`,
  `actions/dispatcher.go:179`, `watch/effector.go:206`, `watch/expire.go:25`,
  `authority/authority.go:167`, `engine/engine_apply.go:186`,
  `policy/policy_store.go:19`, `control/runtime_owner.go:160`.
- Constructors grow parameters to carry the pair, for example
  `NewGatewayWithOwner(digest, ids, owner, epoch)` and
  `NewRunnerWithEpoch(db, exec, clk, ids, epoch)`.

## Why it matters

The fence protects the "one runtime owner" invariant, so every new write path
must remember to add it. Each copy is another chance to get it wrong, for
example by asserting after the first write instead of before it, or by wrapping
the error so that callers cannot detect it with `errors.Is`.

## Recommendation

1. Add a value type in `internal/control`:

   ```go
   // Fence identifies the runtime owner epoch that every durable write must hold.
   type Fence struct{ owner *RuntimeOwner; epoch string }

   func (f Fence) Assert(ctx context.Context, tx *sql.Tx) error
   func (f Fence) Epoch() string
   ```

2. Add one helper for fenced transactions, for example
   `control.WithFencedTx(ctx, db, fence, fn)`. It begins the transaction, asserts
   the fence first, runs `fn`, and commits. Writers call this helper instead of
   `db.WithTx` plus a manual assert.
3. Replace the owner and epoch pair in every constructor with a single `Fence`.
   `control.Unfenced()` is the only way to opt out (see
   [item 1](01-fail-closed-safety-dependencies.md)).
4. Extend `architecture_ownership_test.go` so that production SQL mutations in
   the owner-fenced tables are only allowed inside functions that receive a
   fenced transaction. A first version can be a lint-style AST check.

## Done when

- `owner.Assert` is called in exactly one place, `Fence.Assert`.
- No constructor takes the owner and epoch as two separate parameters.
- One shared sentinel error reports a lost fence, and tests check it with
  `errors.Is`.
