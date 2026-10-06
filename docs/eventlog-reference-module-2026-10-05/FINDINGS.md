# Findings

Survey of `internal/eventlog` (7 production files, ~800 lines) against the
reference-module principles.

## Mixed responsibilities

- `EventLog` is simultaneously the public facade, the configuration holder
  (`requireSchemas` flag), and the owner of SQL: `append.go`, `quarantine.go`,
  `read.go`, `entity_window.go` and `log.go` interleave envelope admission,
  schema checking, SQL and lifecycle orchestration in one flat type.
- `schema_validation.go` mixes the durable schema load (SQL against the
  `event_schemas` registry owned by `internal/eventschema`) with pure payload
  checking rules (declared fields, JSON types, enums, required fields).
- `quarantine.go` mixes pure identity derivation (payload digests, stable
  quarantine IDs, header projection) with the upsert/conflict/overflow SQL and
  the release/redrive lifecycle sequencing.
- `read_record.go` mixes row scanning (codec) with time parsing and envelope
  rebuilding (pure decoding).

## Cross-module data access

Read-only SELECT against `event_schemas`, owned by `internal/eventschema`
(`schema_validation.go:64`). Correct direction (no writes); moves to the store
layer as a named read.

## Durable ownership (verified clean)

`event_log`, `event_quarantine` and `event_gaps` are owned by this module in
`durableOwners`; the quarantine overflow path is the only writer of
`event_gaps` besides explicit `RecordGap`. Writes stay through the store layer;
no ownership table changes.

## Public surface

Used by 40 files across ingress (append/quarantine/read/schema validation),
engine (read/positions), replay (validation log + clock log), runtime
transport (sources), evidence, api and soak: `EventLog`,
`NewEventLog`/`NewEventLogWithClock`, `Record`, `LogPosition`, `ReadRequest`,
`Read`, `Append`, `CurrentPosition`, `RequireSchemaValidation`,
`ValidateEnvelope`, `Quarantine`/`QuarantineEnvelope`/`QuarantineRaw`,
`ReleaseQuarantine`, `RedriveQuarantine`, `RecordGap`, `EntityWindow`,
`EntityEvent`, `ReadEntityWindow`. The facade keeps every symbol and
signature; `RequireSchemaValidation` keeps its chainable return.

## Vocabulary drift

- "admit" is used for envelope + schema validation, not admission in the
  episode sense; the language guide renames the concept evidence admission.
- `storedEvent`/`eventBody`/`quarantineRecord` are untyped-ish internal
  records; they become domain records named after their meaning.

## Smaller defects

- `ReadEntityWindow` takes a raw `*storage.DB` instead of the module facade,
  forcing callers to hold a database handle.
- `ValidateEnvelope` silently passes when schema validation is not required —
  documented fail-open behavior that stays, but belongs to an explicit domain
  rule with a test.

## Non-findings (verified clean)

- No clock reads outside `clk.Now()` at insert time; no network or file I/O;
  lint-clean with files and functions under the bar.
- Bounded retry (10), hash-conflict rejection, duplicate-ignore (`-1`
  positions) and rollback-on-later-failure behavior are pinned by
  `boundaries_test.go`, `quarantine_test.go`, `log_test.go`,
  `schema_test.go` and `entity_window_test.go`.
