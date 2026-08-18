# Documentation reorganization plan

Audience: maintainers. Scope: the public/archive split, information
architecture, source authority, migration sequence, and review loop.

This plan turns the repository into a documentation product without weakening
the existing design and test contracts. It is itself a public maintainer
document: readers can inspect the intended information architecture and the
rules used to keep it honest.

## Design principles

1. `documentation/` is the curated public reading path. `BAR.md` and `PLAN.md`
   are public maintainer-governance pages, not end-user tutorials.
2. `docs/` is the working archive: authoritative design records, research,
   implementation notes, contracts, fixtures, and historical iterations.
3. Code and tests remain the authority for implemented behavior; public pages
   summarize them and link to the exact source evidence.
4. The root README is a short product landing page. It points to
   `documentation/README.md`, not directly into the archive.
5. Every section has a clear audience and a bounded purpose. Reference pages
   state exact names; guide pages show tasks; design pages explain why.
6. Limitations are first-class. A capability is documented as implemented only
   when code and tests support the claim.
7. One release-status record is the canonical posture source for status,
   limitations, roadmap, release, and root README claims.

## Target tree

```text
documentation/
├── README.md                         public map and reader journeys
├── BAR.md                            acceptance criteria for this set
├── PLAN.md                           migration and ownership plan
├── overview/
│   ├── README.md                     section index
│   ├── product.md                    purpose, audience, non-goals
│   ├── concepts.md                   evidence, Situations, episodes, intents
│   ├── status.md                     implemented capability map and evidence
│   ├── compatibility.md              Go, OS, storage, worker, protocol support
│   └── limitations.md                honest current boundaries
├── getting-started/
│   ├── README.md                     section index
│   ├── install.md                    prerequisites and build
│   ├── quickstart.md                 validate, replay, and run the first trace
│   ├── live-runtime.md               run-live and serve workflows
│   └── first-situation.md            author a minimal SituationSpec
├── architecture/
│   ├── README.md                     section index
│   ├── overview.md                   system shape and dependency direction
│   ├── durability.md                 SQLite, ownership, checkpoints, recovery
│   ├── worker-boundary.md            Go worker and EvidenceTools boundary
│   ├── security-model.md             trust boundaries and fail-closed rules
│   ├── invariants.md                 the ten release-blocking invariants
│   └── repository-map.md              where implementation and data live
├── design/
│   ├── README.md                     public design reading order
│   ├── stream-processing.md          ingress, event time, operators, situations
│   ├── cognition.md                  scheduler, episodes, budgets, cancellation
│   ├── decisions-and-actions.md      decision validation, policy, outbox, effects
│   ├── replay-and-shadow.md          deterministic replay and effect isolation
│   ├── observability.md              traces, metrics, logs, notifications
│   └── deployment-model.md           modular monolith and deferred scale-out
├── contracts/
│   ├── README.md                     contract index and versioning rules
│   ├── situation-spec.md              YAML authoring and canonical digest
│   ├── event-envelope.md             normalized input and schema registry
│   ├── decision-intent.md            Decision and Intent safety contract
│   ├── notifications.md              notification types and delivery semantics
│   ├── worker-protocol.md            Protobuf/gRPC current-v1 boundary
│   └── persistence.md                migrations and durable records
├── guides/
│   ├── README.md                     section index
│   ├── predictive-maintenance.md     end-to-end proof walkthrough
│   ├── add-a-domain.md               add schemas, mappings, and intents safely
│   ├── build-a-go-worker.md          worker integration and conformance
│   └── troubleshoot.md               common failures and evidence to collect
├── operations/
│   ├── README.md                     operator entrypoint
│   ├── runtime.md                    readiness, serving, and lifecycle
│   ├── recovery.md                   ownership, restart, quarantine, redrive
│   ├── observability.md              telemetry and SSE operations
│   └── security-hardening.md         deployment checklist and secrets
├── reference/
│   ├── README.md                     section index
│   ├── cli.md                        command and flag reference
│   ├── configuration.md              environment and runtime configuration
│   ├── http-api.md                   health, metrics, SSE, and control surfaces
│   ├── event-catalog.md               built-in schemas and simulator inputs
│   ├── migrations.md                  schema history and upgrade posture
│   └── testing.md                    commands, fixtures, and evidence map
├── adr/
│   └── README.md                     decision index and archive relationship
├── governance/
│   ├── README.md                     section index
│   ├── quality.md                    documentation and engineering gates
│   ├── release-status.json            machine-readable current posture
│   └── release.md                    release evidence and checklist
└── roadmap.md                        shipped, hardening, deferred
```

