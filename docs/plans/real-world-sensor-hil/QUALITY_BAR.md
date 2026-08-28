# Real-World Sensor HIL Quality Bar

Status: active; the repository currently has partial Phase 01 and Phase 03
implementation, but no HIL-0 gate is green yet.

This is the evidence ledger for the Agentic Stream slice of the Real-World
Sensor program. A phase is complete only when its bar is satisfied by named
repository tests and, where marked, lab evidence. Emulator, replay, and
fixture tests prove software behavior; they do not prove physical hardware
behavior.

## Common release bars

- Raw observations remain evidence. Untrusted payload content never becomes an
  executable target, operation, pin, opcode, or unbounded parameter.
- Receipt, execution result, feedback observation, and verification remain
  distinct durable states. A transport acknowledgement never proves a physical
  effect.
- Unknown outcomes stop retries until typed independent evidence reconciles the
  command. Replay and shadow paths cannot perform external effects.
- Every command is bound to an immutable identity, tenant, intent, situation
  version, policy digest, idempotency key, device boot, and expiry.
- Replays are deterministic across duplicate, late, invalid-quality, malformed,
  sequence-wrap, and reboot-boundary inputs.
- `go test ./...`, `go vet ./...`, `make build`, `git diff --check`, and the
  applicable repository gates pass. A local toolchain mismatch is recorded as
  an environment limitation rather than silently treated as a green gate.

## Phase 01 — telemetry vertical (G2)

Status: in progress.

Bar:

- The thermal observation schemas accept the complete provenance envelope and
  reject undeclared fields, wrong types, and invalid quality values.
- Invalid, warming, disconnected, and rail-fault samples remain in the event
  log as evidence but cannot contribute numeric features.
- Operator state is isolated at device-boot boundaries; sequence wrap and
  cross-boot ordering do not fabricate a slope or aggregate.
- `latest` fan feedback means the latest event-time sample, not the maximum in
  the window. Ambient tracking does not open or escalate a thermal Situation.
- The compiler-valid `zone_thermal` replay produces deterministic evidence and
  the focused thermal tests pass.

Repository evidence: `internal/replay/thermal_chamber_test.go` and the
operator, event-schema, event-log, and replay tests. Physical sensor evidence
is not yet available in this repository.

## Phase 02 — shadow path (G3)

Status: not started.

Bar:

- A deterministic baseline and Tamoz shadow executor receive byte-identical,
  immutable Situation snapshots for the same replay/run.
- Shadow execution records validated decision and intent content in the durable
  shadow record, but creates no live command, outbox row, credential, or effect.
- Abstention and adversarial or incomplete evidence are first-class outcomes and
  fail closed. Differences are reported without promoting either executor.
- The comparison artifact is reproducible and includes snapshot, executor,
  policy/spec revisions, and manifests.

## Phase 03 — serial effector (G4a–G4b)

Status: partial scaffolding only; gate not started.

Bar:

- Device wire schemas and codec are version-locked, strict, bounded in size,
  and tested under fragmentation, malformed input, and fuzzing.
- Materialization uses a closed route plus exact normalized target and a
  catalog preset; policy digest, not-before, boot identity, idempotency, and
  expiry are carried into the command. Out-of-bound values fail closed.
- The serial effector separates receipt, result, feedback observation, and
  verification; receipt alone cannot close verification.
- Physical, emulator, replay, and shadow profiles cannot accidentally cross
  effect boundaries. Unknown transport outcomes require reconciliation.
- Capability routing is explicit and unknown routes are rejected.

## Phase 04 — authority and soak (G4c–G5)

Status: not started.

Bar:

- Existing durable runtime ownership and epoch controls fence every dispatch;
  no parallel authority model is introduced.
- Boot change or restart enters a reconciliation barrier before any non-safe
  command; safe-stop has priority and lease loss cannot re-energize outputs.
- Reconciliation uses typed device state/feedback evidence, not caller-supplied
  success claims.
- A run manifest ties deployment, spec, policy, event, situation, decision,
  command, receipt/result/observation, verification, and fault counters.
- Emulator soak tests produce a reproducible verdict with duplicates, delays,
  reordering, disconnects, expiry, reboot, and unknown-outcome injection.

## Phase 05 — validation matrix and gates

Status: not started.

Bar:

- Every HIL-0 acceptance item maps to a named test or explicitly marked lab
  evidence item.
- The matrix distinguishes static, unit, integration, replay, emulator/HIL, and
  physical evidence; lower-level evidence is not promoted to a higher claim.
- The final report states exact green gates, remaining blockers, and the
  hardware-owner decisions required before physical actuation.

## Later maturity phases

Phases 06–09 remain skeletons. They must not be implemented until the preceding
M1–M3 evidence and the corresponding Round 2 investment checkpoints exist.
Their bars are planning guardrails, not current work authorization.
