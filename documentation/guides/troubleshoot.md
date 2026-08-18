# Troubleshooting

Start with the exact error, runtime version, spec digest, trace source, and
database path. Do not attach credentials or raw restricted payloads to an
issue.

## `validate` fails

Check that `apiVersion` is `agentic-stream/v1`, `kind` is `SituationSpec`, all
input fields and units exist in the registry, CEL references declared
features, windows/operators/triggers are unique and declared, schemas and
catalogs are available, and YAML has no duplicate keys. Compare with the
known-good example and rerun `validate --json`.

## `run` refuses the database

Deterministic replay opens a fresh database. Use a new `--db` path for each
attempt; do not point it at a live runtime database or an existing replay
sidecar.

## `serve` exits before listening

Common causes are a missing `AGENTIC_STREAM_SUBSCRIBER_TOKEN`, only one of
`--spec`/`--trace`, a non-positive `--poll-interval`, a non-loopback listener
without an authenticated proxy, invalid worker TLS/evidence combinations, a
stale owner lease, or a failed SQLite migration.

## Worker handshake or execution fails

Confirm protocol/contract versions, worker name, socket permissions, terminal
event behavior, episode identity, fence, deadline, and current-v1 conformance.
If EvidenceTools is enabled, verify the socket and hex key length without
printing the key.

## Events are quarantined or late

Inspect event identity, schema version, tenant, partition key, event/ingestion
times, and source heartbeat. A late event may be intentionally dropped,
history-only, corrective, or reconsideration-producing according to the spec.
Do not edit the database to “fix” a late-data result; use the supported
redrive path and preserve the audit.

## `/v1/events` does not resume

Use the `Last-Event-ID` header or non-negative `cursor` query value. A cursor
older than retained history returns an audited cursor-expired problem and needs
an audited resnapshot. Clients must deduplicate at-least-once delivery.

## Effects are not dispatched

Inspect the Decision validation result, Intent policy status, approval/interlock
state, epoch control, outbox lease, and outcome/reconciliation state. A denied
or unknown result is safer than an untracked retry.

## Next reads

- [HTTP reference](../reference/http-api.md)
- [Recovery](../operations/recovery.md)
- [Security policy](../../SECURITY.md)
