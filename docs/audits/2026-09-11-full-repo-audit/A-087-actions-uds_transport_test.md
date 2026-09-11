# A-087 · `internal/actions/uds_transport_test.go`

LOC: 343 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- F1: every wait on implementation behavior is bounded and failure-diagnosed.

## Findings
- **[LOW] F1. Unbounded wait on the blocked first send** — `internal/actions/uds_transport_test.go:296-299`. `<-firstErr` has no deadline, unlike every other wait in this file (lines 168-172, 194-198, 217-224, 241-247, 287-294 all use `time.After(1s)` + `t.Fatal`). If a regression makes the gate-blocked send ignore connection closure, the test hangs until the package-level go test timeout (10m) with no failure diagnosis. Fix: select on `firstErr` with a `time.After` bound and a `t.Fatal("blocked first send did not return after client close")` branch.

## Checked, not an issue
- T1: assertions check observable wire behavior (frame types, bounded-frame errors, `context.Canceled` propagation, closed-transport refusal); failures named.
- T2: no network (UDS in /tmp justified by sun_path limits, cleaned up; `net.Pipe` elsewhere); synchronization via notifying channels, not sleeps; goroutines joined via bounded waits; no races (`sync.Once` guards on the notifying conn).
- T3: subtests independent with fresh pipes per case; `t.Context()`-style cancellation via explicit contexts; loop capture present (redundant post-Go 1.22 but harmless).
- T4: covers contract framing end-to-end, both oversized-frame rejection and post-oversize closure, receive/send/send-wait cancellation, and all four invalid outbound frame shapes — real edge coverage.
- T5: `contractPeer`/`writeContractFrame`/`notifyingConn` defined once in this file, not duplicated; no fixture repetition (frames come from `contractsv1.ConformanceValidFrame`).
