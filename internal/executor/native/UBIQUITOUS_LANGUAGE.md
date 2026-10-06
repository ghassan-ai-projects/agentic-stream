# Native executor ubiquitous language

| Term | Meaning | Code name | Stored as |
| --- | --- | --- | --- |
| Native executor | The in-process Go episode executor. It owns the bounded model/tool loop and never mutates episode, situation or action state. | `Executor`, `New` | — |
| Model provider | The narrow port that proposes one model response per call. Providers propose; they never execute. | `ModelProvider.Stream` | — |
| Deterministic provider | A scripted provider for replay and tests that otherwise returns an empty-intent Decision. | `DeterministicProvider` | — |
| OpenAI-compatible provider | The HTTP provider for chat-completions endpoints, with streamed responses; a provider adapter only. | `OpenAICompatibleProvider` | — |
| Tool | A read-only capability the model may call. It must not dispatch effects. | `Tool`, `ToolDefinition`, `ToolCall`, `ToolResult` | — |
| Evidence tool | A tool over bounded, read-only event evidence, bound to the trusted request's tenant and entity. | `SQLiteEvidenceTool` | event log (read) |
| Observation | A tool call's result as the model sees it, with an artifact reference when the result is large. | `Observation`, `ArtifactRef` | artifact store |
| Usage | Input tokens, output tokens and cost in microunits reported by a provider response. | `Usage` | — |
| Retryable error | A provider failure that may consume the separate provider-retry allowance; model-call and cost budgets still grow. | `RetryableError` | — |
| Interrupt | A provider or tool interruption. Non-interactive episodes fail immediately and never wait for input. | `ErrInterrupt` | — |
| Repair | At most one attempt to fix structured output that failed validation. | `Config` | — |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Agent | Native executor | The executor is a bounded loop with no authority over effects. |
| Batch runner (`RunBatch`) | — | Served an external benchmark harness; no caller here, slated for deletion. |
