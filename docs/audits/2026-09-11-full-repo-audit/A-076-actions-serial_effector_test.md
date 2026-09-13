# A-076 · `internal/actions/serial_effector_test.go`

LOC: 492 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T5: no copy-pasted session-restart blocks; restart setup exists once as a helper.
- T3: context usage consistent with the rest of package `actions_test` (sibling `phase04_test.go` uses `t.Context()`).

## Findings
- **[MED] F1. Duplicated 20-line session-restart blocks** — `internal/actions/serial_effector_test.go:330-351` and `internal/actions/serial_effector_test.go:378-396`. Both tests hand-roll the same restart sequence (catalog.Digest → goldenDeviceState + capability_digest → fresh fakeDeviceTransport → OpenDeviceSession with the same 8-field config literal → Exchange must fail). The config literal is a third copy of the one inside `openThermalSessionWithControl` (`serial_session_test.go:110-114`). If `DeviceSessionConfig` grows a required field, three call sites must be edited in lockstep. Extract `restartThermalSession(t, control, catalog) (*actions.DeviceSession, *fakeDeviceTransport)` next to the existing helpers and have both tests call it.
- **[LOW] F2. context.Background() while sibling file uses t.Context()** — `internal/actions/serial_effector_test.go:19,67,103,137,156,170,199,232,253,282,311,358,371,406,424,439,459,464,482`. `phase04_test.go` in the same package consistently uses `t.Context()`; this file uses `context.Background()` throughout. AGENTS.md prefers `t.Context()` in tests. Pick one convention for the package (t.Context()) and migrate this file.

## Checked, not an issue
- T1: assertions check observable outcomes (receipt/result maps, send counts, `IsUnknownOutcome`, durable `ReconciliationRequired`/DB barrier rows) with diagnosing failure messages; no tautologies found.
- T2: fully deterministic — no sleeps; cancellation injected via `transport.sendHook`/`receiveErr` hooks guarded by the fake's mutex.
- T3: subtests independent where used; no shared mutable state between tests.
- T4: covers all documented SerialEffector surfaces (Dispatch, DispatchAuthorized, VerifyDeviceCommand incl. LED/fan/boot-rollover mismatches, SafeStop reject/untrustworthy/undurable/ambiguous paths, metrics, duplicate receipt cache, catalog-vs-handshake mismatch).
- T5: `openThermalSession`/`openThermalSessionWithControl`/`acceptedReceipt`/`goldenDeviceState` are shared package helpers, not redefined here.
