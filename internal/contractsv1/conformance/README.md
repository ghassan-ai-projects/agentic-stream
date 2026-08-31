# Device Wire Conformance Fixtures (v1)

This directory is the **cross-repo contract surface** for the Real-World Sensor
serial-effector boundary, including the device wire fixtures and the closed
capability catalog. It is the single source of truth that the other
program repos copy and test against:

- **Streams Simulator** — the device-transport emulator that plays the Arduino.
- **Edge gateway** — the process that turns real USB serial bytes into these records.
- **Firmware** — the Arduino sketch, when it exists.

Agentic Stream owns the schemas (`../schemas/v1/device-*-v1.json`) and generates
these example frames from one Go source (`../conformance.go`). Consumers must not
re-invent the frames — copy these bytes.

## Layout

```
conformance/v1/
  thermal-capability-catalog.json  closed route and safe-stop authority
  valid/     one byte-exact wire example per message type (command, receipt, result, state)
  invalid/   frames a conforming decoder MUST reject, one broken rule each
```

Every wire fixture is **canonical JSON**: object keys sorted, no insignificant
whitespace, one record, trailing `\n`. That is exactly the byte layout
`EncodeDeviceRecord` puts on the wire (raw framing — e.g. NDJSON line, COBS —
remains the gateway's responsibility, layered around these bytes). The
capability catalog uses the same canonical JSON digest rules and its pinned
digest test lives with the action materializer.

## The four message types

| Message | Direction | Meaning |
|---|---|---|
| `command` | host → device | a materialized, bounded operation (`target`, `operation`, `parameters`, `expected_boot_id`, `expires_after_ms`, `policy_digest`, `idempotency_key`). |
| `receipt` | device → host | receipt of the command only — `accepted` + optional `reject_code`. **Receipt is NOT proof of physical effect.** |
| `result` | device → host | terminal outcome of executing the command (`executed` / `rejected` / `expired` / `superseded` / `safe_state`). |
| `state` | device → host | device identity + safety snapshot (`boot_id`, `firmware_digest`, `capability_digest`, `safe_state`, `current_output`). |

## Field convention (do not normalize)

- **Device wire records** (here) use `snake_case`: `message_id`, `boot_id`,
  `idempotency_key`, `expires_after_ms`.
- **SituationSpec** (a different layer) uses `camelCase`: `parameterSchema`,
  `eventType`.

## How a consumer repo uses these

1. Copy `valid/*.json` and `invalid/*.json` into the consumer repo (or read them
   from a vendored copy). Record the source commit.
2. In the consumer's test suite:
   - assert every `valid/*.json` **decodes and passes** the consumer's decoder;
   - assert every `invalid/*.json` is **rejected** (fails closed);
   - for a device emulator: assert it can *produce* `receipt`/`result`/`state`
     records that are byte-identical to the `valid/` examples for the equivalent
     inputs.
3. Never edit a copied fixture to make a test pass — a mismatch means the wire
   formats have diverged; fix the consumer or open a contract change here.

## Changing the contract

These JSON files are the **source of truth** — data, not generated from Go
literals. `../conformance.go` loads them via `go:embed`, and
`../conformance_test.go` validates every `valid/` fixture against its schema and
asserts every `invalid/` fixture fails closed. So editing a fixture here is how
you change an example; the tests catch any fixture that drifts from the schema
Agentic Stream enforces. Invalid fixtures are mapped to the schema they violate
by filename prefix (`command-*` → device-command, etc.).

Bump the version directory (`v2/`) for a breaking wire change rather than
mutating `v1/` in place.
