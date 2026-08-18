# Roadmap

This roadmap is a sequencing view, not a promise. The canonical current
posture is [`governance/release-status.json`](governance/release-status.json).

## Shipped in the current development tree

- Deterministic JSONL stream processing and replay.
- SituationSpec compiler, schema validation, canonical digests, and domain
  data registries.
- Event-time windows/operators, immutable Situation versions, and source-health
  completeness behavior.
- Deterministic cognitive scheduling and bounded episode lifecycle.
- Decision/Intent validation, policy governance, approvals/interlocks, and
  idempotent action outbox.
- Go current-v1 worker boundary, native executor, evidence tools, and
  conformance tests.
- Replay/shadow library modes, loopback HTTP, metrics, authenticated SSE,
  runtime ownership, recovery, and telemetry.

## Hardening next

- Complete backup/restore, WAL, disk-full, crash, and owner-takeover rehearsal.
- Qualify long-running workload behavior and capacity limits.
- Review and qualify concrete external effectors and reconciliation paths.
- Finish deployment-specific worker/TLS/key-rotation evidence.
- Publish release artifacts, compatibility policy, checksums/provenance/SBOM,
  and rollback instructions.
- Keep docs-check and status evidence synchronized with the code.

## Deferred by design

- Kafka, NATS, MQTT, and other broker ingress.
- Web UI and general workflow/graph engine.
- Multi-agent mesh, arbitrary plugin marketplace, and self-modification.
- Python workers and broad remote worker fleet management.
- Distributed scale-out before single-node semantics are stable.

## Next reads

- [Current status](overview/status.md)
- [Limitations](overview/limitations.md)
- [Release evidence](governance/release.md)
