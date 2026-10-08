# AGENTS.md - Agentic Stream

Canonical instructions for coding agents in this repository. Read this file first, then load only the context files needed for the task.

## Purpose

Agentic Stream is a streaming-native agent runtime ("Situation Runtime"): it continuously converts unbounded evidence into durable, versioned **Situations**, and starts bounded agent **episodes** only when a deterministic cognitive scheduler decides reasoning is useful. Agents return typed Decisions and Action Intents; a separate deterministic policy and action plane decides what may execute.

Deliberately lighter than a general-purpose agent framework: no channel gateway, no graph engine in the event hot path, no LLM invocation per event, no multi-agent mesh, no direct model access to effectors or production credentials in version 1.

The design is implementation-ready and lives in [docs/design/](docs/design/README.md). The ten product invariants in the design README are release-blocking, not guidelines. The MVP proof is the predictive-maintenance example: a simulated motor emits sensor events, and the release must satisfy the acceptance list in the design README (deterministic replay, duplicate/out-of-order handling, hysteresis, episode cancellation, governed intents, idempotent effects, shadow mode, explainability).

## Engineering Priorities

Use these priorities in order:

1. Correctness and safety.
2. Simplicity of design and implementation.
3. Test evidence for changed behavior.
4. Consistency with existing repo rules and automation.
5. Speed of implementation.

If two options both work, choose the one that is easier to read, easier to test, and easier for the next agent to extend.

## Read Order

Before editing:

1. Read this file.
2. Read [README.md](README.md).
3. Read the relevant sections of [docs/design/TECHNICAL_DESIGN.md](docs/design/TECHNICAL_DESIGN.md) and [docs/design/DECISIONS.md](docs/design/DECISIONS.md) before touching architecture, invariants, or protocol surfaces.
4. Check the worktree with `git status --short`.
5. Read the smallest relevant context files under `.agents/context/`.
6. Make a short plan before editing.

Start with these context files:

- [.agents/context/project.md](.agents/context/project.md) for repository scope and current state.
- [.agents/context/architecture.md](.agents/context/architecture.md) for layout and dependency direction.
- [.agents/context/testing.md](.agents/context/testing.md) for commands and testing bar.
- [.agents/context/go-style.md](.agents/context/go-style.md) for coding conventions.
- [.agents/context/quality-bar.md](.agents/context/quality-bar.md) for the enforced quality and modularity bar (lint, complexity, file size, coverage, layering).
- [.agents/context/architecture-bar.md](.agents/context/architecture-bar.md) for business ownership and one-way dependency/control-flow gates.
- [.agents/context/review-checklist.md](.agents/context/review-checklist.md) before handoff.

Use the prompt files under `.agents/prompts/` when the task matches them.

## Current Repository State

- Module path: `github.com/ghassan-ai-projects/agentic-stream` (set).
- Design baseline is complete under `docs/` (current v1 design plus archived v0/v0.1 iterations, contracts, examples, research reports). The executable runtime is implemented through the current P-series; deployment qualification remains a separate release gate. Public documentation is curated under `documentation/`.
- Implementation is complete through the P-series phases: the CLI lives in
  `cmd/agentic-stream/` (`version`, `validate`, `run-live`, `serve`, …) and `internal/`
  holds the spec compiler, ingress, eventlog, operators, situations, cognition,
  episodes, decisions, policy, actions, worker runtime, and storage. The worker
  protocol is `proto/agenticstream/runtime/v1/`; migrations live in `migrations/`.
  Domain data is extracted to `internal/spec/internal/domain/event_schema_data.json`,
  `internal/ingress/internal/domain/simulator_data.json`, and
  `internal/episodes/testdata/aquaculture_intents.json` (see
  `docs/design/impl/GO_DOMAIN_DATA_EXTRACTION.md`).

Do not invent architecture outside the documented design. The design was written to be built as specified; deviations need a design change first.

## Architecture Overview

The documented structure (see [docs/design/TECHNICAL_DESIGN.md §23](docs/design/TECHNICAL_DESIGN.md)):

