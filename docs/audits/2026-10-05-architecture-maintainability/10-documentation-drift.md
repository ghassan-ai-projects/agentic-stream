# 10. Keep agent and design docs in sync with the code

## Problem

Tests check that the public module map
(`documentation/architecture/modules.md`) lists every package. The documents
that agents read first are not checked, and they have drifted from the code.
`docs/` has also grown to 244 files, so it is hard to tell which document is
the current authority.

## Evidence

- **`AGENTS.md` leaves out 11 production packages:** `canonicaljson`,
  `costcontrol`, `duration`, `ids`, `interlock`, `notify`, `notifycontract`,
  `runartifact`, `runtime`, `soak`, and `worker`. `runtime` is the composition
  root, and `interlock` is a safety gate.
- **`AGENTS.md` describes operators wrongly.** It says `internal/operators` is
  "deterministic operators (hysteresis, debounce, cooldown)". In the code,
  hysteresis is in `situations` (`MinDuration` in `evaluate.go`), and debounce
  and cooldown are in `cognition/scheduler.go`. `operators` implements
  windowed aggregates, slope, and missing heartbeat.
- **ADRs are hard to find.** `documentation/adr/` contains only a README. All
  decisions live in one 438-line file, `docs/design/DECISIONS.md`. A link or a
  review cannot point to a single ADR.
- **`docs/` mixes current and old material:** `design/` (current), `design-v0/`
  and `design-v0.1/` (old), 82 research files, 80 audit files, 19 plans, and
  more. `.agents/context/architecture.md` still uses the heading "Target
  Structure (per design §23)", although that structure is now built.

## Why it matters

The project is built mostly by coding agents that read `AGENTS.md` first. Wrong
module descriptions send them to the wrong package, and missing packages make
them reinvent helpers that already exist (`ids`, `duration`, `canonicaljson`).

## Recommendation

1. Generate the package list in `AGENTS.md` and
   `.agents/context/architecture.md` from the package doc comments, or extend
   `TestModuleMapListsEveryPackage` to check these files too. Fix the operator
   description.
2. Split `DECISIONS.md` into one file per ADR,
   `documentation/adr/NNN-title.md`, each with a status (proposed, accepted,
   or superseded). Keep `DECISIONS.md` as an index.
3. Move `docs/design-v0`, `docs/design-v0.1`, finished plans, and closed audits
   into `docs/archive/`. Add a one-line status banner to every top-level folder
   README in `docs/` that says whether it is current or archived.
4. Rename the "Target Structure" heading to "Current Structure" and remove
   lines that are already covered by `modules.md`, so there is one source of
   truth.

## Done when

- A test fails when a new `internal/` package is not listed in `AGENTS.md`.
- Each ADR has its own file and can be linked directly.
- A newcomer can tell, from folder names alone, which documents describe the
  system as it is today.
