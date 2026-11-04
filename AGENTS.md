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
- Repository-wide rules are tests in `internal/architecture`, indexed in its
  [README](internal/architecture/README.md); its failure message states the rule. Fix the code.
  Thresholds are never loosened to get a diff green; extending an allowlist or moving a pin is a
  deliberate change named in the handoff.

## Code Rules

- A function name states its intent, its body is at most 15 lines at one level of abstraction, and
  callees sit below their first caller; exported entry points read as short sequences of domain
  steps. Linters enforce only the mechanical floor (rule Q7 in the quality bar).
- Comment only a module's public interface: exported facade symbols and one package comment per
  package. Inside `internal/<module>/internal/**` and `cmd/`, only tool directives.
- Wrap every returned error with `%w` and context the callee lacks. A bare return is allowed only
  for a gated facade delegation or a gRPC `status` error, and its `//nolint` says which.
- Edit Go with syntax-aware tools (`gopls`, `gofmt -r`, `goimports`, a throwaway `go/ast` program),
  never `sed` or regex.
- Duplicated code is a defect to fix in the change that finds it. Before adding a helper, search for
  one that does the job ([scan](.agents/context/duplication.md)).

## Forbidden Changes

- Secrets, credentials or machine-specific private data, in files or in commits.
- Model access to effectors or production credentials.
- Network calls in unit tests.
- Anything executable (Go, tests, Makefile) reading `docs/`; inputs live in `examples/` or a module's `testdata/`.
- Domain data re-authored in Go. Event schemas live in
  `internal/spec/internal/domain/event_schema_data.json` (or a directory given with `--event-schemas`,
  which may add schemas, not change built-ins), simulator channels in
  `internal/ingress/internal/domain/simulator_data.json`, and the aquaculture intent catalog in
  `internal/episodes/testdata/aquaculture_intents.json`. Their digest pins
  (`TestAllBuiltinsLoadFromData`, `TestAquacultureIntentCatalogDigestParity`) move only as reviewed data changes.
- Per-domain code branches; domains are data, through the SituationSpec.
- A graph engine, broker, LangChain/LangGraph, web UI, vector retrieval, multi-agent mesh or LLM call per event in the v1 core.
- New top-level dependencies without clear justification (the stack is SQLite WAL via `modernc.org/sqlite`, `cel-go`, gRPC), or abstraction layers without a current need.
- New string literals of the module path; `go.mod` owns it (`go list -m`).
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
A procedure or a one-area rule goes in `.agents/context/` with a row above; a rule with no
exceptions becomes a gate. Project rules go here, personal preferences in `CLAUDE.local.md`.
`TestAgentInstructionsStayShortAndCurrent` caps this file and fails on a dead link, test name, file
path or make target in the instruction files.
