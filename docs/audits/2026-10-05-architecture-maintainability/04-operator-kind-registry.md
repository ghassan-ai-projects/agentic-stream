# 4. Register operator kinds in one place

## Problem

Operator kinds are plain strings. Several `switch` statements in four packages
check them, so adding a new operator means finding and editing every one of
these places.

## Evidence

| Place | What it does with the kind |
| --- | --- |
| `internal/spec/schema.json:262,294` | enum of allowed kinds |
| `internal/operators/operators.go:227` | dispatches `aggregate`/`slope` vs `missing_heartbeat` |
| `internal/operators/samples.go:74` | decides which quality checks apply |
| `internal/operators/heartbeat.go:107` | filters to `missing_heartbeat` |
| `internal/operators/window.go:161` | maps aggregate functions (`slope`, …) |
| `internal/operators/operators.go:85` | window kind `tumbling`/`sliding` |
| `internal/situations/evaluate.go:184` | special-cases `missing_heartbeat` |
| `internal/engine/engine_heartbeat.go:23` | special-cases `missing_heartbeat` for timers |

`situations` and `engine` should not need to know which operators exist, yet
both check for the `missing_heartbeat` kind by name.

## Recommendation

1. Add a typed `operators.Kind` with constants, and one small interface per
   behavior that differs between kinds:

   ```go
   type Behavior interface {
       Admits(env contractsv1.Envelope, input InputContract) bool
       Apply(blob *OperatorStateBlob, env contractsv1.Envelope, at Instant) ([]Feature, error)
   }
   // TimerSource is implemented by kinds that schedule wall-clock timers.
   type TimerSource interface{ TimerDelay() time.Duration }
   ```

2. Add one registry, `map[Kind]Factory`, built from a literal in
   `operators`. Every `switch inst.def.Kind` becomes a call through the
   registry.
3. `engine` and `situations` ask the operator what it can do ("is it a
   `TimerSource`?", "does it produce absence facts?"). They no longer compare
   the kind string.
4. Add a parity test: the kinds in `schema.json` must equal the keys of the
   registry. The spec compiler rejects a kind that is not registered.

Determinism: the registry is a fixed map built at compile time, and the
iteration order over operators still comes from the spec. Replay output does
not change.

## Done when

- A new operator kind is one new file in `operators` plus one enum entry in
  `schema.json`. The parity test catches a missing entry.
- `grep '"missing_heartbeat"'` finds matches only in `operators` and the
  schema.