## Migration rules

- Do not delete or silently rewrite the existing `docs/` archive in this pass.
- Do not move fixtures that are loaded by tests unless all code references are
  updated and the full suite proves the move.
- Do not copy every historical design page into the public set. Summarize the
  stable contract and link to the archive for deep implementation detail.
- Preserve exact archive links in pages that make design or status claims.
- Update root `README.md` to point to the public set and label the archive.
- Add `docs/README.md` to explain the archive’s purpose and classify its
  current-authority, generated, historical, research, and evidence material.
- Add mandatory open-source entrypoints: `SUPPORT.md`, `CODE_OF_CONDUCT.md`,
  and `CHANGELOG.md`. They must contain project-specific routing and must not
  promise unsupported service levels.
- Add a `make docs-check` target and wire it into CI. It must check internal
  links, public-page reachability, archive markers, root entrypoint links,
  machine-local path/secret leakage, non-stub pages, actual CLI/API/env
  surfaces, and key contract references.
- Perform a surface inventory before any page claims implementation: extract
  CLI commands/flags, registered HTTP/SSE routes, environment variables,
  schemas, protocol files, migrations, and focused test evidence.

## Page contract

Every substantive public page starts with a direct purpose statement and, where
the topic can drift, identifies its status or last-verified source. It must
clearly separate implemented behavior from design target, partial evidence,
deferred work, and historical material. It ends with `Next reads`. Reference
pages own exact names; guide pages own runnable procedures; design pages own
conceptual explanation; operations pages own procedures and failure response.

## Source authority matrix

| Claim or artifact | Machine/source authority | Public summary |
| --- | --- | --- |
| CLI commands and flags | `cmd/agentic-stream/main.go` and CLI tests | `reference/cli.md` |
| HTTP/SSE routes | `internal/api/`, `internal/notify/`, API tests | `reference/http-api.md` |
| SituationSpec runtime schema | `internal/spec/schema.json`, compiler tests | `contracts/situation-spec.md` |
| Worker protocol | `docs/design/contracts/runtime-v1.proto`, generated stubs, proto tests | `contracts/worker-protocol.md` |
| Runtime JSON schemas | `internal/contractsv1/schemas/v1/`, contract tests | `contracts/decision-intent.md` |
| Notification schemas | `internal/notifycontract/contracts/`, contract tests | `contracts/notifications.md` |
| Durable schema | `migrations/`, storage tests | `contracts/persistence.md`, `reference/migrations.md` |
| Runtime behavior | subsystem packages and focused/E2E tests | `overview/status.md`, design summaries |
| Release posture | `governance/release-status.json` | status, limitations, roadmap, release |
| Historical rationale | `docs/design-v0*`, dated audits, research | ADR/archive pages |

The two SituationSpec schema files and duplicated notification contract files
must be explicitly labeled where they differ or mirror one another. The public
documentation must never imply that a design-only endpoint or CLI command is
implemented.

## Review sequence

1. Review this plan for audience coverage, repository fit, and unnecessary
   duplication.
2. Create the archive guide and release-status record.
3. Inventory actual CLI, API, environment, schema, protocol, migration, and
   test surfaces.
4. Build the navigation and quality/governance pages first.
5. Write the overview and getting-started journeys against executable code.
6. Write architecture, design, contracts, guides, operations, and reference
   pages in that order.
7. Update root links and archive labels.
8. Run `make docs-check` and the five reviews in `BAR.md`: completeness, correctness, code alignment,
   style/diagrams, and open-source readiness.
9. Fix all P0/P1 findings, then repeat until the review set is clean.

## Ownership map

| Claim type | Primary evidence |
| --- | --- |
| User-facing command behavior | `cmd/agentic-stream/main.go` and CLI tests |
| Runtime behavior | subsystem packages (`eventlog`, `engine`, `operators`, `situations`, `cognition`, `episodes`, `evidence`, `decisions`, `policy`, `actions`, `replay`, `runtime`, `storage`, `notify`, `telemetry`) and package/E2E tests |
| Invariants and architecture | `AGENTS.md`, `docs/design/README.md`, technical design, tests |
| Contracts and schemas | `internal/contractsv1`, `internal/spec`, `proto/`, `migrations/` |
| Domain data | embedded JSON registries and their digest/parity tests |
| Quality gates | `Makefile`, CI workflow, `CONTRIBUTING.md`, tests |
| Security posture | `SECURITY.md`, runtime validation, policy/action tests |
