# A-015 · `internal/cognition/engine.go`

LOC: 446 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every struct field of an exported type is read somewhere in the repo.
- Function signatures carry only parameters the function uses; sibling functions evaluating the same inputs share one input construction.

## Findings
- **[MED] F1. `Evaluation.PolicySHA256` is a write-only field** — `internal/cognition/engine.go:49,221` (also set at `internal/cognition/reconsideration.go:135`). `saveEvaluation` ignores it and recomputes the digest from `s.spec.Digest` (`internal/cognition/scheduler.go:129-132`), so the field is never read anywhere (repo-wide grep: only the two assignments plus unrelated same-named fields in `internal/storage/shadow_comparison.go` and `internal/replay/replay.go`). Dead data on an exported type that implies a binding that does not exist. Fix: delete the field, or make `saveEvaluation` consume `eval.PolicySHA256` (and fail if empty) so the value actually flows.
- **[LOW] F2. `evalBool` accepts and discards `ctx`** — `internal/cognition/engine.go:381-382` (`_ = ctx`), while `evalScore` (418) takes no ctx. Inconsistent signatures plus an explicit dead assignment. CEL `Eval` is synchronous; drop the parameter from `evalBool`.
- **[LOW] F3. Duplicated CEL evaluation-input map** — `internal/cognition/engine.go:397-403` vs `426-432`. `evalBool` and `evalScore` each build the same five-variable input literal; adding a CEL variable to one silently breaks the other at eval time. Extract one `evalInput(features, situation, delta, eventTime, watermark)` helper used by both.

## Checked, not an issue
- P1: all errors wrapped with `%w`; no swallowed errors; contexts passed to all SQL calls.
- P2: trigger expressions are compiled spec CEL, evaluated against built features/deltas only — event content never becomes instructions; no model invocation here.
- P3: no unused exported symbols (`Engine`, `Evaluation`, `NewEngine` all used by `internal/cognition` callers and tests).
- P4: deterministic scheduler, no LLM per event; operator-kind defaults are generic operator classes, not domain branches.
- P5: CEL via cel-go is the rule engine; programs compiled once at construction; canonical JSON used for the delta digest.
- P6: `engine_test.go` (incl. `TestPolicySHA256Stored`, which pins the stored column, not the dead field) passes.
- P7: trigger IDs derived from `deployment|situation|version|name` via SHA-256; evaluation is a pure function of spec + versions; serial per tx.
