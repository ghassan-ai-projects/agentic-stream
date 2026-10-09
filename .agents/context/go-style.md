# Go Style Context

## Core Rules

- Use `context.Context` as the first parameter for I/O, blocking, or cancellable work.
- Return `error` last.
- Wrap errors with `%w` and context the callee lacks (operation, identifier, layer). Never silence `wrapcheck` because the callee "already names the step"; see AGENTS.md for the two allowed bare returns.
- Use `log/slog` instead of `fmt.Println` for logging.
- Comment only the public interface of a module (exported symbols of the facade package, plus one package comment per package). No comments inside a module; make the code say it.
- Prefer standard library packages like `cmp`, `maps`, and `slices` over new helper dependencies.
- Keep packages small, lowercase, and singular.
- Prefer direct code over indirection when both are maintainable.
- Add new abstractions only after a real second use, repeated logic, or a clear testing need appears.

## Module Path and Imports

- The module path is declared exactly once, in `go.mod`. Every internal import
  names it in full (`github.com/ghassan-ai-projects/agentic-stream/internal/actions`)
  because Go package identity is the module-qualified import path. This is
  required, not a smell. Never use relative imports such as
  `./internal/actions`: module mode rejects them and no tool can resolve them.
- Do not repeat the module path as a new string literal. Ask the toolchain
  instead (`go list -m`) anywhere a command can be derived, as the `Makefile`
  does.
- The only permitted literal copies are ones a tool forces on us: the protobuf
  `go_package` option, the `goimports -local` prefix in
  `.pre-commit-config.yaml`, and the OpenTelemetry instrumentation scope
  constant. `TestModulePathSingleSourceOfTruth` (`internal/architecture/module_path_test.go`)
  pins each of those copies to `go.mod`, so a rename cannot drift.
- To rename the module: `go mod edit -module=<new>`, update the pinned literals
  above, regenerate the protobuf stubs with `make proto-generate` (never hand-edit
  `*.pb.go`), then run `go test ./...`.

## Layering Rules

- Keep transport logic in `server`.
- Keep business logic in `service`.
- Keep persistence logic in `store`.
- Keep data types and validation close to `models`.
- Define interfaces in the consumer package when possible.

## Naming Rules

- Use `ID`, `URL`, `HTTP`, `JSON`, `API`, `YAML`, `MCP`, `SQL` consistently.
- Prefer `ErrXxx` for sentinel errors and `XxxError` for error types.
- Do not mix acronym styles such as `Http` and `HTTP`.

## Avoid

- persistence outside the documented SQLite WAL choice
- `init()` outside configuration/bootstrap cases
- global mutable state
- "future-proof" interfaces or wrapper layers with no current consumer need
- helpers that save only one or two lines while hiding behavior
- placeholder TODO logic shipped as if complete
- unrelated refactors in the same change

## Mechanical Changes

Use `gopls`, `gofmt -r`, `goimports` or a throwaway `go/ast` program for renames,
signature changes and table edits; never `sed` or regex on Go source. See
AGENTS.md, "Refactoring Go Code".
