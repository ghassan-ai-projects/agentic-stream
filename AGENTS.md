# AGENTS.md - Agentic Stream

The canonical instructions for every coding agent here. `CLAUDE.md`, `GEMINI.md` and
`.github/copilot-instructions.md` only point here and add tool-specific notes.

Agentic Stream is a streaming-native agent runtime. It turns unbounded evidence into durable,
versioned **Situations** and starts a bounded agent **episode** only when a deterministic scheduler
decides reasoning is useful. Agents return typed Decisions and Intents; a separate deterministic
policy and action plane decides what may execute.

Code, tests, embedded schemas, `migrations/` and `proto/` are the authority for implemented
behavior; [documentation/](documentation/README.md) explains them and the design. Build the
documented design: a deviation needs a design change first. A local `docs/` directory is a
git-ignored archive, never an authority.

## Priorities

In order: correctness and safety; simplicity; test evidence; consistency with existing rules and
automation; speed. When two options both work, take the one easier to read, test and extend.

## Invariants

Release-blocking, not guidelines. A change that touches one updates its proving tests in
[invariants.md](documentation/architecture/invariants.md).

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. A Situation version is immutable after publication.
4. Deterministic state changes are serial per virtual partition.
5. Every episode is bound to one immutable Situation snapshot and finite budget.
6. A model can read evidence and propose typed Intents; it cannot execute effects.
7. Policy revalidates every Intent against current state immediately before dispatch.
8. Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.
9. Replay never performs external effects unless an explicit, separate simulation mode is selected.
10. Every admitted, deferred, coalesced, rejected, canceled, and expired cognitive opportunity is explainable from durable records.

Replay determinism breaks through wall-clock reads, map iteration order, non-canonical JSON and
unseeded randomness. Take time and identities from `internal/sources`, and compute every digest
over canonical JSON (RFC 8785, `internal/canonicaljson`).

## Owner Decisions

These are not visible in the code. They override what the code seems to suggest.

- **No backward compatibility.** Nothing is released: edit migrations, tables, wire fields and APIs
  in place and fix the callers. No shims, no follow-up migration for an unreleased table. The shipped
  migration rules in [migrations.md](documentation/reference/migrations.md) apply once a release exists.
- **Main paths only.** Find and fix defects on the paths the work exercises. Add no guards for rare
  interleavings, malformed input or corner cases unless an invariant is at stake. State limits and
  defer the rest; good enough beats perfect.
- **The real-world-sensor experiment must keep working.** It is the product's end-to-end proof
  (`examples/real-world-sensor/`, run from the separate `agent-research-lab` repository with Tamoz).
  Before changing CLI flags, spec grammar, compiled spec digests, the thermal capability catalog,
  `runtime-v1.proto`, notification goldens, device-wire records or the run-export format, read the
  guards `cmd/agentic-stream/experiment_*_test.go` and the per-module pins such as
  `internal/notify/experiment_contract_test.go`. A guard that must move is called out in the handoff.
- **Tamoz is the only production reasoner.** Agentic Stream owns time and authority; Tamoz, in its
  own repository, owns judgment. `fixture` is a deterministic demo executor and `native` an in-process
  model loop for tests and baselines; `rule` answers human-approved playbook rules without a model
  call and passes the rest on. Do not propose other agents, MCP or model paths as production
  reasoners. Workers built in this repository are Go, on the current-v1 protobuf/gRPC boundary.
- **Clean the files you edit, and only those.** The tree still holds comments inside modules and
  unwrapped errors: do not copy them and do not sweep them. Fix them in each file a change already
  edits; this is the one exception to the rule against unrelated refactors.

## Commands

| Need | Command |
| --- | --- |
| The module you changed | `go test -race -shuffle=on -count=3 ./internal/<module>/...` |
| One test | `go test ./internal/<module>/... -run '^<Name>$'` |
| Repository-wide gates | `go test ./internal/architecture` |
| Whole tree, short | `make test-short` |
| Lint | `make lint` (a failure writing its cache is a sandbox limit, not a finding; report it) |
| Full gate before handoff | `make ci-check` |
| Documentation only | `make docs-check` and `git diff --check` |
| Regenerate worker stubs | `make proto-generate`; never hand-edit `*.pb.go` |
| Replace vendored plant-wire contract | `make plant-wire-sync`; never edit the copy |

Commits use Conventional Commits ([CONTRIBUTING](CONTRIBUTING.md)).

## Read Before You Act

Start with `git status --short`; the tree may hold someone else's work. These files do not load by
themselves: open the row that matches the work.

