# U03 — Delete the eventlog gap writer and the map quarantine wrapper

Status: done · Decision: **delete** · Priority: P2 · Size: S

## Finding

- `EventLog.RecordGap` → `Service.RecordGap` → `Store.RecordGap`, plus
  `domain.Gap.Valid`: a public "record a durable discontinuity caused by
  bounded overflow" API with no production caller.
- Ingress has no bounded-overflow path. The live socket applies backpressure
  (`internal/ingress/internal/transport/server.go`) and never drops evidence.
- `event_gaps` *is* written in production, by a different path:
  `Unit.InsertOverflowGap` when a quarantine record exhausts its retries
  (`internal/eventlog/internal/store/quarantine.go:74`).
- `EventLog.Quarantine(map[string]any)` is test-only; production uses
  `QuarantineEnvelope` and `QuarantineRaw`.

## Decision and reasoning

Delete the generic `RecordGap` API and the `Gap` type, and keep
`InsertOverflowGap`. A public writer for a situation that cannot happen invites
a future caller to record gaps with made-up positions. If a dropping source
appears later (an ADR-015 bridge, for example), its gap semantics should be
designed with it.

Delete `EventLog.Quarantine` (the map form); its tests switch to
`QuarantineEnvelope`/`QuarantineRaw`, which is what production calls.

`event_gaps` stays write-only until [U16](U16-quarantine-operator-commands.md)
shows exhausted-retry gaps in `quarantine list`.

## Done when

- Five symbols are gone from `deadcode ./...`: `EventLog.Quarantine`,
  `EventLog.RecordGap`, `Service.RecordGap`, `Store.RecordGap`, `Gap.Valid`.
- The eventlog README and UBIQUITOUS_LANGUAGE no longer describe a general gap
  API.