- `cmd/agentic-stream/` - entrypoint, flags, wiring, shutdown
- `internal/contractsv1` - versioned envelopes and JSON contracts
- `internal/canonicaljson` - RFC 8785 canonical JSON and domain-separated digests; thin facade over a pure domain (see [canonical JSON guide](internal/canonicaljson/README.md))
- `internal/spec` - SituationSpec authoring, YAML in, canonical JSON digest
- `internal/ingress` - configured ingress facade; app replay and live-serve use cases, pure domain admission and simulator rules, a checkpoint store and a file/socket transport (see [ingress module guide](internal/ingress/README.md))
- `internal/eventlog` - normalized event log, watermark/completeness tracking; append/quarantine/redrive use cases in `internal/app`, pure admission and identity rules in `internal/domain`, all SQL in `internal/store` (see [event log module guide](internal/eventlog/README.md))
- `internal/engine` - configured stream-engine facade; app use cases, pure domain rules and an opaque-transaction store that owns the inbox, checkpoint, operator-state, Situation, lineage and timer tables (see [engine module guide](internal/engine/README.md))
- `internal/operators` - deterministic operators (hysteresis, debounce, cooldown)
- `internal/situations` - Situation state machine, versioning, publication
- `internal/cognition` - configured scheduler facade; ordered app use cases, pure domain rules, and opaque caller-transaction store (see [cognition module guide](internal/cognition/README.md))
- `internal/episodes` - configured `Service` facade; ordered assembly/execution use cases in `internal/app`, pure contracts and rules in `internal/domain`, opaque transaction joins and SQL in `internal/store` preserving the caller's transaction (see [episodes module guide](internal/episodes/README.md))
- `internal/executor/fixture`, `internal/executor/native`, `internal/executor/remote` - concrete executors behind the episode `Executor` port (in-process and streamed worker protocol)
- `internal/evidence` - configured Service facade over capability-scoped read tools and durable call recovery; app use cases, pure domain rules, opaque store transactions/SQL, exact wire codecs and gRPC/eventlog adapters (see [evidence module guide](internal/evidence/README.md))
- `internal/decisions` - pure Decision/Intent validator; thin facade over `internal/domain`, with opaque compiled intent authority (see [decisions module guide](internal/decisions/README.md))
- `internal/policy` - policy plane; revalidates every intent before dispatch. Reference structure: thin `Service` facade, ordered use cases in `internal/app`, pure governance records/rules in `internal/domain`, and caller-owned transaction plumbing plus SQL in `internal/store` (see [policy module pattern](internal/policy/README.md))
- `internal/actions` - configured dispatch facade; ordered app use cases, pure domain rules and document checks, and an opaque-transaction store that owns the command, outbox, outcome and verification ledgers (see [actions module guide](internal/actions/README.md))
- `internal/watch` - derived-trigger watches installed by approved commands; configured facade, app use cases, pure domain rules and an opaque-transaction store (see [watch module guide](internal/watch/README.md))
- `internal/notify` - durable notification outbox and lifecycle contract; configured facade, app use cases, pure domain rules (including the embedded Channel-B contract) and a store that owns the five notification tables (see [notify module guide](internal/notify/README.md))
- `internal/actionport` - approved-command/effect contracts without implementation dependencies
- `internal/device` - device effect boundary; the reference adapter module: thin facade, session use cases in `internal/app`, pure `internal/domain`, record codec in `internal/wire`, gateway link in `internal/transport` (record in [docs/device-reference-module-2026-10-05](docs/device-reference-module-2026-10-05/README.md))
- `internal/episodeledger` - durable episode pipeline lifecycle (scheduler queue items, episodes, fenced attempts, rejection audit, recovery); configured facade, app use cases, pure domain rules and an opaque-transaction store (see [episode ledger guide](internal/episodeledger/README.md))
- `internal/approvalledger` - durable human approval lifecycle (request, expiry, resolution, withdrawal); configured facade, app use cases, pure domain and an opaque-transaction store; imports no `notify`, the caller supplies the withdrawal publisher (see [approval ledger guide](internal/approvalledger/README.md))
- `internal/runtime` - thin live-pipeline, readiness and worker facades; concrete assembly in `internal/composition`, ordered use cases in `internal/app`, pure rules/reports in `internal/domain`, transaction plumbing in `internal/store`, source and worker resource adapters in `internal/transport` (see [runtime module guide](internal/runtime/README.md)).
- `internal/control` - runtime control plane: owner lease, epoch drain/kill, final readiness capability and cost control (ledger, ceilings, kill switch); configured facade, app use cases, pure domain rules and an opaque-transaction store (see [control module guide](internal/control/README.md))
- `internal/authority` - device claims, bindings, reconciliation and safety evidence; the reference module: thin `Service` facade, use cases in `internal/app`, pure `internal/domain`, transactions and SQL in `internal/store` (see [module pattern](docs/authority-reference-module-2026-10-05/MODULE_PATTERN.md) and its [ubiquitous language](docs/authority-reference-module-2026-10-05/UBIQUITOUS_LANGUAGE.md)). To bring another package to this standard, follow [the reference module refactor prompt](.agents/prompts/reference-module-refactor.md)
- `internal/replay` - effect-safe replay modes; ordered session use cases in `internal/app`, pure verification rules in `internal/domain`, all replay SQL in `internal/store`, trace files and isolated databases in `internal/transport` (see [replay module guide](internal/replay/README.md))
- `internal/api` - JSON/HTTP plus Server-Sent Events
- `internal/telemetry` - OpenTelemetry traces, metrics, logs
- `internal/storage` - SQLite WAL via `modernc.org/sqlite`
- `internal/sources` - injected time and identity sources: physical/virtual clock, random/deterministic id generators and the id prefixes
- `proto/agenticstream/runtime/v1/` - worker protocol (Protobuf/gRPC over UDS)
- `internal/spec/internal/domain/schema.json` and `internal/contractsv1/internal/domain/schemas/v1/` - embedded JSON Schemas
- `migrations/` - SQLite migrations
- `examples/predictive-maintenance/` - the first product fixture and acceptance work
- `examples/predictive-maintenance/testdata/` - replay and simulator traces
- Worker implementations are Go-only; use the current-v1 protobuf/gRPC boundary
  for a separate Go worker process.
