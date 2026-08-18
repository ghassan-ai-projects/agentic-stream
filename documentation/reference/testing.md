# Testing reference

Audience: contributors and maintainers. Scope: repository targets, evidence
categories, and the minimum checks for documentation-only versus code changes.

## Standard targets

| Target | Purpose |
| --- | --- |
| `make build` | compile `cmd/...` into `bin/agentic-stream` |
| `go test ./...` | all package tests |
| `make test` | race, shuffle, one run, coverage profile |
| `make test-short` | short race-enabled suite used by CI |
| `make test-race` | race-enabled tests without shuffle/coverage wrapper |
| `make test-coverage` | HTML coverage report |
| `go vet ./...` | static analysis |
| `make lint` | golangci-lint |
| `make proto-check` | generated worker stubs match source |
| `make ci-check` | local CI-equivalent gate |
| `make docs-check` | public docs structure/surface checks |
| `git diff --check` | whitespace/error check |

Optional local tools such as `deadcode` and `govulncheck` are reported as
skipped by the Makefile when not installed.

## Evidence categories

- Unit tests prove local package rules.
- Integration tests prove sockets, storage, worker, notification, and telemetry
  boundaries.
- Pipeline E2E tests prove event-to-effect composition.
- Replay tests prove deterministic/no-effect behavior.
- Conformance tests prove executor/worker protocol behavior.

Green tests do not replace deployment qualification, backup/restore rehearsal,
or external effector review.

## Documentation changes

Documentation-only changes must run `make docs-check` and `git diff --check` at
minimum. Run the broader suite when examples, contract references, Makefile,
CI, or runtime commands change.

## Next reads

- [Quality](../governance/quality.md)
- [Release evidence](../governance/release.md)
- [Contributing](../../CONTRIBUTING.md)
