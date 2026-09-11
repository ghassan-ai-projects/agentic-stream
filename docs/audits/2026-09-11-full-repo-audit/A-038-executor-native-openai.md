# A-038 · `internal/executor/native/openai.go`

LOC: 242 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The HTTP client used has a timeout, and no code path can block indefinitely.
- Streamed turns request and report usage so token/cost budgets and cost settlement see real consumption.
- The API key appears only in the request header, never in errors or logs.

## Findings
- **[HIGH] F1. Nil `Client` falls back to `http.DefaultClient`, which has no timeout** — `internal/executor/native/openai.go:51-54`. The only production construction site sets no client (`internal/runtime/worker_runtime.go:125`), so every real model call runs on `http.DefaultClient`. Combined with the optional wall-time budget (`internal/spec/spec.go:201` is not required; `internal/episodes/executor.go:695` explicitly handles `wall_time == ""`), a stalled TLS handshake or a silent provider can hang the episode worker indefinitely with no deadline at any layer. Violates P1 (HTTP clients have timeouts). Fix: default to a package-level client with explicit timeouts (dial/TLS/response-header), and reject a supplied `Client` whose `Timeout` is zero.
- **[HIGH] F2. Streaming requests never ask for usage, so token/cost budgets are structurally unenforced** — `internal/executor/native/openai.go:36-39,232-242`. The request is always `Stream: true` but does not set `stream_options.include_usage` (or an equivalent); OpenAI-style endpoints omit `usage` from stream chunks unless it is requested. `responseUsage` then yields zeros for every streamed turn, `checkUsage` (`native.go:477-488`) can never trip `input_tokens`/`output_tokens`/`cost_microunits`, and `Outcome.CostMicrounits` settles as 0 into `costcontrol.Settle` (`internal/episodes/executor.go:492`) — the aggregate ceiling and kill switch never see real spend on the default provider path. Violates P7. Fix: add `stream_options: {"include_usage": true}` to the streamed request and fail (or estimate loudly) when a completed turn reports zero usage.
- **[LOW] F3. `buildUserContent` swallows the marshal error** — `internal/executor/native/openai.go:107`. `raw, _ := json.Marshal(document)` cannot fail for this document today, but the swallowed error would silently drop the entire objective/observation payload into an empty user turn. Return the error like the request marshal at line 40-42 does.
- **[LOW] F4. `sort.Ints` instead of `slices.Sort`** — `internal/executor/native/openai.go:215`. P5 names the stdlib `slices` package as the default.

## Checked, not an issue
- P1: request built with `http.NewRequestWithContext` (43); response body always closed (59); error-path body and JSON decode capped (16 KiB / 16 MiB limits, 61, 156); SSE scanner token bounded at 1 MiB (170); retryable classification is explicit (429/5xx → `RetryableError`, 63-65) with the count capped by the executor's `ProviderRetries` budget.
- P2: API key only in the `Authorization` header (48-50), never logged or embedded in errors; provider error bodies are provider output rendered as a Go error string, never executed or re-sent as instructions; tool declarations are the executor's allow-listed set (`providerTools` receives only matched definitions, `native.go:383-411`); no effectors or credentials reachable.
- P3: no dead symbols — all types/functions are used by the `Stream` path or tests.
- P4: pure provider adapter; no tool execution, no storage, no policy awareness.
- P5: exported symbols documented; errors wrapped `%w`.
- P6: `openai_test.go` covers SSE accumulation, tool-call assembly, usage fallbacks, and retryable classification.
- P7: usage accumulation deterministic; no wall-clock decisions inside the adapter.
