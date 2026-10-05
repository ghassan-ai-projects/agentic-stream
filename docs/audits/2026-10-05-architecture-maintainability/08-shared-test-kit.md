# 8. Add a shared test kit

## Problem

67 test files call `storage.Open` directly, and each package writes its own
setup helpers: `openOwnerDB` (4 copies), `openLedgerDB`, `openActionDB`,
`openSoakDB`, `newCostDB`, and more. `internal/storage/fixture_test.go` cannot
be reused because it is a `_test.go` file. Every package therefore rebuilds the
same set of things: a migrated database, a fixed clock, deterministic IDs, a
runtime owner and epoch, a compiled example spec, and seeded intents or
commands.

## Why it matters

- Each refactor in items 1, 2, and 7 changes constructor signatures. Without a
  shared kit, every package's copy of the setup has to be updated by hand.
- Tests can drift from production wiring, for example by forgetting the
  interlock ([item 1](01-fail-closed-safety-dependencies.md)), and still pass.
- Fixture code adds noise to test files, which makes the behavior under test
  harder to find.

## Recommendation

1. Create `internal/testkit`, a non-test package that only `_test.go` files
   import. Add an architecture rule: production code may not import `testkit`.
2. Give it small helpers that are safe to compose:
   - `testkit.DB(t)` returns a migrated SQLite database in `t.TempDir()`, closed
     with `t.Cleanup`.
   - `testkit.Clock(t, at)` and `testkit.IDs(t)` return a virtual clock and
     deterministic IDs.
   - `testkit.Fence(t, db)` starts an owner and returns a `control.Fence`
     ([item 2](02-ownership-fence-primitive.md)).
   - `testkit.Spec(t, name)` returns a compiled example spec.
   - Builders such as `testkit.Intent(...)` and `testkit.Command(...)` seed rows
     through the **owning package's** API, not with raw SQL. This keeps the
     single-writer rule true in tests too.
3. Add a scenario harness for end-to-end tests: `testkit.Pipeline(t,
   opts...)`. It calls the same `runtime.Open` as production
   ([item 7](07-single-runtime-assembly.md)), with simulated effects.
4. Migrate one package at a time, starting with the four `openOwnerDB`
   copies.

## Done when

- No package defines its own "open migrated DB" helper.
- End-to-end tests build the pipeline through the same assembly as
  production.
- The test suite takes the same time or less (`make test`).
