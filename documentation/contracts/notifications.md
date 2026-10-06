# Notification contract v1

Notifications report committed lifecycle changes. Each has a versioned
contract and a cursor that lets subscribers resume delivery. Stored runtime
records remain authoritative; a notification does not authorize an action.

## Contract identity

- Contract: `situation-runtime/notification-contract/v1`
- Data schema: `urn:situation-runtime:notification-contract:v1`
- Machine authority: [`internal/notify/internal/domain/`](../../internal/notify/internal/domain/)

The repository-level mirror under [`docs/contracts/`](../../docs/contracts/)
exists for contract packaging and review. The embedded files under
`internal/notify/internal/domain/contracts/` are what the runtime loads.

## Current event types

- `io.agenticstream.outcome.recorded.v1`
- `io.agenticstream.outcome.reconciled.v1`
- `io.agenticstream.approval.requested.v1`
- `io.agenticstream.approval.withdrawn.v1`
- `io.agenticstream.approval.resolved.v1`
- `io.agenticstream.command.dispatched.v1`
- `io.agenticstream.situation.superseded.v1`
- `io.agenticstream.reconsideration.admitted.v1`

Each event binds tenant data to the envelope tenant and source authority, and
uses the shared CloudEvent validation rules.

## Delivery semantics

`GET /v1/events` uses Server-Sent Events (SSE). Delivery is at-least-once,
so a client can receive the same notification again. Clients must deduplicate by CloudEvent
`source`/`id`, persist the cursor, and handle:

- `Last-Event-ID` or `cursor` resume;
- cursor expiry after retention;
- bounded slow-subscriber disconnects;
- poison-event retry and audited skip;
- idle comments and retry hints.

## Next reads

- [HTTP and SSE reference](../reference/http-api.md)
- [Observability design](../design/observability.md)
- [Operations observability](../operations/observability.md)
