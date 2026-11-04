# Go Style Context

## Core Rules

- Use `context.Context` as the first parameter for I/O, blocking, or cancellable work.
- Return `error` last.
- Use `log/slog` instead of `fmt.Println` for logging.
- Prefer standard library packages like `cmp`, `maps`, and `slices` over new helper dependencies.
- Keep packages small, lowercase, and singular.
- Prefer direct code over indirection when both are maintainable.
- Add new abstractions only after a real second use, repeated logic, or a clear testing need appears.
- Keep handlers thin; business logic lives in the owning module, not in API handlers.
- Use table-driven tests with `t.Run()` and `t.Parallel()` where safe, and `t.Context()` in tests.

## Errors

Wrap every error you return from another function with `%w` and context the callee
lacks (the operation, an identifier, the layer). Do not silence `wrapcheck` with
`//nolint` or return a callee's error bare because "it already names the step";
that is only true until someone changes the callee. A bare return is allowed in
exactly two cases, and the directive must say which: a facade operation that an
architecture gate requires to be a single delegating return, or an error that must
reach a protocol unchanged (a gRPC `status` error). Check `rows.Err()` and `Close()`
like any other call.

## Comments

Comment only a module's public interface: the doc comment on each exported symbol
of its facade package, and one package comment per package (an architecture gate
requires it). Inside a module (`internal/<module>/internal/**`, `cmd/`) write no
comments: a name, a function boundary or a test says what a comment would. If code
needs a comment to be understood, rename or split it. The only comments allowed
inside are tool directives (`//go:embed`, `//nolint:<linter> // <reason>`), and a
`//nolint` reason must be true of the code, not a convenience.

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

- A module's root package is a thin facade: configuration and delegation only.
- `internal/app` orders use cases and never touches the database.
- `internal/domain` holds pure rules: no I/O and no clock reads.
- `internal/store` holds transactions and every SQL statement, behind an opaque `Tx`.
- `internal/transport` and `internal/wire` hold framing and codecs when a module has them.
- Define interfaces in the consumer package when possible.

The full pattern is rule A12 in [architecture-bar.md](architecture-bar.md).

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

Change Go code with Go tooling that understands syntax, not with `sed`, regular
expressions or find-and-replace. Text edits miss call sites, hit strings and
comments, break formatting and leave the compiler to find what they missed.

- Rename a symbol, find its references or implementations: `gopls rename`,
  `gopls references`, `gopls implementation`.
- Rewrite a call or expression pattern across files: `gofmt -r 'old -> new'`.
- Fix imports after any edit: `goimports -w`.
- For anything else (remove a parameter or struct field, change a signature,
  move comments, edit an entry of a table literal such as `allowedImports`
  (`internal/architecture/import_rules_test.go`), strip or restructure many
  functions), write a small `go/parser` + `go/ast` + `go/format` program, run it
  over the files, and keep it outside the repo (the session scratchpad). The gates
  in `internal/architecture` show the pattern.
- `sed`, `awk` and regex are for non-Go text only: Markdown, YAML, JSON, the
  Makefile.
- After a mechanical change run `gofmt -l`, `go build ./...` and `go vet ./...`
  before the tests, and read the diff: a tool that changes exactly what you
  intended produces a small, uniform diff.
