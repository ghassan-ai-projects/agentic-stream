# Plan

Each round is reviewed, tested, linted and committed.

## Rounds

| Round | Change | Proof |
| --- | --- | --- |
| D0 | This folder and the reusable [refactor prompt](../../.agents/prompts/reference-module-refactor.md). | Review |
| D1 | Delete dead code (only tests call it, no plan requires it): `NewEmulatorEffector` and its config, `NewUDSTransport`, `DeviceSession.Exchange`, `RefreshState`, and the accessors `DeviceID`, `OwnerEpoch`, `SafeState`, `ReconciliationRequired`. Tests move to the live equivalents. | device tests, `deadcode ./...` |
| D2 | `internal/device/internal/domain`: capability catalog, materialization, effect-profile policy, record matching rules, state identity, command identity, deterministic output verification. | table-driven domain tests |
| D3 | Adapters: `internal/wire` (record codec) and `internal/transport` (UDS gateway link). | codec and transport tests |
| D4 | `internal/app`: session and effectors; facade reduced to configuration and delegation; nil-authority guards and telemetry nil checks removed; callers in `cmd` and `runtime` updated. | device, runtime, cmd tests |
| D5 | Gates (app imports no `net`), registration, docs, AGENTS.md. | `go test .`, injected-violation probe |
| D6 | Typed device records (`State`, `Command`, `Receipt`, `Result`) parsed once in `wire`, keeping the original document where a digest depends on it. | full suite |
| D7 | Polish: drop the redundant `AssertRuntime` calls, share the `DispatchAuthorized` preamble, request values for long signatures. | full suite |

## Status

| Round | Commit | Notes |
| --- | --- | --- |
| D0 | `544a45f` | Also adds the reusable refactor prompt. |
| D1 | `9589659` | Dead code removed; tests assert the durable reconciliation state. |
| D2 | `205319e` | Domain coverage 84.7%. |
| D3 | `898bb97` | `transport.PartialSendError` exported: any transport, including test doubles, may report a partial send. |
| D4 + D5 | `9e5f8f0` | Session and effectors in `internal/app`; facade 93.3% covered by driving every operation over a real socket; nil-authority branches and telemetry nil checks gone; gates extended. |

## D6 validation

Typed `State`, `Command`, `Receipt` and `Result` now flow through session,
safe stop and verification. The wire adapter validates incoming frames and
parses their fields once; original documents remain the source of state
and command digests, sealed evidence and provider results. The codec depends
on the pure domain values; dependency levels retain strictly downward imports.

The interrupted round already contained production edits, so test-first was
not possible for that portion. New tests cover state digest/document parity,
optional reply codes and invalid frames. A test first exposed shared optional
code pointers in cached exchanges; cloning them restores caller isolation.

Device and architecture tests pass with `-race -count=1`; whole-tree lint
reports zero issues. Full repository qualification follows D7.

## Behavior that stays the same

- Handshake checks, boot binding, receipt cache and idempotency conflicts.
- Exchange order and every unknown-outcome path, including link invalidation.
- Safe-stop lane semantics, latching and recorded stages.
- Catalog validation, digest (pinned cross-repo) and materialization bounds.
- Wire format: canonical NDJSON, 64 KiB frame limit, schema per message type.

## Deliberate behavior changes

- Dead APIs are removed.
- A session can no longer exist without an authority; the unclaimed-delivery
  branch disappears.
- Output verification fails when a route has more than one numeric output
  parameter, instead of picking one at random.

## Why safe stop and barrier clear stay

Both are required by the hardware-in-the-loop Phase 04 bar
([PHASE-04-EXECUTION.md](../plans/real-world-sensor-hil/PHASE-04-EXECUTION.md),
items 2 and 3; [04-authority-and-soak.md](../plans/real-world-sensor-hil/04-authority-and-soak.md),
tasks 4.2 and 4.3) and back Experiments 8 and 9. The phase built and tested the
mechanisms but never added a caller:

- Without a barrier clear, a device that reboots or has one unknown outcome is
  refused ordinary commands permanently, even across restarts (fail-safe, but
  out of service).
- Without a safe-stop caller, the only safe-state transition is the firmware's
  own lease expiry.

## Deferred follow-ups

1. Design and add production entry points for barrier clear (operator
   procedure) and safe stop (triggers: e-stop, watchdog, lease loss).
2. Rename the package's root types once callers settle (`GatewayEffector`
   already planned in D4).
