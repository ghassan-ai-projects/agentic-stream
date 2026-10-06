# Findings

Survey of `internal/replay` (10 production files, ~1,300 lines) against the
reference-module principles, with file references.

## Mixed responsibilities

- `hashes.go` is the worst offender: trace-file scanning (`traceEpoch`,
  `os.Open`), envelope validation policy, pure earliest-time selection, the
  situation-version SQL query, the pure version hash, and the `RunNTimes`
  orchestration with its own temp-dir lifecycle all live in one file.
- `session.go` mixes use-case sequencing (prepare → ingest → run → collect)
  with engine construction policy (`newReplayEngine`) and the record-time clock
  rule (`advanceToRecordTime`).
- `schedule.go` mixes the `scheduler_items` SQL scan with timestamp parsing and
  the admission-window eligibility decision (`expires.After(admitAt) &&
  !admitAt.After(now)`).
- `recorded.go` and `shadow_input.go` interleave pure digest/schema verification
  with `situation_versions` SQL lookups.
- `shadow.go` mixes validation compilation (intent catalog, policy digest),
  pair execution through caller ports, and comparison persistence.
- `capabilities.go` mixes the mode dispatch decision with the cross-module
  worklist SQL (`scheduler_items` join `episodes` join `situation_versions`).

## Layer-less flat package

The package has no internal layers: SQL, file I/O, caller-supplied ports, pure
rules and orchestration sit side by side, so no architecture gate can pin any
of them. This is the primary defect the migration fixes.

## Public surface versus production use

- Production callers use exactly one symbol: `replay.Run` (`cmd/agentic-stream/main.go:127`).
- `RunMode`, `Capabilities`, the capability ports, `RunNTimes`,
  `NewDeterministicBaseline`, `AllHashesEqual` and the mode constants are used
  only by the package's own tests today.
- Verdict: keep them. Shadow and counterfactual replay are named release
  acceptance items in `docs/design/README.md` ("run a new model or prompt
  against the same trace in effect-disabled shadow mode"); the surface is the
  documented product contract awaiting CLI wiring, not accidental API. Wiring
  the modes into the CLI is recorded as a deferred follow-up.

## Cross-module data access

Read-only SELECTs against tables replay does not own: `situation_versions` and
`situations` (engine), `scheduler_items` (scheduleledger), `episodes`
(episodeledger) — in `capabilities.go:42`, `hashes.go:26`,
`recorded.go:155`, `schedule.go:57`, `shadow_input.go:17`. Writes already go
through owner APIs (`episodes.Assembler.Persist`, `qualification.
ShadowComparisonStore.Record`) inside replay-owned transactions — the correct
handoff, preserved as-is and moved into the store layer.

## Decisions stranded beside I/O

Pure rules that deserve domain tests but currently share files with SQL or file
I/O: capability and mode admission (`modes.go`), recorded-entry completeness /
provenance / digest verification (`recorded.go`), decision↔episode matching
(`recorded.go`), shadow manifest/decision binding and validation precedence
(`shadow_validation.go`), difference computation and comparison sealing
(`shadow_comparison.go`), admission-window eligibility (`schedule.go`),
counterfactual command admission (`counterfactual.go`), epoch selection
(`hashes.go`), the deterministic baseline policy (`baseline.go`), and the
ordered version hash (`hashes.go`).

## Vocabulary drift

- `replayItem` (private) versus `ReplayEpisode` (public projection) for the
  same worklist identity; the SQL side calls it "replay items".
- `caps` abbreviates `Capabilities` throughout.
- `tamoz` names the model-under-test side of a shadow trial; it is product
  vocabulary and stays, but deserves a language entry.
- `executableSchedulerItems` describes storage, not the domain decision
  (admission readiness).

## Smaller defects

- `run()`'s `after` callback threads `*storage.DB`, `*spec.CompiledSpec` and a
  clock read through a function type — an untyped escape hatch that the app
  layer should own as an explicit capability phase.
- `shadowEntityID` decodes the snapshot again after `loadShadowInput` already
  canonicalized it; harmless but duplicated parsing.
- `Result.SimulatedResults` remains `[]map[string]any` (simulator port
  contract); typing it is deferred until the simulator contract needs it.

## Non-findings (verified clean)

- No secrets, no network, no clock reads outside the virtual clock, no effect
  imports; `forbiddenImports` already blocks `internal/actions` and
  `internal/runtime`.
- Golden digests, canonical bytes and error strings are pinned by
  `replay_test.go`, `shadow_comparison_test.go`,
  `extraction_boundaries_test.go` and `thermal_chamber_test.go`.
