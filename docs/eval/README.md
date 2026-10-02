# Agentic Stream — evaluation design

Status: **design only, not implemented.** Date: 2026-09-17.
Authority: code and tests define implemented behavior. This directory defines what the
evaluation *should* measure and how a green result is licensed; it does not describe the
runtime as it is today.

## Why this exists

Agentic Stream's product claim is not "a model answers well". It is: **unbounded evidence
becomes durable, versioned Situations; bounded episodes reason only when useful; a model
proposes typed Intents; deterministic policy disposes; effects are idempotent and
explainable.** Ten product invariants are release-blocking
([`docs/design/README.md`](../design/README.md#product-invariants)).

Most of that is already proven somewhere: `go test ./...`, the conformance fixtures under
`internal/contractsv1/conformance/`, `internal/executor/conformance`, `internal/replay`,
`internal/soak`, and the `export-run` / `verify-run` artifact pair.

What does not exist is one place that says **what is measured, what a green result licenses,
which invariants have no cell, and how a release reviewer verifies the whole thing from one
artifact.** This directory is that design.

## The one idea: two classes, never averaged

| Class | What it is | Grading | A failure means |
| --- | --- | --- | --- |
| **D — deterministic guarantee** | replay determinism, immutability, idempotency, fencing, policy revalidation, replay isolation, contract conformance, explainability, safety counters | exact, record- and byte-based; one counterexample fails | a regression or a safety defect — release-blocking |
| **C — model-dependent judgement** | whether the episode's Decision/Intent matched the scenario's definition of done; abstention quality; counterfactual regret; cost | `k >= 3` trials, paired against `replay.DeterministicBaseline`, a held-out scenario family, confidence interval | a capability signal — never a release gate on its own |

**A Class D failure invalidates every Class C claim from the same run.** That is the same
safety property the runtime exists to provide, applied to its own evaluation. Averaging the two
into a single score is the one thing this design forbids.

## Claim tiers

Every verdict carries the strongest tier its evidence actually supports, and never borrows a
higher one:

| Tier | Evidence | Licenses |
| --- | --- | --- |
| `plumbing` | simulator, emulator, scripted provider | "the harness works" |
| `device_contract` | a real device-wire exchange with device-reported `receipt`/`result`/`state` | "the contract holds" — **not** "the physical effect happened" (`internal/contractsv1/conformance/README.md`: *receipt is NOT proof of physical effect*) |
| `physical` | an independent instrument observed the effect, recorded in the external bench manifest named by `REAL_WORLD_SENSOR_ROOT` | a physical claim |

`internal/soak`'s `EvidenceCompleteness` is the runtime's own version of this distinction. The
evaluation must propagate it, not flatten it.

The cross-repo bench is an optional external input, located only through the environment
(`internal/actions/physical_catalog_crossrepo_test.go` skips without it). Cells at the
`device_contract` and `physical` tiers are therefore **blocked, not failed**, when it is absent —
see bar 7 in [04-evaluation-bar.md](04-evaluation-bar.md).

## Read in this order

1. [01-measurement-model.md](01-measurement-model.md) — the axes, the graders, the verdict rule, the artifact.
2. [02-suite-catalog.md](02-suite-catalog.md) — every cell, what it proves, and its current coverage.
3. [03-implementation-plan.md](03-implementation-plan.md) — which existing seams to extend, and in what order.
4. [04-evaluation-bar.md](04-evaluation-bar.md) — when this evaluation is itself trustworthy.

## Non-goals

- **No new runtime architecture.** The evaluation observes the documented boundary; it does not add one.
- **No re-implementation of existing tests.** The evaluation aggregates them into one verdict and closes the coverage gaps; it does not duplicate `internal/*` test suites.
- **No broker, HTTP, or MQTT cells** while those adapters remain `deferred` in [`documentation/governance/release-status.json`](../../documentation/governance/release-status.json).
- **No LLM judge where a record- or byte-based grader can decide.**
- **No capability number without its trials, its baseline, its tier, and its cost.**
