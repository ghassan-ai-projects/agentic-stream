# Decision Log — Real-World Sensor HIL-0 (Agentic Stream slice)

Status: recorded 2026-08-30. This closes the "decisions before coding" gate that
`README.md` §5 and `EXECUTION_PLAN.md` require. Phase 03 was implemented against
the assumptions below; this file ratifies them and marks the ones that still
need the hardware owner.

Two classes of decision:

- **[RATIFIED]** — already true in the code; recorded here so it is a decision,
  not an accident.
- **[OWNER]** — needs the hardware owner before the *physical* rung (G4a). None
  of these block the emulated path (Phases 01–03 and the emulated G1 run).

| # | Decision | Value | Class | Grounded in |
|---|---|---|---|---|
| 1 | Board + logic voltage | TODO — exact Arduino model and 3.3/5 V logic | **[OWNER]** | drives observation unit/quality fields; blocks HIL only |
| 2 | First sensor + first actuator | LED (`led-01`) first, then a 5 V fan (`fan-01`) via a logic-level MOSFET; sensor = a digital/I2C temperature sensor | **[OWNER]** to confirm exact parts | `EXECUTION_PLAN.md` BOM; catalog targets `led-01`/`fan-01` |
| 3 | Safe state per output | De-energized / off. Explicit `safe_stop` operation per target, `expires_after_ms: 1000` | **[RATIFIED]** | `internal/contractsv1/conformance/v1/thermal-capability-catalog.json` → `safe_stops` |
| 4 | Feedback mechanism + independence | Fan: tach or current sense, independent of the command path. In emulation the independent feedback is the Streams Simulator world oracle (the real process state, not the ack) | **[OWNER]** to confirm the physical sensor | `deviceworld` plant reads process state for verification; receipt ≠ effect |
| 5 | Does dedup survive MCU reset? | **No — dedup is volatile.** A reboot clears the ledger, so the **reboot reconciliation barrier (Phase 04) is mandatory** | **[RATIFIED]** | emulator `device.State().dedup_ledger.persistent = false`; `Reboot()` clears it |
| 6 | NDJSON vs COBS/CBOR frames | **NDJSON first** (canonical JSON, one record per line); framed binary is a later swap behind the same `DeviceTransport` interface | **[RATIFIED]** | `internal/actions/device_codec.go`; emulator `internal/device/codec.go` |
| 7 | Serial library + port ownership | Agentic Stream opens **no serial port**. The emulated boundary is a **UDS** gateway link; on hardware the edge gateway owns the port and speaks the same records over `DeviceTransport` | **[RATIFIED]** | `03-serial-effector.md`; `DeviceTransport` in `internal/actions/serial_session.go` |
| 8 | Command lease + watchdog deadlines | Per-command lease via `expires_after_ms`: `set_pwm_lease` route TTL 20 000 ms, per-command `lease_ms` ≤ 10 000, `duty_permille` ≤ 600; `safe_stop` TTL 1 000 ms. Firmware watchdog must drop to safe state within one `lease_ms`; exact watchdog period **[OWNER]** | **[RATIFIED]** bounds, **[OWNER]** watchdog period | thermal capability catalog `bounds` + `expires_after_ms` |
| 9 | Shared protocol contract location | Owned in `internal/contractsv1/` (`schemas/v1/device-*-v1.json` + `conformance/v1/`); consumed by Streams Simulator as a vendored copy (`internal/device/contract/`) | **[RATIFIED]** | E1 conformance fixtures; `streams-simulator .../contract/SOURCE.md` |
| 10 | Owner authorization for real low-voltage actuation | TODO — explicit owner sign-off | **[OWNER]** — **blocks G4a**; not required for the emulated G1 | `EXECUTION_PLAN.md` decision log #10 |

## What is unblocked now

With items 3, 5, 6, 7, 8 (bounds), 9 ratified, the **emulated** path — Phases 01–03
and the joined emulated G1 run (Agentic Stream serial effector ⟷ Streams
Simulator device emulator over a UDS, with Tamoz as the worker) — needs no
hardware and no owner sign-off. It can proceed today.

## What the emulated path cannot claim

Items 1, 2, 4 (physical sensor), 8 (watchdog period), 10 are hardware evidence.
Passing the emulated gates never licenses a physical claim — voltage, brownout,
serial corruption, actuator stall/backlash, watchdog-during-lockup, and feedback
independence are a separate evidence class (Round 1 review §4). G4a stays blocked
on item 10.
