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
- `make ci-check` runs `proto-check -> tidy -> build -> vet -> lint-ci -> coverage-check -> deadcode -> vulncheck -> docs-check`. `coverage-check` runs the short race suite once and enforces the per-package coverage floor.
- In restricted environments, `golangci-lint` can fail because it writes outside the workspace cache.
- `make deadcode` (`scripts/check-deadcode.sh`) runs `deadcode ./...` without `-test` and fails when a production function is reachable only from tests. Test-support packages are exempt: `internal/testsupport/...` and packages whose name ends in `test` (`controltest`, `spectest`, ...). Fix a finding by wiring the function into production, deleting it, or moving it to test support (an `export_test.go` for a package's own tests).
- `deadcode` and `govulncheck` are optional locally when the tools are missing; the Makefile reports that explicitly. CI installs the pinned versions, so the gates always run there.

## Database Tests

Open runtime databases in tests with `storagetest.Open` (`internal/storage/storagetest`), not `storage.Open`. Replaying every migration costs about a second per database under `-race`; `storagetest.Open` copies a template migrated once per migration set. Use `storage.Open` only where the test is about opening or migrating itself (`internal/storage/**`). Tests that call `t.Setenv` or `t.Chdir` cannot be parallel; the experiment end-to-end tests in `cmd/agentic-stream` set their process environment once instead so they can run together.

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
