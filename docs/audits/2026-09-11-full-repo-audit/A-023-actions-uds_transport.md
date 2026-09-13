# A-023 · `internal/actions/uds_transport.go`

LOC: 319 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported symbol in the file has a production caller (`uds_transport.go:50-52`).
- Named result parameters are declared only where a deferred assignment needs them (`uds_transport.go:63,70`).

## Findings
- **MED F1. `NewUDSTransport` is production-dead** — `internal/actions/uds_transport.go:50-52`. The exported constructor's only callers are `uds_transport_test.go` and `uds_transport_internal_test.go`; production dials via `DialUDSTransport` (`cmd/agentic-stream/effect_profile.go:97`). Its doc comment claims "callers that own connection setup", but no such caller exists. Since `uds_transport_internal_test.go` is in-package, it could use `newUDSTransport` directly; unexport the constructor (or fold it into `newUDSTransport`) and move the external tests in-package.
- **LOW F2. Unnecessary named results** — `internal/actions/uds_transport.go:63,70`. `Send` and `writeFrame` declare `(err error)` but return explicitly and never assign the named result (only `writeFrameWithWriteGate` at `:81` needs it for the deferred `errors.Join`). Drop the names.

## Checked, not an issue
- P1: no data races — write gate serializes writes, `readMu` serializes reads, `stateMu` guards closed state (`:21-28`); the documented lock ordering in `QueryState` (write gate before `readMu`, `:124-132`) prevents the described deadlock; cancellation deadlines applied via `context.AfterFunc` with ordered cleanup (`:227-247`).
- P2: outgoing frames validated (single bounded newline-terminated frame, `:283-294`); inbound reads bounded with connection closed on overflow (`:170-176`, `:296-312`); partial writes surfaced as `possiblySentError` so callers treat them as unknown outcomes (`:87-104`, `:270-281`).
- P3: the `Send`→`writeFrame`→`writeFrameWithWriteGate` chain is thin but each layer exists to reuse the write-gate path from `QueryState`; no duplication with the serial_session family.
- P4: transport concerns stay inside the action plane; the `DeviceTransport` interface (`serial_session.go:16-28`) is the only leak point and is typed.
- P5: errors wrapped `%w` throughout; contexts honored everywhere including lock acquisition (`:200-207`).
- P6: `uds_transport_test.go` + internal tests cover contract, oversized frames, cancellation, partial-write marking, and query-state lock behavior.
- P7: not applicable (stateless byte transport); framing is deterministic.
