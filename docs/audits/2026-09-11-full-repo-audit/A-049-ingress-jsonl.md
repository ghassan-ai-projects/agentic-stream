# A-049 · `internal/ingress/jsonl.go`

LOC: 183 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Quarantine identity is unique per trace/connector so two traces cannot collide on `(tenant_id, event_id)`.
- Checkpoint load treats only `sql.ErrNoRows` as "no checkpoint"; all other DB errors are returned.
- Blank/whitespace lines and oversized lines are handled consistently with the simulator and live-socket connectors in the same package.

## Findings
- **HIGH F1. Quarantine event ID `line:<N>` collides across traces and aborts ingestion** — `internal/ingress/jsonl.go:102` (and the same scheme in envelope quarantine via `QuarantineEnvelope`'s env id, :111/:117 resolve through the envelope, but raw lines use `line:N`). The quarantine table keys on `(tenant_id, event_id)`; `Quarantine` detects a same-ID/different-payload conflict, marks the prior record `rejected`, and returns an error (eventlog/quarantine.go:25-31, 90-92). Two different trace files ingested by the same tenant whose malformed lines share a line number — entirely normal for the batch (`live-jsonl:<path>`) and replay (`replay:<path>`) connectors — make the second run fail with `event id line:N has conflicting quarantined payload` and corrupt the first record's status. The live-socket path already scopes IDs (`live-uds:<instance>:<conn>:<line>`, live_socket.go:315). Fix: derive the ID from the connector identity plus line (and/or a payload digest), e.g. `<connectorID>:line:<N>`.
- **MED F2. `loadCheckpoint` swallows every DB error as "no checkpoint"** — `internal/ingress/jsonl.go:145-151`. Any `Scan` error — not just `sql.ErrNoRows` — silently returns start-of-file, so a transient DB failure replays the entire trace from line 0. Dedupe keeps the log correct, but the error is swallowed (P1). Fix: return `nil` result only for `errors.Is(err, sql.ErrNoRows)`; wrap everything else.
- **LOW F3. Whitespace-only lines quarantined while the simulator skips them** — `internal/ingress/jsonl.go:97` skips only `len(line) == 0`, so a line of spaces is parsed, fails JSON decode, and lands in quarantine as `malformed_json`; simulator.go:110 skips `strings.TrimSpace(...) == ""`. Same package, same input class, different behavior. Fix: align on one rule (skip blank-and-whitespace lines).
- **LOW F4. Oversized lines abort the whole replay instead of being quarantined** — `internal/ingress/jsonl.go:69`. `bufio.NewScanner`'s default 64KiB token limit makes one long line fail `Run` with `token too long`, whereas the live path bounds and quarantines oversized input (live_socket.go:264-285). Fix: size the scanner buffer to the documented event bound and route overflow to `QuarantineRaw` like other malformed input.

## Resolution (2026-09-11) — FIXED
- **F1 (HIGH)** fixed: raw-line quarantine IDs are now connector-scoped via `quarantineID(lineNum)` → `<connectorID>:line:<N>`, so two traces (e.g. `replay:trace-a` / `replay:trace-b`) ingested by the same tenant no longer collide on `(tenant_id, event_id)`. Test `TestJSONLReplayQuarantineIDsAreConnectorScoped`.
- **F2** fixed: `loadCheckpoint` returns start-of-file only for `sql.ErrNoRows`; every other error is wrapped and returned instead of silently replaying from line 0.
- **F3** fixed: blank/whitespace-only lines are skipped via `bytes.TrimSpace`, matching the simulator connector.
- **F4** fixed: replaced the 64 KiB-default `bufio.Scanner` with a bounded `readBoundedLine` reader that caps per-line memory at `maxLiveSocketLineBytes`, quarantines an oversized line as `line_too_large`, and resynchronizes to the next line so ingestion continues (rather than aborting with `token too long`). Unlike the live-socket reader — which drops its connection after an oversized frame and can leave the newline ambiguously consumed — `readBoundedLine` resyncs deterministically. Test `TestJSONLReplayQuarantinesOversizedLine`.
- Verified: `go build ./...`, `go test ./internal/ingress/ ./internal/replay/ ./internal/runtime/` pass.

## Checked, not an issue
- P1: append batches idempotent via log dedupe (position < 0 not counted); scanner and flush errors wrapped `%w`; checkpoint written only after a full pass.
- P2: three-stage validation (JSON, `ValidateEnvelope`, schema) with invalid input quarantined durably, never appended; nothing from the trace becomes instructions; checkpoint identity is connector-scoped even though the quarantine ID (F1) is not.
- P4: pure connector; appends via `eventlog.EventLog`; no transport or engine concerns.
- P7: shared by the live pipeline (`pipeline.RunJSONL`), `serve` polling, and replay (`replay.go:489` uses `NewJSONLReplayWithClock`) — one implementation, no replay-specific fork.
- P6: jsonl_test.go covers malformed and schema-invalid quarantine plus checkpointed resume.
