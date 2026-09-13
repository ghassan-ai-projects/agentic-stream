# A-040 · `internal/actions/serial_session_safety.go`

LOC: 226 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The safe-stop failure epilogue (invalidate transport + reconciliation barrier + durable failure record + telemetry + joined error) is implemented once, not three times (`serial_session_safety.go:65-86`, `:89-98`, `:127-147`).
- Every exported method in the file has a production caller (`serial_session_safety.go:14-20`).

## Findings
- **MED F1. Triplicated safe-stop failure epilogue** — `internal/actions/serial_session_safety.go:65-86`, `:89-98`, `:127-147`. `failedSafeStop`, the receive-error branch of `completeSafeStopExchange`, and `failedSafeStopReceipt` each repeat: `invalidateTransportLocked` (conditionally in the first, always in the others) + `requireReconciliation` + `recordSafeStop("safe_stop_failed", details)` + `ObserveSafeStopFailure` + `errors.Join(...)` into a `deviceExchangeError`. Three near-copies of a safety-evidence sequence will drift — a missed barrier write in one variant silently weakens the durable record. Extract one failure recorder taking (claim, cause, sent, details) and keep only the per-site differences inline.
- **MED F2. `SafeStop` wrapper is production-dead** — `internal/actions/serial_session_safety.go:14-20`. Only tests call it; the production path is `SafeStopWithResult` (via `SerialEffector.SafeStop`, itself production-dead — see A-053). Delete the wrapper or fold it into the effector-facing API once that API is actually wired.

## Checked, not an issue
- P1: locking is correct — `requestSafeStop` latches via `stopMu` before taking `s.mu` (`:32-33`, `:59-63`, `:206-210`); errors wrapped `%w`/`errors.Join`; partial transport invalidation happens before durable bookkeeping so a later priority stop cannot consume a stale response (`:67-72`).
- P2: safe stop bypasses the ordinary authority/barrier deliberately and is catalog-owned with no caller-supplied parameters (`MaterializeSafeStop`); there is intentionally no method that clears a physical e-stop; rejected-but-durable evidence stays in the known lane while undurable evidence escalates to `UnknownOutcomeError` (`:116-122`).
- P3: no serial-family duplication beyond F1; `stopRequested` latch is minimal.
- P4: priority lane separated from policy-approved dispatch as designed; no per-domain branches.
- P5: `releaseClaims` uses `context.Background()` deliberately at the close boundary with a nolint justification (`:190-204`); exported symbols documented.
- P6: `serial_session_safety_internal_test.go` and `serial_effector_test.go` cover possibly-sent safe-stop failure, rejection evidence durability, and transport invalidation.
- P7: deterministic lifecycle event types (`safe_stop_requested/failed/completed`) with stable claim identities.
