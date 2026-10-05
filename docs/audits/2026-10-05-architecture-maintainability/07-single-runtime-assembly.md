# 7. Build live, serve, and replay through one assembly

## Problem

The runtime is put together in three places, each a little differently:

- `cmd/agentic-stream/live.go` holds `runtimeCore`, a `cleanups` stack, and
  the code that opens the database, owner, service, worker runtime, effects,
  and pipeline.
- `internal/runtime` holds `NewPipeline` and the plane composition.
- `internal/replay` builds the stream engine and episode assembler itself
  (`replay/session.go:104`, `replay/schedule.go:21`).

Configuration is spread out too. Most of it comes from flags, but some of it is
read from the environment deep inside the code. `internal/runtime/worker_runtime.go:107`
reads `AGENTIC_STREAM_MODEL_API_KEY`, and `cmd` reads the OTLP endpoint and
the control and subscriber tokens. `agentic-stream config effective` is still a
placeholder (`cmd/agentic-stream/main.go:155`).

## Evidence that the paths already differ

- The live assembler has cost control
  (`runtime/pipeline_composition.go:77`). The replay assembler does not
  (`replay/schedule.go:21`). Nothing records whether this difference is
  intended.
- The `serve` and `run-live` commands share flag helpers
  (`workerRuntimeFlagTargets`), which were added to stop them from drifting
  apart. This shows that drift has already been a problem.

## Why it matters

Deterministic replay is a release gate. If replay assembles the stream and
cognition planes differently from live, the "replay matches live" claim relies
on reviewers spotting differences. The `cmd` package also carries lifecycle
logic that is hard to test without running the binary.

## Recommendation

1. Add one `runtime.Config` type that holds every setting, including secrets.
   The `cmd` package fills it from flags and the environment. Code under
   `internal/` never calls `os.Getenv`.
2. Add `runtime.Open(ctx, cfg) (*Runtime, error)`. It returns a value with a
   single `Close` that runs cleanups in LIFO order. Move `runtimeCore`,
   `cleanups`, and the worker monitor out of `cmd`.
3. Add a shared `planes.Stream(cfg)` constructor (engine, cognition, admission
   assembler) that both `runtime.Open` and `replay` call. Replay never adds the
   effect plane, so invariant "replay never performs external effects" is kept
   by construction. Every intended difference becomes an explicit option, for
   example `CostControl: replay.Disabled`.
4. Implement `config effective` by printing the resolved `runtime.Config` with
   secrets redacted.
5. Add an architecture test: `os.Getenv` is only allowed in `cmd/`.

This changes how the runtime is composed. Write an ADR first.

## Done when

- Each `cmd` subcommand is roughly "parse flags → build Config → `runtime.Open`
  → run → `Close`".
- Replay and live share the stream and cognition constructors. The code lists
  every intended difference.
- `config effective` prints real output and has a test.