| When you | Read |
| --- | --- |
| Edit Go | [go-style.md](.agents/context/go-style.md), [quality-bar.md](.agents/context/quality-bar.md) |
| Write or change tests | [testing.md](.agents/context/testing.md) |
| Add a package, import edge, table or cross-module write | [architecture.md](.agents/context/architecture.md), [architecture-bar.md](.agents/context/architecture-bar.md) |
| Touch an invariant, contract, protocol or the design | [architecture docs](documentation/architecture/README.md), [design docs](documentation/design/README.md), [contracts](documentation/contracts/README.md) |
| Work inside a module | its `README.md` and `UBIQUITOUS_LANGUAGE.md` |
| Need to find what owns something | [repository-map.md](documentation/architecture/repository-map.md) |
| Hand off a change | [duplication.md](.agents/context/duplication.md), [review-checklist.md](.agents/context/review-checklist.md) |
| Run a long campaign or experiment | "Long Runs" in [testing.md](.agents/context/testing.md) |
| Build or rebuild a module to the reference pattern | [reference-module-refactor.md](.agents/prompts/reference-module-refactor.md) |
| Fix a bug, add a feature, plan or review | the matching file in [.agents/prompts/](.agents/prompts/) |

## Architecture Rules

- Events flow `ingress -> eventlog -> engine/operators -> situations -> cognition -> episodes ->
  evidence/decisions -> policy -> actions`. Imports do not follow that order: every import points to
  a strictly lower layer, and feedback re-enters through durable records, never a callback.
- A module with tables and rules has a thin facade, use cases in `internal/app`, pure rules in
  `internal/domain` and all SQL in `internal/store`; `internal/authority` is the reference. Only
  `runtime` and `cmd` wire concrete adapters.
- No foreign lifecycle writes, upward imports, or reasoning or replay access to effect implementations ([architecture bar](.agents/context/architecture-bar.md)).
- `internal/kernel` is the shared pure vocabulary: a symbol enters only when two or more modules need it and it is a stable representation rule, not business behaviour. Tests open a migrated database with `storagetest.OpenTemp`; production code never imports `storagetest`.
- Repository-wide rules are tests in `internal/architecture`, indexed in its
  [README](internal/architecture/README.md); its failure message states the rule. Fix the code.
  Thresholds are never loosened to get a diff green; extending an allowlist or moving a pin is a
  deliberate change named in the handoff.

## Go Standards

- Use `context.Context` as the first parameter for cancellable or I/O work, and `log/slog` for logging.
- Keep handlers thin; business logic lives in the owning module, not in API handlers.
- Prefer standard library helpers such as `cmp`, `maps` and `slices`, and existing package boundaries and local helpers over new abstractions.
- Wrap every error you return from another function with `%w` and context the callee lacks (the operation, an identifier, the layer). Do not silence `wrapcheck` with `//nolint` or return a callee's error bare because "it already names the step"; that is only true until someone changes the callee. A bare return is allowed in exactly two cases, and the directive must say which: a facade operation that an architecture gate requires to be a single delegating return, or an error that must reach a protocol unchanged (a gRPC `status` error). Check `rows.Err()` and `Close()` like any other call.
- Comment only a module's public interface: the doc comment on each exported symbol of its facade package, and one package comment per package. Inside a module (`internal/<module>/internal/**`, `cmd/`) write no comments: a name, a function boundary or a test says what a comment would. If code needs a comment to be understood, rename or split it. The only comments allowed inside are tool directives (`//go:embed`, `//nolint:<linter> // <reason>`), and a `//nolint` reason must be true of the code, not a convenience.
- Use table-driven tests with `t.Run()` and `t.Parallel()` where safe, and `t.Context()` in tests.
- Change Go with tools that understand syntax, never `sed`, `awk`, regex or find-and-replace: `gopls rename`, `references` and `implementation`; `gofmt -r 'old -> new'`; `goimports -w`; for anything else a throwaway `go/parser` + `go/ast` + `go/format` program kept outside the repo. Then run `gofmt -l`, `go build ./...` and `go vet ./...` before the tests, and read the diff ([go-style.md](.agents/context/go-style.md)).
- Duplicated code is a defect to fix in the change that finds it. Before adding a helper, search for the job it does and use what exists: `storage.QueryAll`, `storage.CollectRows` (read rows), `storage.NullIfEmpty` (empty string to NULL), `storage.QueryOptional` (one value or none), `storage.RowsAffected`, `storage.BoolInt`, `contractsv1.DocumentString`, `contractsv1.DocumentInt` (decoded JSON fields), `sources.OrLease` (default lease), `sources.OrPhysical`, `sources.OrRandom` (default clock and identities), `interlock.Assert` (the interlock check), `newOperatorCommand`, `newDatabaseCommand`, `newByIDCommand` (operator CLI). If two modules need the same helper, put one in the module that owns the concept and call it.
- Keep a wrapper only when an architecture gate requires it (a facade delegating to `app`, an opaque `store.Tx` method) or it adds something: a fence, a transaction, context for the error. One `SELECT` of a row belongs in one place. Before handoff, run the [duplication scan](.agents/context/duplication.md) on the files you changed.

