# Worker Brief

One worker takes one round: the modules named in the [tracker](README.md#tracker).
It brings every test in those modules to the [test bar](TEST_BAR.md), records
what it found in `modules/<module>.md`, and leaves the change uncommitted for
the orchestrator to review and commit.

## Rules

- Read [AGENTS.md](../../AGENTS.md), [TEST_BAR.md](TEST_BAR.md) and
  [.agents/context/testing.md](../../.agents/context/testing.md) first.
- Touch only the round's modules and their report files. Other workers may be
  editing other modules in the same worktree: do not format, move or revert
  files outside the round, and never run `git checkout`, `git stash`,
  `git reset` or `git commit`.
- Production code does not change behavior. Change it only when a test cannot
  reach the behavior otherwise (inject a clock, delete a test-only export), and
  list every such change in the report.
- Do not weaken a test to make it pass, lower a threshold, skip an acceptance
  test under `-short`, or exclude a package from coverage.
- Change Go code with Go tooling (`gopls rename`, `gofmt -r`, `goimports`, a
  `go/ast` program kept in the scratchpad). Move files with `git mv`.
- A test that fails before your change is a finding: report it, do not hide it.

## Steps

1. **Inventory.** For each package: test files, test names, the production
   file each one proves; coverage (`go test -short -race -count=1 -cover`);
   timing (`go test -short -race -count=1 -json`, slowest tests); test-hygiene
   lint findings (TEST_BAR.md); `time.Sleep` uses.
2. **Assess** every test against T1–T12 and list, per package:
   - *Missing*: behavior that matters and has no test (uncovered facade
     operations, invariant-enforcing error branches, boundaries).
   - *Improve*: tests that break a rule, with the rule ID.
   - *Remove*: dead, redundant or assertion-free tests, with the reason.
   - *Speed*: what makes slow tests slow and the fix.
3. **Fix**, in this order: remove and merge; rename files and tests; make
   tests parallel and deterministic; share setup; add missing tests; speed up.
4. **Verify**, for the round's modules:
   - `gofmt -l`, `go build ./...`, `go vet ./internal/<module>/...`
   - `go test -race -shuffle=on -count=3 ./internal/<module>/...`
   - `golangci-lint run ./internal/<module>/...` (the repository config: 0 issues)
   - the test-hygiene lint from TEST_BAR.md: 0 issues
   - `go test -count=1 ./internal/architecture/...` (the repository gates)
   - coverage and timing again, for the report
5. **Report** in `modules/<module>.md` using the template below, and return a
   short summary: metrics before and after, production files changed, open
   items.

## Report template

```markdown
# <module>

Status: done | partial (say what is left)
Round: <n>

## Metrics

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |

## Findings and changes

### Removed
- `TestX` (path): reason.

### Renamed or moved
- old → new: reason.

### Improved
- `TestY`: rule, what changed.

### Added
- `TestZ`: the behavior it proves.

### Speed
- what was slow, why, what changed.

## Production code touched
- none | path: why.

## Invariants proven here
- invariant number: test names.

## Open items
- anything not fixed and why.
```
