# HTTP and SSE reference

The current handler is intentionally small. It exposes health, metrics,
notifications, and operator controls; it does not expose a general CRUD API.

## Routes

| Method | Route | Auth | Behavior |
| --- | --- | --- | --- |
| `GET` | `/health/live` | none | `{ "status": "live" }` when process is live |
| `GET` | `/health/ready` | none | `{ "status": "ready" }` or RFC 9457-style problem |
| `GET` | `/v1/events` | subscriber bearer token | cursor-resumable SSE notification stream |
| `GET` | `/metrics` | none in the handler; deployment boundary | low-cardinality runtime metrics |
| `POST` | `/control/drain` | exact control Authorization header | refuse new admission for current epoch |
| `POST` | `/control/kill` | exact control Authorization header | refuse later decisions for current epoch |

Source: [`internal/api/events.go`](../../internal/api/events.go) and
[`internal/api/http.go`](../../internal/api/http.go).

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
an audited resnapshot. Slow subscribers are bounded and disconnected.

## Controls

The current control handler expects the configured token as the full
`Authorization` header value. It returns the current epoch and state as JSON.
Treat these endpoints as privileged operator actions; put them behind a local
administrative boundary and audit every call.

## Not current

The detailed design describes future JSON endpoints for specs, situations,
episodes, intents, and replay. They are not registered by the current handler.

## Next reads

- [Notification contract](../contracts/notifications.md)
- [Runtime operations](../operations/runtime.md)
- [Troubleshooting](../guides/troubleshoot.md)
