# Quality and Modularity Bar

Every Go change must keep the repository at this bar. Each rule is enforced by a
named check in `make ci-check`, so the bar is reviewable and cannot drift into
opinion. Tightening a threshold is welcome; loosening one is a reviewed decision
recorded in the same change, never a way to get a diff green.

| ID | Rule | Enforced by |
| --- | --- | --- |
| Q1 | `golangci-lint` reports 0 issues across the whole tree, with no "new issues only" baseline. | `make lint-ci` (`.golangci.yml`) |
| Q2 | Production functions stay small and flat: cognitive complexity ≤ 30, cyclomatic complexity ≤ 20, nested-`if` complexity ≤ 5, at most 100 lines and 60 statements. | `gocognit`, `gocyclo`, `nestif`, `funlen` in `.golangci.yml` |
| Q3 | Production Go files stay at most 800 lines. Generated files are exempt. | `TestProductionFileSize` (`architecture_test.go`) |
| Q4 | Every package with statements has its own tests and at least 60% statement coverage, measured with `-short`. Generated protobuf stubs are exempt. | `make coverage-check` (`scripts/check-coverage.py`) |
| Q5 | Package imports follow the declared layering. Foundation packages import no domain package; cognition and episodes never import policy or actions; replay never imports actions or the runtime; nothing under `internal/` imports `cmd/`. | `TestPackageLayering` (`architecture_test.go`) |
| Q6 | The existing gates stay green: proto, tidy, build, vet, race tests, deadcode, vulncheck, docs. | `make ci-check` |

## Rules for meeting the bar

- Refactors made to meet Q2 and Q3 must not change behavior. The golden replay
  and predictive-maintenance suites are the proof, so run them unchanged.
- Split by responsibility: extract a named step or a type that owns one concern.
  Do not split a function mechanically into `partA` and `partB`.
- Do not use `//nolint` for `gocognit`, `gocyclo`, `nestif`, or `funlen`. Fix
  the code instead. Any other `//nolint` names its linter and gives a reason.
- Do not delete or weaken tests, and do not exclude packages, to meet Q4.
- A new package edge must follow the data flow in
  [architecture.md](architecture.md). Add it to the allowlist in
  `architecture_test.go` in the same change, with the reason in review.
