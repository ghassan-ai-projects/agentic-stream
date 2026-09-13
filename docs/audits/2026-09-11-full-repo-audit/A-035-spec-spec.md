# A-035 · `internal/spec/spec.go`

LOC: 272 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every field declared on a spec type and accepted by `internal/spec/schema.json` is consumed by the runtime, rejected by the compiler, or explicitly marked reserved.
- No declared configuration knob is silently a no-op after a successful compile.

## Findings
- **MED F1. Twelve schema-accepted spec fields are dead configuration** — `internal/spec/spec.go:77-82,91-92,102,105-107,110,133,140,146`. Verified by repo-wide grep (zero production references outside this file): `TimePolicy.WatermarkStrategy`, `TimePolicy.IdleTimeout`, `TimePolicy.ClockSkewTolerance` (lines 77-82; only `MaxOutOfOrderness` and `AllowedLateness`/`LatePolicy` are consumed, e.g. `internal/engine/engine_events.go:197`), `Window.Count` and `Window.HalfLife` (lines 91-92), `Operator.Fields`, `Operator.Where` (validated as CEL in `cel.go:71` but never applied as a filter by `internal/operators`), `Operator.Configuration` (lines 105-110), `Reducer.Limit` (line 133), `Occurrence.ReopenCooldown` (line 140), `Situation.EntityKey` (line 146), and `Input.PartitionKey` (line 69; partitioning derives from the envelope, not the spec). Each is schema-valid, part of the compiled digest, and ignored at runtime — an author setting them gets silently different behavior from the documented intent (e.g. `where` filters change aggregate inputs; `reopenCooldown` changes occurrence lifecycle). Fix: implement, reject with a "not supported in v1" compile error, or move to an explicit `reserved` block excluded from the schema until implemented.
- **LOW F2. `DeltaKeys` is a mutable package-level struct** — `internal/spec/spec.go:35-53`. The delta key names are effectively constants but declared as a writable `var` of anonymous struct type; any package can mutate them process-wide. Export them as typed constants or an immutable struct.

## Checked, not an issue
- P3: package-level `CompileFile` (lines 10-12) is a used convenience wrapper (9+ call sites), not duplication; `EffectiveWatchConfidenceFloor` (lines 243-248) is consumed by the episode assembler and tested.
- P5: all exported types documented; yaml+json tags kept in sync across every field; `CompileError` is a structured, documented diagnostic.
- P4: pure type declarations only, no logic, no domain branches; dependencies flow downward (no imports besides stdlib).
- P7: type declarations carry no behavior that could diverge across runs; digest stability is governed by the compiler's normalization.
