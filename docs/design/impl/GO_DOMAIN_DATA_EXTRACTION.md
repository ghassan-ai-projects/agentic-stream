# Go-side domain knowledge → data (agentic-stream) — implementation plan

Status: **implemented** — T1–T3 shipped on branch `smarter-agent` (intent-catalog
parity digest `e4f86620…` unchanged). Plan-review gap/completeness findings
integrated (32 refs not
"4 groups" — `sensor.temperature/1.0` is a 5th standalone ref; `numericWithUnit`
synthesizes the `unit` field (Type string, Optional true) on the 19 numeric
refs; the `mode` special-case writes a SECOND field on top of the switch
default, and heartbeat must NOT fall to default — both stay machinery with a
data sentinel; simulator_test covers only 4/25 channels, so the rewrite adds
mode/heartbeat/unknown/unit cases; the Go intent-catalog data file is a
SUPERSET of tamoz's aquaculture.json (Go schemas carry binding fields +
Policy/RateLimit/Description/ModelWritableFields/Presets the Ruby file lacks)
— there is NO shared shape to reuse, only the parity digest binds the two;
`conformance.go` is a non-test file re-authoring a 1-entry ticket fixture
(deliberate, kept with justification); the route/rule constants
(`install_watch_condition` in validator.go/watch_effector.go/composite_effector.go)
are machinery branches, carved out of the zero-knowledge bar).

## The problem

The Ruby side (tamoz) extracted all domain knowledge to
`test/fixtures/domains/*.json`. The Go side (agentic-stream) still embeds it:

- `internal/eventschema/registry.go:27-63` — the PRODUCTION `builtins` map:
  32 event-schema refs (motor, sensor, pump, pond, bay) as a Go map literal;
  the spec compiler consumes it via `eventschema.Lookup`/`JSON`.
- `internal/ingress/simulator.go:238-279` — the channel→field mapping switch
  (20 cases / 25 channel names) + the mode special-case + unit passthrough.
- `internal/episodes/intent_catalog_test.go:24-76` — the FULL aquaculture
  intent catalog (22 entries, risks, compensation, watch schema, binding
  fields) re-authored in Go test code — a superset copy of the Ruby data.
- `internal/executor/conformance/conformance.go:25` — a non-test file with a
  deliberate 1-entry ticket fixture.

The owner's directive (same as Ruby): domain knowledge becomes DATA, not
code. Go is architecturally different — the registry is PRODUCTION runtime,
so this is a bigger change: the schema catalog must still be available at
runtime, now loaded from embedded data.

## Bar (finished line)

1. **Zero domain DATA in Go code.** The event schemas, the simulator
   channel→field mapping, and the intent-catalog test data live in JSON data
   files. Go code keeps machinery only (Lookup/JSON over loaded data; the
   simulator's switch becomes a map lookup with the mode/heartbeat
   special-cases as machinery; the test builds intents from loaded data).
   Carved out (machinery, not data): the `install_watch_condition` route/
   rule constants in validator.go / watch_effector.go / composite_effector.go
   (branch conditions, not catalog rows).
2. **Behavior byte-identical.** `eventschema.Lookup`/`JSON` return identical
   results for all 32 refs — proven by the full-catalog load test (every ref
   loads, count pinned at 32) PLUS a golden digest over the canonicalized
   loaded registry (a typo in ANY ref's event_type/unit/field values breaks
   the digest even in entries the targeted tables do not cover) PLUS
   per-field structural invariants (every field is typed or
   numeric-with-unit; the unit field is a string-typed optional). The
   simulator produces identical envelopes — proven by the
   mode/heartbeat/unknown/unit test cases (the unit case passes a real
   `unit` key and asserts `data["unit"]`). The intent-catalog parity
   digest `e4f86620…` still matches the Ruby side.
3. **Data files live IN this repo** (hermetic `go test`); the parity digest
   is the only cross-repo binding.
4. All affected Go suites green (`go test ./...`).

## Change

### T1 — Event schema registry → embedded data
- `internal/eventschema/registry_data.json`: all 32 refs — ref-keyed object,
  each with event_type, schema_version, fields {path, unit, type, optional}.
  The `unit` field on numeric refs is explicit in the data (the loader
  unmarshals verbatim; `JSON()` output byte-identical because Type "" →
  "number" and the unit field stays Type "string" Optional true).
