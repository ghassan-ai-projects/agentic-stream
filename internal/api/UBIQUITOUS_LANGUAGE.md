# API ubiquitous language

| Term | Meaning | Code name | Wire name |
| --- | --- | --- | --- |
| Readiness | Whether the runtime can safely accept work. Liveness is separate and always true while the process serves. | `Readiness.Ready`, `NewHealthHandler` | `/health/live`, `/health/ready` |
| Runtime handler | The loopback HTTP surface: health, epoch controls, metrics and event delivery. | `NewRuntimeHandler` | — |
| Epoch control | Operator requests to drain or kill the running epoch; the work belongs to `control`. | `NewRuntimeHandler` (control routes) | `POST /control/drain`, `POST /control/kill` |
| Event delivery | Server-Sent Events over the durable notification outbox, one cursor per subscriber. | `NewSSEHandler`, `SSEConfig` | `text/event-stream` |
| Subscriber authorization | Decides whether a request may subscribe to a tenant's events. | `AuthorizeSubscriber`, `BearerTokenAuthorizer` | `Authorization: Bearer` |
| Approval selection | A request identifier plus the human decision to be signed. | `ApprovalSelection` | JSON request body |
| Approval submission | A selection with the approver's signature and reason. | `ApprovalSubmission` | JSON request body |
| Problem | The error body of a failed approval request. | `Problem` | JSON `type`, `title`, `status`, `detail` |

Handlers are thin: they translate HTTP into calls supplied by the caller
(`ApprovalConfig.Present`/`Resolve`, the notification store, epoch control) and
hold no business rules.

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Webhook / push | Event delivery | The runtime serves a pull stream from its outbox; it never calls out. |
