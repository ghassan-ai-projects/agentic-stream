# A-011 · `internal/situations/situations.go`

LOC: 597 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every dependency injected into `Engine` is used; identity prefixes come from `internal/ids`, not string literals.
- Every spec field the engine reads is honored; spec fields the engine ignores are rejected at compile time or documented as reserved.
- No dead statements, dead fields, or discarded parameters.
- A resolved occurrence can reopen per the declared `reopenCooldown`, or the field is removed and the one-shot lifecycle is explicit.
- Published `Version` values share no mutable state with caller-owned structures.
- Error paths from spec-authored values (durations, phase names) are surfaced, not defaulted.
- Package tests cover the occurrence open/close lifecycle and version materialization.

## Findings
- **MED F1. `idGen` dependency and `partitionID` field are dead** — `internal/situations/situations.go:30,32,167,175,177`. `Engine.idGen` is stored and never read (IDs are sha256-derived at lines 221-232), yet every `NewEngine` caller must supply a generator (14+ call sites). `Engine.partitionID` (line 30) is set and never read — `situationKey` uses `feature.PartitionID` (line 185) and `CurrentState` takes the partition as a parameter. Remove both; this is also why `ids.PrefixSituation` has zero references while `"sit_"`/`"occ_"` literals are hard-coded at lines 223 and 232.
- **MED F2. Resolved occurrences can never reopen; `reopenCooldown` silently ignored** — `internal/situations/situations.go:293-303,332-341`, `internal/spec/spec.go:140`. After `CloseWhen` fires, the entity's `Situation` sits at `Version > 0` and phase `resolved` forever: the open gate (line 332) requires `Version == 0 && Phase == initialPhase`, and nothing ever resets it, so the same entity can never experience a second occurrence. `Occurrence.ReopenCooldown` is parsed into the spec, validated by the schema, included in the digest, and then ignored — a spec author tuning reopen behavior gets a silent no-op. Either implement the declared reopen cycle or remove the field and make the one-shot lifecycle explicit in the schema.
- **MED F3. Dead statements and discarded context** — `internal/situations/situations.go:288,569`. `_ = feature` at line 288 is a leftover no-op (`feature` is used at lines 317-321); delete it. `evalBool` accepts `context.Context` and discards it (`_ = ctx`, line 569) — if cancellation will never apply to in-process CEL evaluation, drop the parameter.
- **LOW F4. Swallowed duration parse error** — `internal/situations/situations.go:320`. `minDur, _ := duration.Parse(tr.MinDuration)` silently treats an invalid authored duration as 0, making the transition immediate. The schema's duration pattern makes this unreachable through the compiler, but `Engine` accepts any `*spec.CompiledSpec` built in-process; surface the error or validate at compile time only and assert here.
- **LOW F5. Silent severity fallback for undeclared phases** — `internal/situations/situations.go:250-257,368-375,299`. `transition(sit, "resolved", ...)` hard-codes the closed-phase name; if a spec omits a `resolved` phase, `severityForPhase` silently returns 0 instead of failing. Validate at compile time that the close target is a declared phase.
- **LOW F6. Published `Version` aliases caller state** — `internal/situations/situations.go:201,458,467`. `sit.Facts["timer_provenance"]` is a shallow `cloneMap` of `feature.Metadata` (nested maps shared), and `materialize` returns `Facts: facts` referencing those values; the JSON snapshot is immutable, but the Go `Version` struct is not deep-copied. Deep-clone nested values in `cloneMap` or document `Version` as valid only until the next feature.
- **LOW F7. Snapshot timestamp encoding inconsistent with state encoding** — `internal/situations/situations.go:416-417` vs `482,489`. The snapshot uses `Format(time.RFC3339Nano)` without `.UTC()` while `stateJSON` normalizes with `.UTC()`. All current inputs are UTC so digests are stable, but the invariant should not depend on caller discipline; normalize both paths.
- **LOW F8. Thin package tests** — `internal/situations/situations_test.go` holds 3 tests (transition, completeness publish, nil-fact handling). The occurrence open/close lifecycle, `Restore`/`Reset` round-trip, and condition-start `MinDuration` gating are only covered indirectly via `internal/engine` tests.

## Checked, not an issue
- P1: errors wrapped with `%w`; CEL compile/eval errors surfaced; restore path re-parses `_event_time` facts to `time.Time` (`internal/engine/engine_restore.go:157-171`), so reducer semantics survive restart.
- P2: versions materialize as canonical JSON snapshots with SHA-256 digests (`materialize`, lines 402-470); spec digest is validated before publication (lines 395-401); raw event data never becomes instructions (only typed feature values enter CEL).
- P4: single reducer per partition, dependencies flow downward (spec, operators, canonicaljson, contractsv1, duration, ids); no per-domain branches.
- P5: exported symbols documented; `sort.Strings` on evidence before digest; state map keys canonicalized.
- P7: evidence sorted before digest; state document key set fixed; `ConditionStart` cloned into versions.
