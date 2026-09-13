# A-017 · `internal/eventlog/log.go`

LOC: 419 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Schema validation enforces what it claims: every declared property's type (including array items and nested objects) is checked before an envelope enters `event_log`.
- No validation gap lets a declared-shape violation pass silently.
- Duplicate handling is explicit and reported (`-1` positions); appends are transactional per batch.
- Read path preserves global position order and reconstructs envelopes faithfully.

## Findings
- **[MED] F1. Hand-rolled schema validator checks only top-level scalar types** — `internal/eventlog/log.go:72-111` and `internal/eventlog/log.go:126-166`. The validator reads `properties[].type`/`enum`/`required`/`additionalProperties` but never inspects nested `items` or `properties` of object/array schemas: `{"type":"array","items":{"type":"number"}}` accepts `["x", 1]`, and object properties are unvalidated at any depth. `RequireSchemaValidation` gates ingress (`internal/ingress/jsonl.go`, `internal/ingress/live_socket.go`, `internal/replay/replay.go`), so a payload whose declared shape is violated only at depth enters the durable evidence log with a passing schema check. Either validate recursively or state the shallow contract in the function docs and at the gate sites.
- **[LOW] F2. Typeless schema properties silently pass** — `internal/eventlog/log.go:127`. `expected == ""` returns nil (no type check), and the combined condition `expected == "" || expected == "null" && value == nil` relies on implicit precedence. Fail (or skip with an explicit comment) on an empty type; split the condition.
- **[LOW] F3. ReadRequest.PartitionID zero value silently filters to partition 0** — `internal/eventlog/log.go:39-44` and `internal/eventlog/log.go:316-319`. `PartitionID >= 0` means "filter", so the zero value selects partition 0 rather than all partitions; only the undocumented `-1` sentinel reads everything. Document the sentinel on `ReadRequest` or make "all partitions" an explicit field/pointer.
- **[LOW] F4. Written payload_sha256 is never verified on read** — `internal/eventlog/log.go:247-299` vs `internal/eventlog/log.go:302-342`. `appendOne` computes and stores `payload_sha256`, but `scanRecord` rebuilds the envelope without comparing digests, so bit-rot or tampered rows are served silently to replay and operators. Storage's own evidence readers fail closed on digest mismatch (`VerifyStoredJSONDigest`); the event log can recompute and compare cheaply per row.

## Checked, not an issue
- P1: batch `Append` is a single transaction; per-row errors abort with positions discarded; all errors wrapped `%w`; contexts honored; duplicate handling is explicit (`ON CONFLICT DO NOTHING`, `-1` reported, documented).
- P2: the log stores raw evidence only — no execution semantics attach; `validateEnvelopeAgainstSchema` runs inside the append transaction against the durable `event_schemas` registry.
- P4: insert/read columns match migrations 001 + 007 (`tracestate`); position is the AUTOINCREMENT primary key so read order is authoritative global order.
- P5: exported symbols documented; clock injection (`NewEventLogWithClock`) keeps `created_at` replay-safe; stdlib usage clean.
- P6: `log_test.go` covers append/read round-trip with trace context and duplicate `-1`; `schema_test.go` covers the validation surface.
