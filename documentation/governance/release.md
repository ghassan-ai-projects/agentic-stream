# Release evidence and checklist

The current release status is the machine-readable record in
[`release-status.json`](release-status.json). It says `development` and
`unreleased`; this page explains the evidence required before changing that
posture.

## Required evidence

- clean build and module state;
- full or CI-equivalent tests, vet, lint, race, protocol drift, and vulnerability
  checks;
- deterministic replay of the predictive-maintenance trace with matching
  version-history hash;
- duplicate, out-of-order, late-correction, heartbeat, cancellation, fencing,
  policy, idempotency, and unknown-outcome evidence;
- worker conformance and capability-boundary evidence;
- replay/shadow no-effect evidence;
- HTTP/SSE authentication, cursor, readiness, control, and bounded-lag checks;
- database backup/restore, migration, owner takeover, crash, WAL, and disk-full
  rehearsal in the target deployment;
- external effector idempotency/reconciliation and provider timeout review;
- release artifact, checksum, provenance/SBOM, compatibility, and rollback plan.

## Release review

The release owner must reconcile this checklist with
[`docs/design/BUILD_COMPLETION_BAR.md`](../../docs/design/BUILD_COMPLETION_BAR.md),
[`docs/design/OPERATIONS_READINESS.md`](../../docs/design/OPERATIONS_READINESS.md),
and the current code/tests. A passing unit suite cannot close a deployment
evidence gap by itself.

## Before publishing

1. Update `release-status.json` and the root README together.
2. Confirm limitations no longer claim a closed gap, or record the remaining
   evidence.
3. Run `make ci-check`, `make docs-check`, and `git diff --check`.
4. Build from a clean checkout and rehearse the quickstart.
5. Review security, dependencies, migrations, protocol compatibility, and
   rollback.

## Next reads

- [Current status](../overview/status.md)
- [Limitations](../overview/limitations.md)
- [Quality](quality.md)
