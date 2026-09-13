# A-039 · `internal/actions/serial_session_exchange.go`

LOC: 237 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Every exported method in the file has a production caller (`serial_session_exchange.go:22-28`).

## Findings
- **MED F1. `Exchange` is production-dead** — `internal/actions/serial_session_exchange.go:22-28`. The receipt-only wrapper has no production caller; the action plane uses `ExchangeWithResult` (`serial_effector.go:155`) and every other reference is a test (`phase04_test.go`, `serial_session_test.go`). Delete the wrapper and port the tests to `ExchangeWithResult`, or document it as a test seam and unexport it.

## Checked, not an issue
- P1: entire exchange serialized under `s.mu` (`:45-46`); safe-stop priority checked before any wire write (`:50-52`); boot lifetime re-validated against the session (`:103-109`); errors wrapped `%w`, `errors.Join` for compound send/bind failures.
- P2: durable target claim + command binding + a second authority assert immediately before delivery (`:111-139`) minimize the revoke-to-send race; bind failure releases the claim and tolerates `ErrTargetClaimNotOwned` (`:125-129`); post-send failures always route to `unknownDeviceOutcome`, which latches the reconciliation barrier and invalidates the transport (`:205-212`) — a possibly-sent frame is never retried blindly; cached receipts reject idempotency-key collisions on semantic digest (`:141-150`).
- P3: `prepareCommand`/`receiptMatchesCommand`/`resultMatchesCommand` are single-purpose helpers, not speculative abstraction; no duplicated logic with `serial_session_safety.go` beyond the shared helpers (`failedSafeStopReceipt`-style duplication is charged to A-040).
- P4: transport accessed only through the typed `DeviceTransport` interface; untrusted device frames are validated records, never instructions.
- P5: exported symbols documented; contexts honored.
- P6: `serial_session_exchange_test.go` pins the result/receipt matching matrix including safe-stop status requirements; `phase04_test.go` covers barrier, cache, and stop-priority behavior.
- P7: stable identities (command_id, boot_id, semantic digest) enforced on both receipt and result (`:214-237`).