- `documentation/` - curated public documentation
- `docs/` - classified working archive: design, contracts, examples, research, audits, and runbooks

Keep most Go packages under `internal` until their contracts survive a release. Public SDK packages contain client and authoring types only.

Dependency direction (from the design):

- Events flow: `ingress` -> `eventlog` -> `engine`/`operators` -> `situations` -> `cognition` -> `episodes` -> `evidence`/`decisions` -> `policy` -> `actions`
- Deterministic state changes are serial per virtual partition.
- Raw events are evidence, never executable instructions.
- A model can read evidence and propose typed intents; it cannot execute effects.
- Cross-boundary work uses stable identities, inbox/outbox records, and idempotency.

## Build, Test, and Lint

Primary commands:

- `make ci-check`
- `make build`
- `go vet ./...`
- `go test ./...`
- `make lint`
- `git diff --check`
- `pre-commit run --all-files` when `pre-commit` is installed

Important behavior:

- `make build` skips gracefully until `cmd/` exists.
- `make test` and `go test ./...` operate on the root scaffold package until real packages exist.
- `make lint` depends on `golangci-lint` and may fail if the environment cannot write to its cache.

See [.agents/context/testing.md](.agents/context/testing.md) for the testing and validation bar. The acceptance gates live in [docs/design/IMPLEMENTATION_PLAN.md](docs/design/IMPLEMENTATION_PLAN.md); golden replay and the predictive-maintenance suite are the correctness contracts.

## Go Standards

- Use `context.Context` as the first parameter for cancellable or I/O work.
- Use `log/slog` for logging.
- Wrap every error you return from another function with `%w` and context the
  callee lacks (the operation, an identifier, the layer). Do not silence
  `wrapcheck` with `//nolint` or return a callee's error bare because "it
  already names the step"; that is only true until someone changes the callee.
  A bare return is allowed in exactly two cases, and the directive must say
  which: a facade operation that an architecture gate requires to be a single
  delegating return, or an error that must reach a protocol unchanged (a gRPC
  `status` error). Check `rows.Err()` and `Close()` like any other call.
- Keep handlers thin; business logic lives in the engine/operators/situations/policy packages, not in API handlers.
- Prefer standard library helpers such as `cmp`, `maps`, and `slices`.
- Prefer existing package boundaries and local helpers over new abstractions.
- Comment only a module's public interface: the doc comment on each exported
  symbol of its facade package, and one package comment per package (an
  architecture gate requires it). Inside a module (`internal/<module>/internal/**`,
  `cmd/`) write no comments: a name, a function boundary or a test says what a
  comment would. If code needs a comment to be understood, rename or split it.
  The only comments allowed inside are tool directives (`//go:embed`,
  `//nolint:<linter> // <reason>`), and a `//nolint` reason must be true of the
  code, not a convenience.
