# Agentic Stream documentation

Agentic Stream is a streaming-native agent runtime: it turns unbounded
evidence into durable, versioned **Situations**, then starts bounded agent
**episodes** only when a deterministic cognitive scheduler says reasoning is
useful. Agents return typed Decisions and Action Intents; deterministic policy
and action planes decide what may execute.

This is the curated public documentation set for the current repository. The
repository is an implementation-ready development snapshot, not a declaration
of a stable production release. Read [limitations](overview/limitations.md)
before building on it.

## Start here

- [Product](overview/product.md) — what the runtime is, who it is for, and what
  it deliberately does not do.
- [Core concepts](overview/concepts.md) — the vocabulary and mental model.
- [Quickstart](getting-started/quickstart.md) — build, validate, replay, and
  run the first trace.
- [Current status](overview/status.md) — what is implemented, partial, or
  deferred, with code evidence.
- [Limitations](overview/limitations.md) — release posture and fault-model
  boundaries.

## Choose a path

### I want to run it

- [Install and build](getting-started/install.md)
- [Quickstart](getting-started/quickstart.md)
- [Live runtime](getting-started/live-runtime.md)
- [Predictive-maintenance walkthrough](guides/predictive-maintenance.md)
- [Troubleshooting](guides/troubleshoot.md)

### I want to understand it

- [Architecture overview](architecture/overview.md)
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

- [Product](overview/product.md)
- [Quickstart](getting-started/quickstart.md)
- [Current status](overview/status.md)
