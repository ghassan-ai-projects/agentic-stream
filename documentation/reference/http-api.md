# HTTP and SSE reference

The HTTP API provides health checks, metrics, notifications, and operator
controls. It does not provide general endpoints to create, read, update, or
delete runtime records.

## Routes

| Method | Route | Auth | Behavior |
| --- | --- | --- | --- |
| `GET` | `/health/live` | none | `{ "status": "live" }` when process is live |
| `GET` | `/health/ready` | none | `{ "status": "ready" }` or RFC 9457-style problem |
| `GET` | `/v1/events` | subscriber bearer token | cursor-resumable SSE notification stream |
| `GET` | `/metrics` | none in the handler; deployment boundary | low-cardinality runtime metrics |
| `GET` | `/v1/approvals/{id}` | approval relay bearer token | immutable request and exact bytes for an authorized human decision |
| `POST` | `/v1/approvals/{id}` | approval relay bearer token + approver signature | commit approve/deny and revalidate before command creation |
| `POST` | `/control/drain` | exact control Authorization header | refuse new admission for current epoch |
| `POST` | `/control/kill` | exact control Authorization header | refuse later decisions for current epoch |

Source: [`internal/api/internal/transport/events.go`](../../internal/api/internal/transport/events.go) and
[`internal/api/internal/transport/http.go`](../../internal/api/internal/transport/http.go).

## Problem responses

Health failures use `application/problem+json` with a stable problem type,
status, detail, and instance. SSE errors use the same media type with codes
such as `cursor_expired`, `subscriber_too_slow`, and `notification_retry`.

## SSE

```bash
curl -N \
  -H "Authorization: Bearer $AGENTIC_STREAM_SUBSCRIBER_TOKEN" \
  -H "Last-Event-ID: 12" \
  'http://127.0.0.1:8080/v1/events?tenant=default'
```

The stream is at-least-once. The numeric event ID is a tenant-local cursor;
deduplicate by CloudEvent source/id. `cursor` may be used instead of
`Last-Event-ID`. A cursor outside retained history returns HTTP 409 and needs
an audited resnapshot. The server disconnects subscribers that exceed the allowed lag.

## Controls

The current control handler expects the configured token as the full
`Authorization` header value. It returns the current epoch and state as JSON.
Treat these endpoints as privileged operator actions; put them behind a local
administrative boundary and audit every call.

## Human approvals

Approval routes are enabled only with `serve --spec` and both
`AGENTIC_STREAM_APPROVAL_TOKEN` and `AGENTIC_STREAM_APPROVAL_RELAY` set. The
credential authenticates one configured relay in the served tenant; it is
independent of subscriber and control credentials. Durable principals,
Ed25519 verification keys, roles and tenant/entity/risk authorities are
provisioned with `agentic-stream principals apply --file <principals.yaml>`
while the runtime is stopped (see the CLI reference and
`examples/real-world-sensor/principals.example.yaml`). There is no HTTP
provisioning endpoint.

Fetch `/v1/approvals/{id}?approver={principal}&approved=true` (or `false`) using
`Authorization: Bearer $AGENTIC_STREAM_APPROVAL_TOKEN`. The response includes
`approval_id`, `status`, immutable `request` JSON and base64 `signing_bytes`.
The independent human approver signs the decoded bytes with their Ed25519 key.
The relay then submits:

```json
{
  "approver_id": "operator-1",
  "approved": true,
  "signature": "<base64 Ed25519 signature>",
  "reason": "Reviewed the bound evidence"
}
```

The signed bytes bind the decision boolean, durable digests, nonce, expiry and
principals. Older signatures without the decision boolean are incompatible.
Both approval and denial require authorization and a valid signature. The
reason is relay-supplied audit metadata, not a signed assertion field. Tenant,
relay identity and evaluation time come from server configuration and runtime;
they cannot be overridden by JSON. Bodies are limited to 16 KiB and must contain
one JSON document with only the fields shown above.

Unknown and foreign-tenant IDs both return 404. Invalid fresh submissions return
403 without consuming the pending request. Invalid bodies return 400;
infrastructure or ownership failures return 503. Terminal requests cannot be
presented for signing (409). Resolution retries return the durable disposition
without creating another command. Responses use `Cache-Control: no-store`.

A 200 response confirms committed governance, including denied/stale/expired
outcomes; inspect the result and reason. It does not confirm an external effect.
Approved commands enter the durable outbox, which runtime maintenance drains
without waiting for new sensor input. Existing dispatch fences and verification
still apply. Observe durable events for delivery and outcome evidence.

The [package integration design](../../internal/policy/APPROVAL_HTTP_DESIGN.md)
records the contract and validation. A CLI approval client remains follow-up work.

## Not current

The detailed design describes future JSON endpoints for specs, situations,
episodes, intents, and replay. They are not registered by the current handler.

## Next reads

- [Notification contract](../contracts/notifications.md)
- [Runtime operations](../operations/runtime.md)
- [Troubleshooting](../guides/troubleshoot.md)
