# Project Context

## What This Repo Is

Agentic Stream is a streaming-native agent runtime. It continuously converts unbounded evidence into durable, versioned Situations and starts bounded agent episodes only when a deterministic cognitive scheduler decides reasoning is useful. Agents return typed Decisions and Action Intents; a separate deterministic policy and action plane decides what may execute.

The design is implementation-ready; the curated public documentation is under `documentation/`. The ten product invariants are release-blocking.

## Current State

- Design baseline v1 is complete; runtime contract sources are `proto/agenticstream/runtime/v1/runtime-v1.proto`, `internal/contractsv1/`, `internal/spec/internal/domain/schema.json`, `proto/`, and `migrations/`.
- Specs shared by modules live in `examples/`; data private to one module lives in that module's `testdata/`.
- Module path `github.com/ghassan-ai-projects/agentic-stream` is set.
- The CLI lives in `cmd/agentic-stream/`; runtime packages live under `internal/`; generated worker stubs live under `proto/agenticstream/runtime/v1/`.
- The current implementation includes deterministic replay, live JSONL processing, bounded cognition/episodes, policy/actions, worker/evidence boundaries, notifications, telemetry, and storage recovery. The remaining release posture is documented in `documentation/governance/release-status.json`.

## What Agents Should Optimize For

- Build the documented design; do not invent architecture outside it.
- Preserve the invariants: evidence is never instructions; Situation versions are immutable after publication; deterministic state changes are serial per virtual partition; models propose intents but cannot execute effects; policy revalidates before dispatch; replay never performs external effects.
- Keep determinism airtight — replay of any committed trace must reproduce the same Situation history.
- Preserve parity between prose, `Makefile`, CI, and the design docs.
- Prefer durable repo files over long always-loaded guidance.

## Main Risks

- Letting untrusted content become instructions or executable parameters.
- Weakening determinism (timing, map iteration, non-canonical JSON, unseeded randomness, out-of-order watermark handling).
- Adding deferred scope (Kafka/NATS/Pulsar, web UI, multi-agent, vector retrieval) before the single-node semantic suite passes.
- Drifting from the documented contracts (`documentation/contracts/`, `proto/agenticstream/runtime/v1/runtime-v1.proto`, embedded schemas, `proto/`, and `migrations/`).
- Documenting commands that do not match actual Makefile/CI behavior.
