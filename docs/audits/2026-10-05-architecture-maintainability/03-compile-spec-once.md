# 3. Compile the spec once into an executable plan

## Problem

`spec.CompiledSpec` is validated, but it still holds raw strings: durations,
CEL expressions, and operator kinds. The runtime packages parse and compile
these strings again, each in its own way and often on the hot path.

## Evidence

- **Durations are parsed at run time** in five places:
  `operators/operators.go:153`, `cognition/scheduler.go:214,219,231`,
  `situations/evaluate.go:98`, `engine/engine_heartbeat.go:34`, and
  `engine/engine_events.go:254`.
- **An error is ignored:** `situations/evaluate.go:98` calls
  `minDur, _ := duration.Parse(tr.MinDuration)`. A bad value becomes `0`, and the
  hysteresis check then passes immediately.
- **CEL is compiled for every evaluation:** `situations/evaluate.go:208,220`
  compiles and plans the expression on each call. `cognition/engine_cel.go:24`
  caches its programs, so the two packages behave differently.
- **There are two CEL environments:** `spec/cel.go:24` and
  `watch/expression.go:33`. Each declares its variables separately. If they
  drift apart, an expression can pass validation and then fail at run time, or
  the other way round.

## Why it matters

Determinism and "fail at validate time" are product goals. Every time a value
is parsed again at run time, a spec that `agentic-stream validate` accepted can
still fail, or quietly behave differently, during a live run. It also means
that every new spec field needs parsing code in more than one package.

## Recommendation

1. Have the spec compiler produce typed values that are ready to run, kept next
   to the canonical JSON. The digest still comes from the canonical JSON only:
   - `time.Duration` fields (`Debounce`, `Cooldown`, `ExpiresAfter`,
     `MinDuration`, `MaxOutOfOrderness`, window sizes);
   - compiled `cel.Program` values for transitions, triggers, and watch
     conditions, built from one `spec.CELEnvironment(kind)` factory;
   - typed operator kinds (see [item 4](04-operator-kind-registry.md)).
2. Change the consumers (`operators`, `cognition`, `situations`, `engine`,
   `watch`) so that they read these typed fields. Remove their own parsing.
3. Remove `watch`'s own CEL environment. Build it from the shared factory with
   the variable set that watches use.
4. Add a test that every duration and expression field in `schema.json` has a
   typed counterpart in the plan.

## Done when

- `duration.Parse` and `cel.NewEnv` are called only inside `internal/spec`.
- No spec-derived parse error is ignored anywhere.
- Golden replay hashes do not change, because this change only moves code.
