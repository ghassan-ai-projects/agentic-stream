# Troubleshooting

Start with the exact error, runtime version, spec digest, trace source, and
database path. Do not attach credentials or raw restricted payloads to an
issue.

## `validate` fails

Check these parts of the spec:

- Set `apiVersion` to `agentic-stream/v1` and `kind` to `SituationSpec`.
- Use input fields and units defined in the event registry.
- Refer to declared features in Common Expression Language (CEL) expressions.
- Give windows, operators, and triggers unique names and declare their references.
- Make the required schemas and catalogs available.
- Remove duplicate YAML keys.

Compare with the working example and rerun `validate --json`.

## `run` refuses the database

Deterministic replay opens a fresh database. Use a new `--db` path for each
attempt; do not point it at a live runtime database or an existing replay
sidecar.

## `serve` exits before listening

Check the error for one of these common causes:

- A missing `AGENTIC_STREAM_SUBSCRIBER_TOKEN`.
- A `--spec` without exactly one of `--trace` or `--live-socket`.
- A non-positive `--poll-interval` or a non-loopback `--listen` address.
- An incomplete worker TLS or evidence configuration.
- A stale owner lease or failed SQLite migration.

## Worker handshake or execution fails

Confirm protocol/contract versions, worker name, socket permissions, terminal
event behavior, episode identity, fence, deadline, and current-v1 conformance.
If EvidenceTools is enabled, verify the socket and hex key length without
printing the key.

## Events are quarantined or late

Inspect event identity, schema version, tenant, partition key, event/ingestion
times, and source heartbeat. A late event may be intentionally dropped,
history-only, corrective, or reconsideration-producing according to the spec.
Do not edit the database to “fix” a late-data result. Quarantine release and
redrive are internal capabilities; there is no packaged public CLI or HTTP
redrive command. Use approved deployment tooling and preserve the audit.

## `/v1/events` does not resume

Use the `Last-Event-ID` header or non-negative `cursor` query value. A cursor
older than retained history returns an audited cursor-expired problem and needs
an audited resnapshot. Clients must deduplicate at-least-once delivery.

## Effects are not dispatched

Inspect the Decision validation result, Intent policy status, approval/interlock
state, epoch control, outbox lease, and outcome/reconciliation state. A denial
means the request was not permitted. An unknown outcome means the effect may
have happened; resolve that uncertainty before retrying.

## Next reads

- [HTTP reference](../reference/http-api.md)
- [Recovery](../operations/recovery.md)
- [Security policy](../../SECURITY.md)
