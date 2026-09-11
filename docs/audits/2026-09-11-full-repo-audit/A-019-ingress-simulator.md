# A-019 · `internal/ingress/simulator.go`

LOC: 393 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every channel→field mapping lives in `internal/ingress/simulator_data.json`; no channel names in Go code.
- Checkpoint load treats only `sql.ErrNoRows` as "no checkpoint"; all other DB errors are returned.
- Checkpoint persistence exists once in the package, not twice with diverging blob shapes.
- Every validation error names the offending line number.

## Findings
- **HIGH F1. `mode` channel is a hardcoded domain branch in Go, not data** — `internal/ingress/simulator.go:276-280`. `if channelName == "mode" { data["mode"] = value }` sits beside the data-driven `channelFields` mapping, and `internal/ingress/simulator_data.json` contains no `mode` entry (grep-verified), so the "mode" channel's semantics are authored as a Go literal. AGENTS.md is explicit: adding a channel means editing the JSON, "never a Go literal". Because `mode` is currently unknown to the map it also takes the `data["value"]` fallback, so the branch encodes a dual-field behavior (`value` + `mode`) the JSON schema cannot express — domain behavior in code. Fix: extend `simulator_data.json` to express the mapping (e.g. allow a multi-target or `mode` entry with a declared fallback) and delete the branch; add a test pinning that no `channelName ==` literal exists in the converter.
- **MED F2. `loadLineCheckpoint` swallows every DB error as "no checkpoint"** — `internal/ingress/simulator.go:365-369`. `if err := ...Scan(&blob); err != nil { return 0, nil }` returns start-of-file on any failure — lock contention, corruption, a dropped connection — not just `sql.ErrNoRows`. A transient DB error silently reprocesses the whole trace. Fix: `if errors.Is(err, sql.ErrNoRows) { return 0, nil }` and wrap otherwise, matching quarantine.go's pattern.
- **MED F3. Checkpoint load/save duplicated with jsonl.go, already diverged** — `internal/ingress/simulator.go:365-393` vs `internal/ingress/jsonl.go:143-178`. Same table, same upsert shape, but the simulator writes `map[string]any{"version":1,...}` while jsonl writes a typed `checkpoint` struct, and the error semantics of the two loaders already differ (jsonl also swallows all errors, simulator's save hardcodes kind `'simulator-jsonl'`). Fix: extract shared `loadConnectorCheckpoint`/`saveConnectorCheckpoint` helpers parameterized by kind, with one blob type.
- **LOW F4. One validation error lacks line context** — `internal/ingress/simulator.go:141`. `fmt.Errorf("event arrival_time is required")` is returned bare while every sibling error wraps with `convert simulator line %d` / `validate simulator line %d` at the call site (:135-137). The message is also duplicated by `parseSimulatorTime`'s `%s is required`. Fix: include the line number at the raise site like the rest of `Run`.

## Checked, not an issue
- P1: errors wrapped `%w` elsewhere; batch appends idempotent via log dedupe (position < 0 not counted); scanner error checked; checkpoint saved only after a fully validated pass.
- P2: strict frame discipline — `runtime_config` must be first, nothing after `trace_end`, unknown fields rejected at every level, strictly increasing recorded times, `arrival >= event_time`; nothing from the trace becomes executable beyond normalized envelope data.
- P4: channel→field mapping loaded from embedded JSON via `sync.OnceValues` (go:embed), per the data-not-code rule for everything except F1.
- P5: stdlib-only helpers; exported symbols documented.
- P6: simulator_test.go and simulator_convert_test.go cover conversion edges, out-of-order/flattened rejection, and unknown record types.