- `registry.go`: `builtins` map literal → `//go:embed registry_data.json` +
  unmarshal into a `map[string]Definition` (struct fields get json tags or
  the loader uses a tagged DTO — encoding/json matches case-insensitively,
  so lowercase JSON keys work). `Lookup`/`JSON` stay; `numericWithUnit` is
  REMOVED — the unit field is now explicit data, and `JSON()` still coerces
  an empty Type to "number". `Lookup`
  fails loudly (panic with the wrapped decode error) if the embedded data
  is corrupt, rather than reporting every ref as "unknown".
- **Full-catalog test**: `TestAllBuiltinsLoadFromData` iterates every ref
  from the embedded data (count pinned at 32), pins a golden digest over the
  canonicalized loaded registry (catches any value typo — event_type, unit,
  field path — in uncovered refs), and enforces per-field structural
  invariants (typed or numeric-with-unit; unit is a string-typed optional) —
  closing the 18/32 coverage gap.

### T2 — Simulator channel→field mapping → data + machinery split
- The 20-case switch becomes a channel→field map from a data file
  (`simulator_data.json` or in the registry data). The map is
  channel-name → target field; heartbeat maps to a sentinel (empty string →
  emit nothing), mode is EXCLUDED from the map (the existing post-switch
  block writes the second `mode` field as machinery, and the default
  `data["value"]` fallback handles mode's value key).
- The unit passthrough (all channels incl. heartbeat) and the mode block
  stay machinery.
- **New test cases**: mode (expects `{value, mode}`), heartbeat (expects
  empty data), unknown channel (expects `data["value"]`), unit passthrough.

### T3 — Intent catalog test → loaded data (Go superset)
- `internal/episodes/testdata/aquaculture_intents.json`: the 22 rows
  (type, risk, compensation) AND the schema-builder DATA — actionSchema's
  4 properties, watchSchema's 9 properties (incl. the binding fields +
  `["string","number"]` arrays), compensationSchema's note/priority — plus
  the construction constants Policy "automatic", RateLimitPerHour 60, and
  the watch preset. This is a SUPERSET of tamoz's aquaculture.json (the
  Ruby file lacks the binding fields and construction constants) — no
  shared shape, only the parity digest binds the two.
- The test loads the JSON, keeps `targetTypes` derivation + the compile loop
  as machinery, and asserts the pinned digest `e4f86620…` (unchanged).
  Description interpolation, ModelWritableFields ["hypothesis"], and the
  {"default": {}} preset shape are deliberately NOT in the JSON — they stay
  test machinery, mirroring tamoz's `DomainLoader.intent_entry`, which
  derives the same three from intent_types on the Ruby side. Any drift in
  these constants changes the pinned digest and fails BOTH repos' tests, so
  they are gated, not silent.
- `conformance.go`'s 1-entry fixture: KEPT (deliberate minimal fixture),
  stated.

### T4 — Test fixtures stay (classified)
- validator_test / simulator_test / composite_effector_test / parity_p1_test
  literals are shape-asserting test data or frozen digest vectors — kept,
  stated.

## Phase bar per finding

T1 = full-catalog test green over the loaded data; registry_test +
compiler_test + deployments_test green; no schema literal in registry.go.
T2 = simulator_test green with the new mode/heartbeat/unknown/unit cases;
envelopes identical. T3 = parity test green with the loaded data; the test
file has no inline CATALOG literal (the 22 rows, the three schemas, and the
construction constants Policy/RateLimit/watch preset come from JSON; only
the derivation rules mirroring the Ruby loader remain). Finished line:
`go test ./...` green;
grep shows no domain schema/catalog literals in the three files; commit.

## Cross-repo note

The tamoz side is committed (86e8dce). The Go data files are a parallel
superset copy pinned by the parity digest — a deliberate duplicate with a
catching gate, not a silent second source. T1/T2 data have NO Ruby
counterpart (the event schemas and simulator mapping are Go-only) — those
files become the single source of truth within Go. One shared vocabulary to
coordinate: the simulator's channel→field targets are field paths declared
by registry_data.json (an adapter projection — the mapping is new
information, the field vocabulary is not). Drift on either side is caught
at ingestion by eventlog's rejection of undeclared payload fields, and the
intent-catalog digest gates the episodes side; no test currently binds the
two files' field vocabularies directly.
