# Architecture and maintainability audit — 2026-10-05

Baseline: `6750c10` on branch `general-improvements-1`.

Scope: architecture, modularity, and how easy it is to maintain the code and add
features. This audit does not repeat the per-file findings of the
[2026-09-11 full repository audit](../2026-09-11-full-repo-audit/INDEX.md). It
looks at patterns that repeat across packages.

## What already works

- Imports go one way only, and tests pin them (`architecture_flow_test.go`).
- Each table has one owner that writes to it (`architecture_ownership_test.go`).
- Functions are small (at most 15 lines), and lint enforces the limit.
- Domain data is kept in JSON files, not in Go code.
- Every package has a package comment.

The ten items below cover what these rules do not catch yet: how dependencies
are wired, how the spec is executed, and how a new feature is added.

## The ten improvements

| # | Improvement | Main benefit | Effort | Risk |
| --- | --- | --- | --- | --- |
| 1 | [Make safety dependencies required (fail closed)](01-fail-closed-safety-dependencies.md) | Correctness, safety | M | Low |
| 2 | [One ownership fence and one fenced transaction](02-ownership-fence-primitive.md) | Less duplication, safety | M | Low |
| 3 | [Compile the spec once into an executable plan](03-compile-spec-once.md) | Correctness, speed, one source of truth | L | Medium |
| 4 | [Register operator kinds in one place](04-operator-kind-registry.md) | Easier new operators | M | Low |
| 5 | [Turn effect routes into data](05-effect-routes-as-data.md) | Easier new devices and actions | M | Medium |
| 6 | [Use typed records instead of `map[string]any`](06-typed-domain-records.md) | Readability, compile-time checks | L | Medium |
| 7 | [Build live, serve, and replay through one assembly](07-single-runtime-assembly.md) | Replay matches live, simpler `cmd` | L | Medium |
| 8 | [Add a shared test kit](08-shared-test-kit.md) | Faster, consistent tests | M | Low |
| 9 | [Clean up the repository and name tests by behavior](09-repository-hygiene.md) | Smaller clones, clearer intent | S | Low |
| 10 | [Keep agent and design docs in sync with the code](10-documentation-drift.md) | Fewer wrong assumptions | S | Low |

Effort: S under 1 day, M 1–3 days, L more than 3 days.

## Recommended order

1. **Items 1 and 2 together.** They touch the same constructors and remove the
   main safety risk: optional safety checks that are silently skipped when
   nothing is wired.
2. **Item 9.** It is quick: remove the 40 MB binary from git before more history
   builds up on top of it.
3. **Item 8.** The later refactors are easier with a shared test kit.
4. **Items 3 and 4.** Spec compilation is the base for the operator registry.
5. **Items 5 and 7.** They change how the runtime is composed. Each needs a
   short design note first, as `AGENTS.md` requires.
6. **Item 6**, one aggregate at a time, starting with the reconsideration
   documents.
7. **Item 10**, as an ongoing gate.

## Constraints for every item

- Behavior stays the same. Golden replay digests, the intent catalog digest
  (`e4f86620…`), and the registry digest do not change unless the change is a
  deliberate, reviewed data change.
- Each item must respect the ten product invariants in
  [docs/design/README.md](../../design/README.md) and the
  [architecture bar](../../../.agents/context/architecture-bar.md).
- Items 3, 5, and 7 change how components are composed. Record each one as an
  ADR in [DECISIONS.md](../../design/DECISIONS.md) before writing code.
- `make ci-check` must pass after each item. Never loosen a threshold to make a
  diff pass.