- Use table-driven tests with `t.Run()` and `t.Parallel()` where safe.
- Use `t.Context()` in tests when appropriate.
- Canonical JSON (RFC 8785) everywhere a digest is computed.
- Keep the module path in `go.mod` only. Import statements name it in full
  because Go requires a module-qualified import path; everywhere a command can
  derive it, do so (`go list -m`). Do not add new string literals for it. The
  unavoidable copies (protobuf `go_package`, pre-commit `goimports -local`,
  telemetry instrumentation scope) are pinned by
  `TestModulePathSingleSourceOfTruth` in `module_path_test.go`.

See [.agents/context/go-style.md](.agents/context/go-style.md) for the repo-specific style rules.

## Code Quality Expectations

Code is written to be read top-down by the next reviewer. Every function in
production code follows these rules (quality-bar rule Q7):

- A function name states its intent.
- A function is short (at most 15 body lines) and does one thing.
- A function stays at one level of abstraction.
- Public, top-level functions read like a small domain-specific language: a
  short sequence of domain verbs over domain nouns.
- Each function calls functions one level below it, and the code keeps
  stepping down until the remaining operations are small and concrete (the
  stepdown rule): entry point first, its steps below it.

The linters enforce the mechanical floor (cognitive complexity ≤ 15, ≤ 15
body lines and 15 statements, nested-`if` ≤ 3, files under 300 lines); review
enforces the rest. A refactor toward these rules never changes behavior, and
it never adds abstraction layers without a current need. The full bar, with
examples, is [.agents/context/quality-bar.md](.agents/context/quality-bar.md).

Before accepting a refactoring round:

- Read changed entry points aloud as domain steps. Move SQL, serialization,
  transport framing, and loop bookkeeping into the step that owns them.
- Place private steps below their first caller, splitting files by responsibility
  when needed. Use names that explain the outcome; avoid numbered parts and
  wrappers that merely rename another call.
- Preserve public signatures, error precedence, identity and digest inputs,
  ordering, clock reads, transaction boundaries, locks, cancellation, and effects.
  Add regression tests for the boundaries touched by the extraction.
- Review the diff against Q7 as well as the mechanical limits. Run focused tests
  before each commit and the full gate before handoff. Keep any blocked check
  explicit; passing lint alone does not demonstrate clean functions.


## Refactoring Go Code

Change Go code with Go tooling that understands syntax, not with `sed`, regular
expressions or find-and-replace. Text edits miss call sites, hit strings and
comments, break formatting and leave the compiler to find what they missed.

- Rename a symbol, find its references or implementations: `gopls rename`,
  `gopls references`, `gopls implementation`.
- Rewrite a call or expression pattern across files: `gofmt -r 'old -> new'`.
- Fix imports after any edit: `goimports -w`.
- For anything else (remove a parameter or struct field, change a signature,
  move comments, edit an entry of a table literal such as `allowedImports`,
  strip or restructure many functions), write a small `go/parser` + `go/ast` +
  `go/format` program, run it over the files, and keep it outside the repo
  (the session scratchpad). The architecture tests in the repo root show the
  pattern.
- `sed`, `awk` and regex are for non-Go text only: Markdown, YAML, JSON, the
  Makefile.
- After a mechanical change run `gofmt -l`, `go build ./...` and `go vet ./...`
  before the tests, and read the diff: a tool that changes exactly what you
  intended produces a small, uniform diff.

## Duplication Scan

Duplicated code is a defect to fix in the change that finds it, not to leave
behind or to note for later. Run this scan on every file you add or change,
before handoff, and repeat it after fixing, until nothing is left. This stays a
required step until the repository is clean at the current threshold and the
review checklist (Q8) has stopped finding duplicates.

1. **Token clones.** `make lint` runs `dupl` at the threshold in
   `.golangci.yml`. For a stricter look at your files, run it at a lower
   threshold: copy `.golangci.yml`, set `linters.settings.dupl.threshold` to 50,
   and run `golangci-lint run --config <copy> ./...`; fix what falls in files
   you touched.
