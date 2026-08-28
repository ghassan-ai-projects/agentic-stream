# Phase 04 execution plan — authority, recovery, and soak evidence

Status: repository implementation in progress; the phase gate is open until
repository tests, emulator evidence, and external HIL evidence are all
available.

This phase closes the Agentic Stream side of G4c/G5. It reuses the existing
`storage.RuntimeOwner`, `storage.EpochControl`, action dispatcher, and durable
ledgers. It does not add a second authority model, raw serial implementation,
firmware behavior, or a claim that an emulator proves physical safety.

## Bar

The repository implementation is complete only when all of the following are
true and named tests prove them:

1. A live serial command is bound to both the current runtime authority epoch
   and the current device boot. A durable target claim records the owner epoch,
   instance, and lease. A lost/controlled epoch cannot start ordinary work, and
   a target-scoped second epoch is rejected with an observable durable reason.
   If authority expires during transport, the result is treated as unknown; the
   software contract does not pretend a host-side check can cancel bytes already
   in flight.
2. A device boot change opens a durable reconciliation barrier recorded in the
   runtime database. On every startup, an active barrier fails closed until an
   explicit state query and reconciliation-clear operation complete. While it
   is open, no energizing command is sent; safe-stop remains available. Unknown
   outcomes are resolved only through `Dispatcher.ReconcileUnknown` with typed
   state/feedback evidence.
3. Safe-stop uses a dedicated, non-queued path with a fixed catalog-owned
   command. It cannot clear a physical e-stop, cannot be replaced by a normal
   route, and is ordered above ordinary approved commands. Tests prove no
   ordinary frame is emitted after a kill/interlock/barrier condition.
4. `export-run` requires an explicit tenant, reads one SQLite read transaction
   snapshot for that tenant, and atomically publishes an immutable snapshot
   directory containing the canonical manifest, durable ledger JSONL
   projections (including serial command/device/boot bindings), canonical spec/policy inputs where available, metrics, verdict,
   and a SHA-256 checksum file. The current ledgers do not carry an execution
   ID, so this is not yet a row-filtered single-execution artifact; the manifest
   records that limitation. Export refuses path traversal and checksum
   verification detects tampering.
5. A durable-ledger soak report computes every zero-tolerance counter declared
   by Experiment 10. Process-local telemetry is diagnostic only and is never
   used as safety proof. The report includes evidence completeness and
   diagnostic metrics, and fails the verdict if any zero-tolerance counter is
   non-zero or a physical transition lacks evidence. Synthetic fault fixtures
   exercise pass and fail verdicts deterministically.
6. Modified packages have meaningful tests. Focused tests, race tests for the
   session/effector, `go vet ./...`, `go build ./...`, `go test ./...`,
   `make ci-check`, and `git diff --check` are run; environment/toolchain
   failures remain recorded as blockers rather than being called green.

The phase remains externally blocked until the gateway adapter, firmware
watchdog/e-stop behavior, independent feedback verifier, emulator fault
schedule, and physical Experiments 7–10 are supplied by their owning systems.

## Implementation sequence

### 4.1 Epoch and target fencing

- Extend the typed device-session configuration with an authority gate that is
  checked immediately before ordinary send and again after transport returns.
  The post-send check classifies a lease/epoch loss as unknown; it cannot undo
  transport bytes already accepted by the gateway.
- Reuse `RuntimeOwner` for singleton ownership and add a small durable
  target-claim ledger so duplicate epochs are rejected and observable without
  permitting a second writer.
- Claims have an epoch/instance/lease fence, are recovered on startup, and are
  released on session close only when still owned. Expiry, killed, and draining
  epochs fail closed. Safe-stop is explicitly exempt from the ordinary
  admission gate.

### 4.2 Reconciliation barrier

- Persist `reconciliationRequired` and the boot that opened the barrier in a
  small target state table. A refresh that sees a new boot clears receipt cache
  and opens the barrier before exposing the new session for energizing work.
  Startup restores an uncleared barrier and fails closed.
- Add a typed state-query method and an explicit barrier-clear method requiring
  a durable reconciliation record containing the queried state digest and
  final status. The session itself never declares an unknown command
  successful. The clear method accepts only `succeeded`, `failed`, or
  `manual_review`, and requires non-empty evidence.
- Ensure ordinary `SerialEffector.Dispatch` refuses while the barrier is open;
  route reconciliation through the existing dispatcher unknown-outcome path.

### 4.3 Safe-stop priority

- Add a fixed, catalog-bound safe-stop operation and a dedicated transport send
  path with a priority mutex. It may wait for an already-in-flight frame to
  finish, but it is never queued behind the reconciliation barrier or a normal
  command; queued normal work is rejected after the stop is requested.
- Reject remote e-stop-clear operations and any model-supplied safe-stop
  parameters. Keep firmware-owned hard interlock/e-stop/local-safety ordering
  outside the supervisory command API.
- Add telemetry for barrier entries, target-claim rejections, safe-stop sends,
  and safe-stop failures.

### 4.4 Run artifact exporter

- Add an `internal/runartifact` package with explicit manifest input and
  deterministic export/verify helpers.
- Require an explicit tenant for export and apply it to every tenant-bearing
  ledger projection. Device authority/safety tables are currently shared
  device evidence because their schema has no tenant column.
- Export selected durable tables as stable JSONL with row digests from one
  SQLite read transaction. Include schema/migration metadata, canonical
  deployment metadata, and write `checksums.sha256` last. Until ledgers gain an
  execution identifier, call the result a tenant snapshot rather than a
  single-execution export.
- Add `agentic-stream export-run --db --output` and
  `agentic-stream verify-run <directory>`. Export refuses an existing output
  directory; callers choose a new directory for each immutable artifact.
  Publication writes a sibling temporary directory and atomically renames it.
  Verify returns a non-zero error for
  missing files, checksum mismatch, malformed JSONL, or a manifest digest
  mismatch. The exporter is read-only with respect to the hot-loop database.

### 4.5 Soak report and verdict

- Add a ledger query/report package that derives zero-tolerance counts from
  commands, outcomes, verifications, reconciliation records, and durable
  state/feedback records. Persist required safety events rather than reading
  process-local counters.
- Keep report-only latency/cost/reset fields separate from safety verdicts.
- Persist the generated report only in the exported run artifact; do not make
  the hot loop depend on a mutable process-local verdict.

## Review checkpoints

- Before implementation: two independent plan reviews, one safety/correctness
  lens and one architecture/operability lens.
- After implementation: three independent code reviews, one correctness,
  one architecture, and one clean-code/quality lens. Findings must be fixed
  or explicitly recorded as external blockers before commit.
- Before commit: focused tests and the repository-wide commands above; commit
  status says repository implementation complete only if all repository-owned
  bar items are evidenced.

## Explicit non-goals

- No direct USB/Arduino serial library, raw framing, reconnect loop, or device
  identity implementation.
- No firmware watchdog, hard interlock, physical e-stop, local safety policy,
  or remote e-stop clear.
- No claim that a receipt, device state, emulator, fixture, or telemetry
  counter proves a physical actuator transition without independent feedback.
- No start of Phases 06–09; those are checkpoint-gated maturity skeletons.
