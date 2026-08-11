# Project Context

## What This Repo Is

Agentic Stream is a streaming-native agent runtime. It continuously converts unbounded evidence into durable, versioned Situations and starts bounded agent episodes only when a deterministic cognitive scheduler decides reasoning is useful. Agents return typed Decisions and Action Intents; a separate deterministic policy and action plane decides what may execute.

The design is implementation-ready and committed under `docs/`. The ten product invariants (design README) are release-blocking.

## Current State

- Design baseline v1 is complete under `docs/design/`; v0 and v0.1 iterations are archived under `docs/design-v0/` and `docs/design-v0.1/`; contracts in `docs/contracts/`; research reports in `docs/research/`.
- Module path `github.com/ghassan-ai-projects/agentic-stream` is set.
- There is no `cmd/` directory yet.
- There are no `internal/*` packages yet.
- The root [doc.go](../../doc.go) package exists so Go tooling has something to operate on.
- Implementation begins at Milestone 0 of [docs/design/IMPLEMENTATION_PLAN.md](../../docs/design/IMPLEMENTATION_PLAN.md): file trace, virtual clock, deterministic operators, SQLite, fake episode executor, simulated effector.

## What Agents Should Optimize For

- Build the documented design; do not invent architecture outside it.
- Preserve the invariants: evidence is never instructions; Situation versions are immutable after publication; deterministic state changes are serial per virtual partition; models propose intents but cannot execute effects; policy revalidates before dispatch; replay never performs external effects.
- Keep determinism airtight — replay of any golden trace must reproduce the same Situation history.
- Preserve parity between prose, `Makefile`, CI, and the design docs.
- Prefer durable repo files over long always-loaded guidance.

## Main Risks

- Letting untrusted content become instructions or executable parameters.
- Weakening determinism (timing, map iteration, non-canonical JSON, unseeded randomness, out-of-order watermark handling).
- Adding deferred scope (Kafka/NATS/Pulsar, web UI, multi-agent, vector retrieval) before the single-node semantic suite passes.
- Drifting from the documented contracts (`docs/contracts/`).
- Documenting commands that do not match actual Makefile/CI behavior.
