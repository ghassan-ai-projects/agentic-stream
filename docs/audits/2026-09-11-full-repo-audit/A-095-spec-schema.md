# A-095 · `internal/spec/schema.json`

LOC: 752 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- F1: every field the runtime reads from an authored spec is allowed by the schema.
- F2: every value the schema admits for operator/window/aggregate/reducer enums is implemented by the runtime.
- F3: schema-defined top-level sections are consumed by the runtime.

## Findings
- **[HIGH] F1. `executor.skills` is implemented by the compiler types, assembler, wire, and worker — but rejected by the schema** — `internal/spec/schema.json:598-659` (executor def, `additionalProperties: false` at `:600`, no `skills` property). `spec.Executor.Skills` (`internal/spec/spec.go:184`) is decoded and used by `internal/episodes/assembler.go:226` and `internal/episodes/worker_executor.go:585` to emit `skill_refs_json` (proto `runtime-v1.proto:123`, P5/§B6-B9). Because the compiler validates against this schema (`internal/spec/compiler.go:146`), any authored spec declaring `executor.skills` fails compilation — the P5 skill path is unreachable from YAML and is exercised only by tests that construct `CompiledSpec` directly (false confidence). Fix: add a `skills` property (array, maxItems bounded, items: `{name, tree_sha256}`) to the executor def.
- **[MED] F2. Schema admits operator/window/aggregate values the runtime rejects at startup** — `internal/spec/schema.json:203,284-293,320-337`. Window kinds `count`/`decay` fail in `newWindowConfig` (`internal/operators/operators.go:80-99`); operator kinds `map`/`filter`/`rate`/`correlation`/`duration` fail in the kind switch (`operators.go:175-189`); aggregates `variance`/`stddev`/`first`/`last`/`delta`/`rate`/`quantile`/`correlation` fail in `computeAggregate` (`operators.go:592-635`). Schema-valid specs compile, pass the digest, then hard-fail at engine build. Fix: narrow the enums to the implemented sets or implement the advertised surface.
- **[MED] F3. Schema admits reducer strategies the runtime silently ignores** — `internal/spec/schema.json:412-420`. Only `latest_event_time` and `set_union` are handled (`internal/situations/situations.go:263-281` and `:524-531`, both switch statements without a default); `min`/`max`/`bounded_append`/`weighted_confidence`/`source_priority` are schema-valid yet silently reduce nothing — facts missing from published versions with no error. Fix: reject unimplemented strategies at compile time or implement them.
- **[LOW] F4. `retention` and `telemetry` sections are digested but never consumed** — `internal/spec/schema.json:747-779`. Parsed into `CompiledSpec.Retention`/`Telemetry` (`internal/spec/spec.go:251-262`) and folded into the digest, but no runtime code reads them (retention windows unenforced; `RecordModelDeltas`/`TraceSampleRatio` unused). Dead authoring surface: remove from the schema until enforced, or wire them.

## Checked, not an issue
- S1 (compiler consistency): required fields and shapes match the compiler's reads — `input.schema` ↔ `SchemaRef` (validated against `eventschema.Lookup`, `compiler.go:252-258`), `timePolicy`, `window`, `operator`, `situation`/`occurrence`, `trigger` (incl. required `materialDelta`), `budget` (9 fields), `intent` (incl. `presets`, `modelWritableFields`, `rateLimitPerHour` ↔ migration 022, `policy` default `approval` ↔ `compiler.go:227-231`), `dispatchPolicy` default `shadow` ↔ `compiler.go:238-240`, `riskCeiling` default `R1` ↔ `compiler.go:232-234`, input classification/maxPayloadBytes defaults ↔ `compiler.go:209-216`.

## Resolution

F1 is fixed by adding the bounded `executor.skills` array and a strict
`skillRef` definition requiring a name and a lowercase 64-hex-character
`tree_sha256` digest. The compiler now accepts the authored path used by the
assembler and worker instead of rejecting it at JSON Schema validation.

F2 is fixed by narrowing the schema to the implemented operator surface:
`tumbling`/`sliding` windows, `aggregate`/`slope`/`missing_heartbeat` operators,
and the eight aggregates implemented by `computeAggregate`. The unsupported
window, operator, and aggregate values are no longer schema-valid.

F3 is fixed by narrowing reducer strategies to the two strategies handled by
both reducer application and feature-map construction: `latest_event_time` and
`set_union`. Unsupported strategies can no longer silently publish incomplete
Situation facts from an authored spec.

Focused compiler coverage proves one digest-pinned skill compiles and that
count windows, map operators, variance aggregates, and max reducers are
rejected. `go test ./...`, `go vet ./...`, `git diff --check`, and direct
SQLite contract validation pass in the isolated worktree.

F4 is fixed by removing the unenforced `retention` and `telemetry` sections
from the v1 authoring schema and compiled representation. The three canonical
examples were updated accordingly, and the compiler now uses strict YAML
field decoding so an old or misspelled section is rejected instead of silently
dropped from the digest. `TestCompileRejectsUnenforcedTopLevelControls` proves
both removed sections fail closed.
- S2: no redundant or dead per-field definitions within the defs; conditional window requirements via `allOf`/`if` are correct for the kinds the runtime supports (tumbling/sliding).
- S3: not a domain-data file; `additionalProperties: false` throughout keeps authoring surface explicit.
