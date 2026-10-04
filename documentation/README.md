# Agentic Stream documentation

Agentic Stream watches evidence over time, keeps a record of the condition it
reveals, and starts a limited agent reasoning session when that would be useful.
A separate policy and action path checks proposed changes before they execute.

This is the public reading path for an unreleased development snapshot. See
[current status](overview/status.md) and [limitations](overview/limitations.md)
for the boundary between implemented behavior and production qualification.

## Start with the idea

![Evidence becomes versioned Situation state; useful changes may start an episode; permitted proposals enter governed action.](assets/runtime-story.svg)

Observations build a **Situation**, the evolving condition. A meaningful change
may start an **episode**, a bounded reasoning session. Its proposal reaches an
effect only through policy and governed dispatch. Each arrow is conditional;
not every reading needs reasoning, and not every proposal may execute.
[Diagram source](assets/runtime-story.mmd) ·
[Architecture evidence](architecture/overview.md).

## Three starting points

| Your goal | Start here |
| --- | --- |
| Understand the concepts, how it works, and why | [Guided learning path](learn/README.md) |
| Build and run the local example | [Quickstart](getting-started/quickstart.md) |
| Check an exact contract or command | [Reference](reference/README.md) and [contracts](contracts/README.md) |

## Choose a path

### I want to run it

- [Install and build](getting-started/install.md)
- [Quickstart](getting-started/quickstart.md)
- [Live runtime](getting-started/live-runtime.md)
- [Predictive-maintenance walkthrough](guides/predictive-maintenance.md)
- [Troubleshooting](guides/troubleshoot.md)

### I want to understand it

- [Learn step by step](learn/README.md) — purpose, motor story, time, reasoning, actions, and design choices.
- [Core concepts](overview/concepts.md) — a short vocabulary reference.
- [Architecture overview](architecture/overview.md)
- [Business modules and ownership](architecture/modules.md)
- [Durability and recovery](architecture/durability.md)
- [Worker boundary](architecture/worker-boundary.md)
- [Security model](architecture/security-model.md)
- [Product invariants](architecture/invariants.md)
- [Public design reading order](design/README.md)

### I want to integrate or extend it

- [Author a SituationSpec](getting-started/first-situation.md)
- [Add a domain](guides/add-a-domain.md)
- [Build a Go worker](guides/build-a-go-worker.md)
- [Contract index](contracts/README.md)
- [CLI reference](reference/cli.md)
- [HTTP and SSE reference](reference/http-api.md)

### I want to operate or review it

- [Operations](operations/README.md)
- [Recovery runbook](operations/recovery.md)
- [Security hardening](operations/security-hardening.md)
- [Telemetry](operations/observability.md)
- [Quality and release gates](governance/quality.md)
- [Release evidence](governance/release.md)
- [Roadmap](roadmap.md)

## Documentation map

| Area | Purpose |
| --- | --- |
| [Learn](learn/) | Concepts explained through a motor story, with gradually deeper diagrams |
| [Overview](overview/) | Product, concepts, status, compatibility, limitations |
| [Getting started](getting-started/) | Build and run the supported local workflows |
| [Architecture](architecture/) | Runtime shape, durability, trust boundaries, invariants |
| [Design](design/) | Public summaries of the subsystem design |
| [Contracts](contracts/) | SituationSpec, events, Decisions, worker protocol, persistence |
| [Guides](guides/) | Task-oriented authoring, integration, proof, troubleshooting |
| [Operations](operations/) | Serving, recovery, telemetry, and hardening |
| [Reference](reference/) | Exact CLI, configuration, API, event, migration, and test details |
| [ADRs](adr/README.md) | Decision index and relationship to the design archive |
| [Governance](governance/) | Quality and release criteria |

Maintainers can also read the [documentation plan](PLAN.md) and the
[acceptance bar](BAR.md).

## Public docs and the working archive

`documentation/` is the public reading path. `docs/` remains the working
archive for full technical design records, research, audits, implementation
notes, contracts, examples, and historical design iterations. The archive is
useful evidence, but it is intentionally not the first stop for a new reader.

See [the archive guide](../docs/README.md) for the relationship between the two
trees and the source-of-truth rules.

## Source of truth

Public pages are summaries. Implemented behavior is verified against code,
tests, schemas, Protobuf, migrations, the `Makefile`, and CI. The public set is
accepted against [the documentation quality bar](BAR.md). When a summary and
the repository disagree, the discrepancy is a documentation bug to resolve;
when the design and implementation disagree, the page must name the boundary
and link to the evidence.

## Project entrypoints

- [Root README](../README.md) — product landing page.
- [Contributing](../CONTRIBUTING.md) — contribution workflow and gates.
- [Security](../SECURITY.md) — vulnerability reporting and security baseline.
- [Support](../SUPPORT.md) — issue triage and reproducible reports.
- [Code of Conduct](../CODE_OF_CONDUCT.md) — community expectations.

## Next reads

- [Start learning](learn/why.md)
- [Product](overview/product.md)
- [Quickstart](getting-started/quickstart.md)
- [Current status](overview/status.md)
