# Package ratings (2026-10-06, after the facade + domain migration)

Judgment, not tool output. Inputs: per-layer coverage, untyped `map[string]any`
count in production code, unwired code, foreign-table SQL and layering. Score out
of 10 with the main limiter.

| Package | Score | Main limiter |
| --- | --- | --- |
| `actionport` | 9 | none significant |
| `sources` | 9 | none significant |
| `canonicaljson` | 9 | none significant |
| `control` | 8.5 | reads `episodes` and `cost_reservations` in the kill path |
| `approvalledger` | 8.5 | reads `intents` (policy) |
| `evidence` | 8.5 | none significant |
| `operators` | 8.5 | none significant |
| `engine` | 8.5 | `RunGlobal` re-reads the log from position 0 on every call |
| `interlock` | 8 | `Set` has no production caller |
| `storage` | 8 | `DB` embeds `*sql.DB`, so every method leaks through the facade |
| `telemetry` | 8 | `Runtime` mixes counters, tracer and span helper |
| `worker` | 8 | reference server lives in the production package |
| `api` | 8 | SSE logic tied to `net/http`, no app layer |
| `ingress` | 8 | 27 untyped maps (simulator records) |
| `notify` | 8 | `Prune` never scheduled |
| `watch` | 8 | none significant |
| `runtime` | 8 | wide composition |
| `executor/remote` | 8 | domain depends on generated protobuf types |
| `authority` | 8 | 43 untyped maps |
| `episodeledger` | 8 | owner-lease time encoding differs from `control` |
| `contractsv1` | 8 | 11 exported symbols nobody uses |
| `spec` | 8 | 9 maps; still holds the event-schema registry |
| `decisions` | 8 | 58 untyped maps |
| `situations` | 7.5 | 37 maps |
| `eventlog` | 7.5 | quarantine release/redrive and `RecordGap` unwired (14 symbols) |
| `episodes` | 7.5 | 81 maps, foreign reads |
| `cognition` | 7.5 | 28 maps, foreign reads |
| `policy` | 7.5 | 31 maps, foreign reads |
| `actions` | 7.5 | foreign reads, provider results untyped |
| `device` | 7.5 | 2.9k lines, 53 maps |
| `runartifact` | 7.5 | reads tables it does not own, untyped rows |
| `executor/native` | 7.5 | facade is mostly aliases; batch runner unwired |
| `replay` | 6.5 | about 93 symbols unwired (shadow, recorded, counterfactual, baseline), 37 maps |
| `executor/fixture` | 5 | no domain layer, 14 maps, used by production demo mode |

## Common weaknesses, in order of impact

1. Foreign-table reads (owner read ports pending).
2. Untyped documents (`map[string]any`); typed records parsed once at the boundary
   would fit, keeping the original bytes where a digest depends on them.
3. Operator levers with no production caller (owner decision pending).
4. Facades that are mostly aliases (`executor/native`, `storage`).

Coverage is not a limiter: every layer is at or above 60%.

## Typed-records rounds

Tracked in [TYPED_RECORDS.md](TYPED_RECORDS.md), one commit per round.
