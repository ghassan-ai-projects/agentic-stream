# A-083 · `internal/actions/phase04_test.go`

LOC: 333 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- T5: no helpers in this file that duplicate an existing same-package helper; one definition per concern.

## Findings
- **[MED] F1. `materializedCommandWithBoot` duplicates `materializedCommand`** — `internal/actions/phase04_test.go:323-333` vs `internal/actions/serial_session_test.go:121-131`. Identical bodies except the boot ID is a parameter here and hardcoded `"boot-A"` there. Keep only the parameterized version and define `materializedCommand(t, catalog, id, key) = materializedCommandWithBoot(t, catalog, id, key, "boot-A")` in one file.
- **[MED] F2. `openActionDB` duplicates `openActionFixture`** — `internal/actions/phase04_test.go:258-266` vs `internal/actions/dispatcher_test.go:408-411`. Both open a throwaway `storage.DB` on `t.TempDir()+"/actions.db"` with cleanup-on-exit. Two DB-open helpers in one package will drift (e.g., one gains `SetMaxOpenConns` or a fixed `Now`). Keep one package-level helper.
- **[LOW] F3. Repeated receipt+boot mutation idiom** — `internal/actions/phase04_test.go:55-56,81-82`. `receipt := acceptedReceipt("…"); receipt["boot_id"] = "boot-B"` appears twice (and `acceptedReceipt` already exists in `serial_session_test.go:133` without a boot parameter). Add an `acceptedReceiptForBoot(commandID, bootID)` variant instead of mutating at each call site.

## Checked, not an issue
- T1: assertions check durable, observable outcomes — barrier rows (`device_reconciliation.status`), `device_authority_events` counts, send counts, and post-restart Exchange refusals — with diagnosing failure messages; no tautologies.
- T2: fully deterministic — the authority-loss test advances the lease clock via a `receiveHook` mutating a fixed `now` (line 163) rather than sleeping; `t.Context()` used throughout.
- T3: `t.Context()` where appropriate; helpers use `t.Helper()` and `t.Cleanup`; no subtests needed at this scenario granularity.
- T4: covers boot-barrier survival and bound-state-only clearing, safe-stop priority over barriers plus durable completion across restart, authority-loss-as-unknown after transport send, session disable on reconciliation-persistence failure (DROP TABLE), and startup barrier requiring a fresh state query — genuine edge coverage of the session safety machinery.
- T5: `mustDeviceFrames`, `reconciliationEvidence`, `sha256Hex` are the sole definitions in the package and are reused by `serial_session_test.go`/`serial_effector_test.go`.