2. **Same job, different code.** Before adding a helper, search the repo for the
   job it does (`grep -rn` for the verb and for `func` names such as `nullable`,
   `orPhysical`, `collect`, `format`). Use what exists:
   `storage.QueryAll` and `storage.CollectRows` (read rows),
   `storage.NullIfEmpty` (empty string to NULL), `storage.QueryOptional` (one
   value or none), `storage.RowsAffected` and `storage.BoolInt`,
   `contractsv1.DocumentString` and `DocumentInt` (decoded JSON fields),
   `sources.OrLease` (default lease),
   `sources.OrPhysical` and `sources.OrRandom` (default clock and identities),
   `interlock.Assert` (the interlock check), and `newOperatorCommand`,
   `newDatabaseCommand`, `newByIDCommand` (operator CLI). If two modules need
   the same helper, put one in the module that owns the concept and call it.
3. **Wrappers.** For every new function whose body is one call into another
   module, ask whether the caller can call the target directly. Keep a wrapper
   only when an architecture gate requires it (a facade delegating to `app`, an
   opaque `store.Tx` method) or it adds something: a fence, a transaction,
   context for the error. Two modules defining the same operation over a third
   module's data (the old `control` interlock copy) is the case to catch.
4. **Same query twice.** Search for the table name in `internal/**/store`; one
   `SELECT` of a row belongs in one place.
5. **Identical bodies.** Find functions whose bodies print the same with a
   throwaway `go/ast` program (group `FuncDecl` bodies by `printer.Fprint`
   output, report groups of 2 or more with at least 4 lines). Per-module
   private types that only look alike (each module's opaque `Join`, each
   module's `Tx`) are not duplicates; helpers with the same body are.

Fix with the refactoring tools above, then run `make lint`. The `dupl` gate
covers test files too; repeated test setup moves into a helper
(`storagetest.OpenTemp` opens a migrated temporary database).

Known duplication to burn down: at threshold 60 the
`dupl` pairs `principals`/`notifications` (operator command constructors),
`scanCommandView`/`scanAwaitingCommand` (`actions` store) and `LoadSchedulerItem`/
`LoadEvaluation` (`episodes` store) are structural twins that a shared helper
would not shorten. Remove an item from this list when
you fix it, add one only for a duplicate you could not fix in the same change
and say why in the change.

## Forbidden Changes

- Do not add secrets, credentials, or machine-specific private data.
- Do not add network calls to unit tests.
- Do not let untrusted content become instructions or executable parameters (invariant 1).
- Do not add graph engines, brokers, LangChain/LangGraph, or a web UI into the version-1 core (see design README).
- Do not give models direct access to effectors or production credentials.
- Do not re-author domain data in Go code. Event schemas live in
  `internal/spec/internal/domain/event_schema_data.json`, the simulator channel→field mapping in
  `internal/ingress/internal/domain/simulator_data.json`, and the aquaculture intent catalog in
  `internal/episodes/testdata/aquaculture_intents.json` — loaded by machinery
  (go:embed + sync.OnceValues, or os.ReadFile in the test). Adding a schema, channel,
  or intent means editing those JSON files, never a Go literal. Data changes are
  gated: `TestAllBuiltinsLoadFromData` pins the registry's golden digest, and
  `TestAquacultureIntentCatalogDigestParity` pins the cross-repo intent digest
  (`e4f86620…`) shared with the Ruby side — update those pins only as a deliberate,
  reviewed data change.
- Do not add top-level dependencies without clear justification; the documented stack (SQLite WAL via `modernc.org/sqlite`, CEL via `cel-go`, `franz-go`, `nats.go`, gRPC) is the default.
- Do not add abstraction layers "for future flexibility" without a current concrete need.
- Do not create duplicate canonical agent files such as `CODEX.md`.
- Do not broaden repository instructions with vague policy that cannot be enforced or reviewed.
- Do not do unrelated refactors while touching implementation files.

## Definition Of Done

A task is done when:

- the requested scope is complete
- the change is specific to this repository and useful to future agents
- the change is the simplest correct one that fits the documented design
- production-code changes include meaningful tests, and modified packages do not show 0% coverage
- behavior changes respect the ten product invariants and the deterministic-replay contract
- the change meets [architecture-bar.md](.agents/context/architecture-bar.md), with no foreign lifecycle writes, upward imports, or reasoning/replay access to effect implementations
- the change keeps the [quality and modularity bar](.agents/context/quality-bar.md); thresholds are never loosened to get a diff green
- `make ci-check` passes, unless the change is documentation-only and a narrower check is clearly sufficient
- documentation is updated when behavior, commands, or expectations change
- secrets are not added, security-sensitive changes are called out, and dependency or workflow permission changes receive extra review
- Makefile targets, CI, hooks, and documented commands agree
