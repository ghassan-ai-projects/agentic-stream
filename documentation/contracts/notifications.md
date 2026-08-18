# Notification contract v1

Notifications are a versioned, cursor-resumable observation surface. They are
not a second source of truth and do not authorize an action.

## Contract identity

- Contract: `situation-runtime/notification-contract/v1`
- Data schema: `urn:situation-runtime:notification-contract:v1`
- Machine authority: [`internal/notifycontract/`](../../internal/notifycontract/)

The repository-level mirror under [`docs/contracts/`](../../docs/contracts/)
exists for contract packaging and review. The embedded files under
`internal/notifycontract/contracts/` are what the runtime loads.

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

`GET /v1/events` is at-least-once. Clients must deduplicate by CloudEvent
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
