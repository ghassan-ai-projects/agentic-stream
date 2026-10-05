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
  Domain data is extracted to `internal/eventschema/registry_data.json`,
  `internal/ingress/simulator_data.json`, and
  `internal/episodes/testdata/aquaculture_intents.json` (see
  `docs/design/impl/GO_DOMAIN_DATA_EXTRACTION.md`).

Do not invent architecture outside the documented design. The design was written to be built as specified; deviations need a design change first.

## Architecture Overview

The documented structure (see [docs/design/TECHNICAL_DESIGN.md §23](docs/design/TECHNICAL_DESIGN.md)):

- `cmd/agentic-stream/` - entrypoint, flags, wiring, shutdown
- `internal/contractsv1` - versioned envelopes and JSON contracts
- `internal/spec` - SituationSpec authoring, YAML in, canonical JSON digest
- `internal/ingress` - ingress adapters (normalized JSONL and simulator replay; HTTP/MQTT deferred)
- `internal/eventlog` - normalized event log, watermark/completeness tracking
- `internal/engine` - deterministic stream engine core
- `internal/operators` - deterministic operators (hysteresis, debounce, cooldown)
- `internal/situations` - Situation state machine, versioning, publication
- `internal/cognition` - deterministic cognitive scheduler
- `internal/admission` - episode admission from the scheduler queue
- `internal/episodes` - bounded episode lifecycle
- `internal/executor/native`, `internal/executor/remote` - concrete executors behind the episode `Executor` port (in-process and streamed worker protocol)
- `internal/evidence` - evidence/tool boundary for episodes
- `internal/decisions` - typed Decision model
- `internal/policy` - policy plane; revalidates every intent before dispatch
- `internal/actions` - governed dispatch plane, idempotency, verification
- `internal/watch` - derived-trigger watches installed by approved commands
- `internal/actionport` - approved-command/effect contracts without implementation dependencies
- `internal/device` - concrete device adapters, sessions, materialization and gateway links
- `internal/episodeledger` / `internal/scheduleledger` / `internal/approvalledger` - durable lifecycle owners shared through transaction-scoped operations
- `internal/control` - runtime ownership, epoch drain/kill and final readiness capability
- `internal/authority` - device claims, bindings, reconciliation and safety evidence; the reference module: thin `Service` facade, use cases in `internal/app`, pure `internal/domain`, transactions and SQL in `internal/store` (see [module pattern](docs/authority-reference-module-2026-10-05/MODULE_PATTERN.md) and its [ubiquitous language](docs/authority-reference-module-2026-10-05/UBIQUITOUS_LANGUAGE.md))
- `internal/qualification` - calibration and shadow evidence
- `internal/replay` - deterministic replay; replay never performs external effects
- `internal/api` - JSON/HTTP plus Server-Sent Events
- `internal/telemetry` - OpenTelemetry traces, metrics, logs
- `internal/storage` - SQLite WAL via `modernc.org/sqlite`
- `internal/clock` - virtual/physical clock abstraction
- `proto/agenticstream/runtime/v1/` - worker protocol (Protobuf/gRPC over UDS)
- `internal/spec/schema.json` and `internal/contractsv1/schemas/v1/` - embedded JSON Schemas
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
- Wrap errors with `%w`.
- Keep handlers thin; business logic lives in the engine/operators/situations/policy packages, not in API handlers.
- Prefer standard library helpers such as `cmp`, `maps`, and `slices`.
- Prefer existing package boundaries and local helpers over new abstractions.
- Document exported symbols.
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


## Forbidden Changes

- Do not add secrets, credentials, or machine-specific private data.
- Do not add network calls to unit tests.
- Do not let untrusted content become instructions or executable parameters (invariant 1).
- Do not add graph engines, brokers, LangChain/LangGraph, or a web UI into the version-1 core (see design README).
- Do not give models direct access to effectors or production credentials.
- Do not re-author domain data in Go code. Event schemas live in
  `internal/eventschema/registry_data.json`, the simulator channel→field mapping in
  `internal/ingress/simulator_data.json`, and the aquaculture intent catalog in
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
