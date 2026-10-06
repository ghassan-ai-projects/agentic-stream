# 03 — Implementation plan

How to build the evaluation by extending what exists. The rule from the repository's own
`AGENTS.md` applies: **understand before you build; extend, don't reinvent.** Nothing here adds
runtime architecture.

## 1. Seams to extend

| Seam | Change | Why it is the right seam |
| --- | --- | --- |
| `internal/runartifact` | add `eval_suite_digest`, `scenario_catalog_digest`, `claim_tier`, `trials_per_cell` to `Manifest`; extend `Verify` to the `eval/` files | provenance, immutability, and tamper detection already exist; the evaluation must ride them, not invent a second artifact |
| `internal/soak` | **no logic change**; surface `Report` into the run verdict and digest it into `verdict.json` | `soak` is already the safety verdict engine (`ZeroTolerance`, `EvidenceCompleteness`); forking it would create two verdicts |
| `internal/contractsv1/conformance` | grow `invalid/` as rules are added; publish the capability-catalog digest in the report | it is the declared cross-repo contract surface |
| `internal/executor/conformance` | extend the semantic parity cases beyond the fixture request | it already compares in-process and streamed worker semantics |
| `internal/replay` | add the three-run canonical projection digest, and use `DeterministicBaseline` as the Class C baseline | determinism and the non-model baseline both already live here |
| `internal/actions` device and serial tests | add the S4.4 dispatch fault matrix over PTY and the emulator | the matrix is the largest real gap; the fault points already exist as code seams |
| `internal/evidence` | capability-denial cells; the evidence ledger as the grader source | A6 needs the boundary that already denies capabilities |
| `internal/control` (cost files) | a per-cell cost record | the caps exist; the reporting granularity does not |
| `cmd/agentic-stream` | `eval run` and `eval report`; `export-run` / `verify-run` keep the artifact plane | one documented entry point |

## 2. New package

```text
internal/eval/
  catalog.go      # loads and validates the suite catalog; computes its digest
  cell.go         # Cell and Trial records; ids, class, tier, cost
  grade.go        # the five graders (record, byte, contract, counter, statistics)
  verdict.go      # cell -> axis -> run; hard-zero set; tier ceilings
  report.go       # writes cells.jsonl, coverage.json, report.json
  catalog/
    suites.json   # the catalog from 02-suite-catalog.md, as data
```

**The catalog is data, not code.** The repository already extracts domain data to JSON
(`internal/eventschema/registry_data.json`, `internal/ingress/simulator_data.json`,
`internal/episodes/testdata/aquaculture_intents.json`); the suite catalog follows that
precedent so a new cell is a data change with a digest, not a Go change.

**Scoring lives in `grade.go` and nowhere else.** The runner orchestrates; graders decide. A
grader that appears in the CLI or in a cell definition is a defect.

## 3. Command surface

```text
agentic-stream eval run    --suite <id> [--scenario-catalog <file>] --db <path> --output <dir>
agentic-stream eval report --artifact <dir>
agentic-stream export-run  --db <path> --output <dir>     # unchanged
agentic-stream verify-run  <dir>                          # unchanged, extended coverage
```

Rules:

- The evaluation runner **never writes to the runtime database.** It drives the runtime and reads records; a runner that mutates the subject cannot measure it.
- `eval run` is the only new surface. `report` is a read-only roll-up of an existing artifact.
- A run without `--output` is refused: a verdict with no artifact is not evidence.

## 4. CI lanes

| Lane | Contains | Bounds |
| --- | --- | --- |
| `make ci-check` (extend) | all Class D cells | no network, no hardware, no provider; must stay inside the current CI budget |
| `make eval-capability` (new, on demand) | S7 with a real provider | never in CI; env-gated; artifact under an ignored path; refuses to run without a holdout digest |
| `make eval-soak` (new, on demand) | S8.1 | long-running; produces a `soak.Report` digest into a real artifact |

The Class C lane is deliberately outside CI: a stochastic provider in the gate would make the
gate flaky and tempt tuning. This mirrors the separation the runtime itself makes between
deterministic policy and model judgement.

## 5. Work packages

Each package has one gate and one claim it licenses — the repo's existing plan style. The claim
is a ceiling: a later change must not present more.

| WP | Work | Gate | Claim licensed |
| --- | --- | --- | --- |
| E0 | Catalog and report schema as data; digest it | catalog compiles; every axis has cells or an explicit gap entry | the evaluated surface is enumerated and versioned |
| E1 | Conformance coverage closure (S0.2, S0.3) | every documented invalid frame is rejected by a fixture | the boundary rejects what the contract says it must |
| E2 | Determinism harness (S1.1, S1.2) | three fresh runs over each golden trace produce one projection digest | replay is byte-identical across fresh runs |
| E3 | Event-time matrix (S1.4–S1.9, S2) | each event-time case has a cell on a named trace | duplicates, out-of-order, late, idle, and restart behavior are correct |
| E4 | Dispatch fault matrix (S4.4) | every `(boundary × fault)` cell holds; zero duplicate accepted effects | no duplicate accepted effect under the fault set |
| E5 | Soak wiring (S8.1) | an 8-hour soak produces a zero-tolerance report bound to the artifact | a soak run yields a verifiable safety verdict |
| E6 | Capability harness (S7) | `k >= 3`, paired baseline, holdout digest, interval reported | a paired, held-out, interval-backed capability statement — or none |
| E7 | Operational cells (S8.2–S8.4) | backup/restore, disk-full, and unclean shutdown each have a cell | those operational properties are qualified |
| E8 | Release wiring | `documentation/governance/release-status.json` cites an artifact digest and its tier | the release claim is backed by a verifiable artifact |

## 6. Sequence

1. **E0–E2** — cheap, deterministic, and they produce the artifact everything else writes into.
2. **E4** — highest risk, and the safety property the product is sold on.
3. **E3** — alongside E4; different packages, no file overlap.
4. **E6** — off the critical path; a capability claim is worthless until the deterministic class is green.
5. **E5, E7** — before any release claim.
6. **E8** — last, so the release posture cites evidence rather than intention.

## 7. Risks and their mitigations

| Risk | Mitigation |
| --- | --- |
| A second verdict system emerges and disagrees with `soak` | `soak` stays the single safety slice; the evaluation composes it and digests it |
| The evaluation duplicates the test suite | cells reference existing tests by name; the evaluation adds only the missing matrix, the aggregation, and the coverage map |
| Hardware coupling makes CI flaky | Class D uses PTY and the emulator; physical tiers are off-CI and tier-labelled |
| A capability number is over-claimed | tier ceilings are enforced by the grader, not by convention |
| The catalog rots | it is data with a digest; `coverage.json` names every axis with zero cells |
| The runner perturbs what it measures | the runner never writes to the runtime database |
