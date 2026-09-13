# A-020 · `internal/actions/watch_effector.go`

LOC: 379 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every parameter of every function in the file is used (`watch_effector.go:77`).
- `sql.ErrNoRows` is detected with `errors.Is` consistently with the rest of the package (`watch_effector.go:119,185` vs `dispatcher.go:403,496,674`).
- The CEL environment is not rebuilt per event on the fire hot path (`watch_effector.go:337-363`).
- No one-line wrapper exists that only re-exports another package symbol (`watch_effector.go:314-316`).

## Findings
- **LOW F1. Dead third parameter in `dispatch`** — `internal/actions/watch_effector.go:77`. `dispatch(ctx, command, _ func(context.Context) error)` accepts the authorization check passed by `DispatchAuthorized` (`:74`) and discards it; the check already ran once at `:71`. The blank parameter falsely suggests a re-check at the install boundary. Drop the parameter.
- **LOW F2. Inconsistent `sql.ErrNoRows` comparison** — `internal/actions/watch_effector.go:119,185`. Direct `!=`/`==` comparison where the same package uses `errors.Is(err, sql.ErrNoRows)` (`dispatcher.go:403,496,674`). Use `errors.Is` in both places.
- **LOW F3. CEL environment recompiled on every fire** — `internal/actions/watch_effector.go:337-363`, driven per event x watch from `FireEvent` (`:257-265`). `cel.NewEnv` + `env.Compile` run on the event hot path although the expression was already validated at install (`:93`, `:318-335`). Compile once at install (or cache the program keyed by watch) and evaluate only.
- **LOW F4. Pointless `isSQLiteBusy` wrapper** — `internal/actions/watch_effector.go:314-316`. A one-line alias for `storage.IsSQLiteBusy` used at a single call site (`:295`). Call `storage.IsSQLiteBusy` directly.

## Checked, not an issue
- P1: errors wrapped `%w`; transactions and context honored, including the retry timer honoring `ctx.Done()` (`:299-309`); no shared mutable state without builder-time writes.
- P2: idempotent by `watch_id` with conflict detection on stored vs requested fields (`:107-118`, `:143-152`); install is policy-approved via the dispatcher; bounded `max_fires` enforced atomically with the fire insert (`:207-220`); expression validated as CEL and size-bounded at install (`:90-94`).
- P3: no duplicated serial-family logic; `Fire`'s re-validation inside the tx (`:184-192`) is necessary, not duplicate.
- P4: no per-domain branches; route check (`:81`) is protocol routing, and `interlock.Assert` with empty tenant/target is fine for `DurableReader`, which ignores those arguments (readiness-only).
- P5: log/slog used (`:195`); exported symbols documented.
- P6: `watch_effector_test.go` covers bounded one-shot firing, expiry, CEL-error skip, and SQLite-busy retry.
- P7: deterministic clock injection (`:54-59`); fire exactly-once via `watch_fires` primary key.
