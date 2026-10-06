# Remote executor ubiquitous language

| Term | Meaning | Code name | Wire name |
| --- | --- | --- | --- |
| Remote executor | Adapts the streamed `EpisodeWorker` protocol to the episode `Executor` port: maps durable requests to the wire, issues the evidence capability, accounts the budget, and accepts a Decision only with a matching terminal. | `Executor`, `NewExecutor`, `NewExecutorWithEvidence` | `EpisodeWorker.Execute` |
| Handshake | The version and feature check run before every episode. | `Execute` | `Handshake` |
| Attempt capability | A short-lived evidence capability bound to the exact attempt identity and trace. The raw token exists only in the dispatch call and is never stored or logged. | `AttemptCapabilityIssuer`, `CapabilityFactory` | capability token |
| Budget usage | The runtime's trusted count of what a worker consumed: model calls, tool calls, retries, bytes, tokens, cost. It takes the maximum of event-derived and worker-reported totals, so a worker cannot under-report past a limit. | `budgetUsage` | `EpisodeBudget`, budget events |
| Stream state | The validated sequence of events: one started, at most one Decision, one terminal, nothing after. | `stream.go` | `EpisodeEvent` |
| Terminal | The event that classifies how the attempt ended and gives the reason code. | `Terminal` | `Terminal` |
| Transport error | A failure of the connection, as opposed to a worker-reported failure. | `transport_error.go` | gRPC status |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Agent, client (for the worker) | Worker | The worker is the remote party; the runtime is the client. |
| Credential (for the capability) | Attempt capability | It grants read-only evidence for one attempt, not access to a system. |
