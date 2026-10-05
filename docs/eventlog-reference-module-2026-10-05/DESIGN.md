# Design

The event log owns three durable tables and their rules, so it takes the
authority shape: facade · app · domain · store. There is no transport layer —
the module owns no file, socket or process resource; callers hand it the
database handle.

## Layers

| Layer | Owns | Must not |
| --- | --- | --- |
| Facade `internal/eventlog` | `EventLog` with the current method set, constructors, `ReadEntityWindow`, and type aliases; one documented line per operation | Contain logic, SQL or transactions |
| App `internal/eventlog/internal/app` | Use cases: append batch (admit → encode → insert, one unit of work, rollback on later failure), quarantine (derive identity → upsert → conflict → overflow gap), release, redrive (load released → admit → append → mark, one unit), read streaming, current position, gap recording | Import `database/sql`, `internal/storage`, `net`, or open files |
| Domain `internal/eventlog/internal/domain` | Vocabulary (`Record`, `LogPosition`, `ReadRequest`, `EntityWindow`, `EntityEvent`) and pure rules: registered-schema payload checking (declared fields, JSON types, enums, required), quarantine identity derivation, payload conflict decision, encoded event columns, stored-record decoding, gap and quarantine input validation | Any I/O, clock reads, `database/sql`, `os`, `net`, storage |
| Store `internal/eventlog/internal/store` | Every SQL statement and transaction: event insert with duplicate-ignore, record queries, current position, quarantine upsert/conflict/overflow/release/redrive, gap insert, schema load, entity window scan; a `Unit` type gives app caller-owned transaction plumbing | Decide admission, derive identities, or parse schemas |

Dependency levels: domain 1 (imports only `contractsv1`), store 2 (storage +
domain + contractsv1), app 3 (domain + store + contractsv1 + clock), facade 4
(clock + storage for constructors + the inner layers). Everything stays above
the existing importers' expectations; `durableOwners` is unchanged.

## Public API (unchanged)

`EventLog` (Append, Read, CurrentPosition, Quarantine, QuarantineEnvelope,
QuarantineRaw, ReleaseQuarantine, RedriveQuarantine, RecordGap,
RequireSchemaValidation, ValidateEnvelope), `NewEventLog`,
`NewEventLogWithClock`, `Record`, `LogPosition`, `ReadRequest`,
`EntityWindow`, `EntityEvent`, `ReadEntityWindow`.

## Rules every operation follows

1. A batch append is one transaction: each envelope is admitted (contract,
   then registered schema when required) and inserted in order; a later
   failure rolls back earlier inserts; duplicate event ids insert nothing and
   report position -1; trace context is validated before the duplicate check.
2. Quarantine identity is derived from the declared event id (or a payload
   digest when absent) plus the payload bytes; the same payload under the same
   id counts one bounded retry (max 10), a different payload is a hash
   conflict that rejects the record.
3. Overflow (retries spent) records exactly one `event_gaps` row; quarantine
   never deletes evidence.
4. Redrive loads only a released, not-yet-redriven record, re-admits it
   through the same rules as append, appends it, and marks it redriven — all
   in one transaction, exactly once.
5. Schema validation is opt-in (`RequireSchemaValidation`) and fail-open only
   in that unvalidated logs accept contract-valid envelopes; once required, an
   unregistered schema or violating payload rejects the append.
6. Reads stream in log order with the tenant scope and limit rules preserved;
   the entity window is read-only, event-time ordered and stoppable.

## Enforcement

| Guarantee | Gate |
| --- | --- |
| Downward-only imports | `packageLayers`, `allowedImports`, stale-edge test |
| Domain purity | `TestDomainPackagesArePure` |
| App infrastructure isolation | `TestApplicationLayersDoNotTouchInfrastructure` |
| SQL only in store | `TestModuleSQLStaysInStore` |
| Durable ownership unchanged | `TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase` |
| Behavior preserved | The existing package test suite runs unchanged against the facade |

## Schema / wire changes

None. No migrations, no envelope or digest changes, no storage-format edits.