## Code Quality Expectations

Code is written to be read top-down by the next reviewer. Every function in production code follows these rules as it is written, not only at review (rule Q7; examples in the [quality bar](.agents/context/quality-bar.md)):

- A function name states its intent.
- A function is short (at most 15 body lines) and does one thing.
- A function stays at one level of abstraction.
- Public, top-level functions read like a small domain-specific language: a short sequence of domain verbs over domain nouns.
- Each function calls functions one level below it, and the code keeps stepping down until the remaining operations are small and concrete (the stepdown rule): entry point first, its steps below it.

The linters enforce the mechanical floor (cognitive complexity ≤ 15, ≤ 15 body lines and 15 statements, nested-`if` ≤ 3, files under 300 lines); review enforces the rest. A refactor toward these rules never changes behavior, and it never adds abstraction layers without a current need. Do not use `//nolint` for `gocognit`, `gocyclo`, `nestif` or `funlen`, and do not delete or weaken tests or exclude packages to meet coverage: fix the code.

Before accepting a refactoring round:

- Read changed entry points aloud as domain steps. Move SQL, serialization, transport framing, and loop bookkeeping into the step that owns them.
- Place private steps below their first caller, splitting files by responsibility when needed. Use names that explain the outcome; avoid numbered parts and wrappers that merely rename another call.
- Preserve public signatures, error precedence, identity and digest inputs, ordering, clock reads, transaction boundaries, locks, cancellation, and effects. Add regression tests for the boundaries touched by the extraction.
- Review the diff against Q7 as well as the mechanical limits. Run focused tests before each commit and the full gate before handoff. Keep any blocked check explicit; passing lint alone does not demonstrate clean functions.

## Forbidden Changes

- Secrets, credentials or machine-specific private data, in files or in commits.
- Model access to effectors or production credentials.
- Untrusted content becoming instructions or executable parameters (invariant 1).
- Network calls in unit tests.
- Anything executable (Go, tests, Makefile) reading `docs/`; inputs live in `examples/` or a module's `testdata/`.
- Domain data re-authored in Go. Event schemas live in
  `internal/spec/internal/domain/event_schema_data.json` (or a directory given with `--event-schemas`,
  which may add schemas, not change built-ins), simulator channels in
  `internal/ingress/internal/domain/simulator_data.json`, and the aquaculture intent catalog in
  `internal/episodes/testdata/aquaculture_intents.json`. Their digest pins
  (`TestAllBuiltinsLoadFromData`, `TestAquacultureIntentCatalogDigestParity`, whose digest is shared with the Ruby side) move only as reviewed data changes. Load the data with `go:embed` + `sync.OnceValues`, or `os.ReadFile` in a test.
- Per-domain code branches; domains are data, through the SituationSpec.
- A graph engine, broker, LangChain/LangGraph, web UI, vector retrieval, multi-agent mesh or LLM call per event in the v1 core.
- New top-level dependencies without clear justification (the stack is SQLite WAL via `modernc.org/sqlite`, `cel-go`, gRPC), or abstraction layers without a current need.
- New string literals of the module path; `go.mod` owns it (`go list -m`). Import statements name it in full because Go requires it.
- Another canonical instruction file. A bridge file only points here.
- Unrelated refactors.

## Definition Of Done

- The requested scope is complete, by the simplest correct change that fits the design.
- Production changes carry tests that prove the behavior, written first when feasible, and no modified package shows 0% coverage.
- `make ci-check` passes, or for documentation-only work the narrower checks above do.
- Golden replay and the predictive-maintenance suite still pass. They are the correctness contracts:
  deterministic replay, duplicate and out-of-order handling, hysteresis, episode cancellation,
  governed intents, idempotent effects, shadow mode, explainability.
- Documentation, Makefile, CI and hooks agree with the behavior and commands you changed.
- The handoff lists what changed, checks run, checks skipped and why, and remaining risk, and calls
  out anything security-sensitive or any dependency or workflow-permission change. Blocked or failed
  checks are reported as such.

## Changing This File

Keep a line only if an agent could not work it out from the repository and would err without it.
A rule an agent needs while writing code stays here, even when a gate or review also enforces it, so
the code is right before review. A procedure or a one-area rule goes in `.agents/context/` with a row
above; a rule with no exceptions also becomes a gate. Project rules go here, personal preferences in `CLAUDE.local.md`.
`TestAgentInstructionsStayShortAndCurrent` caps this file and fails on a dead link, test name, file
path or make target in the instruction files.
