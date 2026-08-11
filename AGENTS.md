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
- [.agents/context/review-checklist.md](.agents/context/review-checklist.md) before handoff.

Use the prompt files under `.agents/prompts/` when the task matches them.

## Current Repository State

- Module path: `github.com/ghassan-ai-projects/agentic-stream` (set).
- Design baseline is complete under `docs/` (design v1 plus archived v0/v0.1 iterations, contracts, examples, research reports). Status: implementation-ready design baseline.
- There is no `cmd/` tree yet.
- There is no `internal/` tree yet.
- The root scaffold package in [doc.go](doc.go) exists so Go tooling has something to operate on.
- Implementation starts at Milestone 0 per [docs/design/IMPLEMENTATION_PLAN.md](docs/design/IMPLEMENTATION_PLAN.md). The first vertical slice uses a file trace, virtual clock, deterministic operators, SQLite, a fake episode executor, and a simulated effector. Do not start with MQTT, LangGraph integration, a web UI, or a real model provider.

Do not invent architecture outside the documented design. The design was written to be built as specified; deviations need a design change first.

## Architecture Overview

The documented structure (see [docs/design/TECHNICAL_DESIGN.md §23](docs/design/TECHNICAL_DESIGN.md)):

- `cmd/agentic-stream/` - entrypoint, flags, wiring, shutdown
- `internal/contracts` - internal data contracts and validation
- `internal/spec` - SituationSpec authoring, YAML in, canonical JSON digest
- `internal/ingress` - ingress adapters (simulator, file replay, HTTP, later MQTT)
- `internal/eventlog` - normalized event log, watermark/completeness tracking
- `internal/engine` - deterministic stream engine core
- `internal/operators` - deterministic operators (hysteresis, debounce, cooldown)
- `internal/situations` - Situation state machine, versioning, publication
- `internal/cognition` - deterministic cognitive scheduler
- `internal/episodes` - bounded episode lifecycle
- `internal/evidence` - evidence/tool boundary for episodes
- `internal/decisions` - typed Decision model
- `internal/policy` - policy plane; revalidates every intent before dispatch
- `internal/actions` - action plane, effectors, idempotency
- `internal/replay` - deterministic replay; replay never performs external effects
- `internal/api` - JSON/HTTP plus Server-Sent Events
- `internal/telemetry` - OpenTelemetry traces, metrics, logs
- `internal/storage` - SQLite WAL via `modernc.org/sqlite`
- `internal/clock` - virtual/physical clock abstraction
- `proto/agenticstream/runtime/v1/` - worker protocol (Protobuf/gRPC over UDS)
- `schemas/v1/` - JSON Schemas
- `migrations/` - SQLite migrations
- `examples/predictive-maintenance/` - the first product acceptance test
- `testdata/golden/` - golden replay traces
- `sdk/python/` - optional model/ML workers (Python 3.12, Pydantic v2)
- `docs/` - design (v1 + archived v0/v0.1), contracts, examples, research reports

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

See [.agents/context/go-style.md](.agents/context/go-style.md) for the repo-specific style rules.

## Forbidden Changes

- Do not add secrets, credentials, or machine-specific private data.
- Do not add network calls to unit tests.
- Do not let untrusted content become instructions or executable parameters (invariant 1).
- Do not add graph engines, brokers, LangChain/LangGraph, or a web UI into the version-1 core (see design README).
- Do not give models direct access to effectors or production credentials.
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
- `make ci-check` passes, unless the change is documentation-only and a narrower check is clearly sufficient
- documentation is updated when behavior, commands, or expectations change
- secrets are not added, security-sensitive changes are called out, and dependency or workflow permission changes receive extra review
- Makefile targets, CI, hooks, and documented commands agree
