# Testing Context

## Authoritative Commands

Use these commands unless the task is documentation-only:

- `go test ./...`
- `go vet ./...`
- `make build`
- `make lint`
- `make ci-check`
- `git diff --check`
- `pre-commit run --all-files` if `pre-commit` is installed

## Template-Specific Behavior

- `make build` prints a skip message when `cmd/` does not exist.
- `make ci-check` runs `tidy -> build -> vet -> lint-ci -> test-short -> deadcode -> vulncheck`.
- In restricted environments, `golangci-lint` can fail because it writes outside the workspace cache.
- `deadcode` and `govulncheck` are optional locally when the tools are missing; the Makefile reports that explicitly.

## Test Quality Bar

For production-code changes:

- write a failing or expectation-setting test before implementation when feasible
- add or update `*_test.go` files in every modified package
- cover new exported behavior and meaningful branches
- use table-driven tests where that improves clarity
- use `t.Helper()` in test helpers
- use `t.Context()` for test contexts when appropriate
- avoid network calls in unit tests

If test-first is not feasible:

- say why explicitly in the plan or handoff
- still add the proving test in the same change
- make sure the test demonstrates the behavior change, not just execution

For documentation-only changes:

- run the narrowest useful checks
- still run `git diff --check`
- explain skipped commands in the final handoff

## Failure Handling

- Fix failures caused by your changes before stopping.
- If a command fails for an existing repo issue or environment restriction, say so clearly and do not misreport it as validated.
