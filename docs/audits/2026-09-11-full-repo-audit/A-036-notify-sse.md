# A-036 · `internal/notify/sse.go`

LOC: 260 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- The handler fails closed when authorization is not configured; an unauthenticated request can never read any tenant's notifications.
- Mid-stream error reporting carries the same generic codes as the pre-stream path, not raw internal error strings.

## Findings
- **[MED] F1. Authorization is fail-open by default** — `internal/notify/sse.go:89,78-84`. `serveSSE` skips the authz check entirely when `cfg.Authorize == nil`, and falls back to `r.URL.Query().Get("tenant")` when no tenant binding is configured. A wiring mistake (`NewSSEHandler(SSEConfig{DB: db})`) silently publishes every tenant's durable notifications to anonymous callers with cross-tenant selection via a query parameter. The production wiring does set both (`cmd/agentic-stream/main.go:421-424`, and an empty bearer token fails closed inside `BearerTokenAuthorizer`), but the infrastructure component's default direction violates the safety bar. Fix: in `NewSSEHandler`, reject configuration with nil `Authorize` (or return a handler that 503s), and drop the query-param tenant fallback.
- **[LOW] F2. Mid-stream control event leaks the raw internal error string to the client** — `internal/notify/sse.go:138-141`. `writeControl(..., map[string]any{"detail": readErr.Error()})` ships driver/storage error text (DSN fragments, SQL details) to the subscriber, while the pre-stream path deliberately maps errors to generic problem codes (230-241). Fix: send only `streamErrorCode(readErr)` in the control event and log the detail server-side.

## Checked, not an issue
- P1: request ctx honored for shutdown (130-131); tickers stopped; write errors terminate the stream; `BearerTokenAuthorizer` is constant-time (`subtle.ConstantTimeCompare`, 27) and fails closed on an empty expected token; headers committed only after the first successful read (103-118) so expired cursors get a real problem response.
- P2: no execution surface — events are validated CloudEvents re-marshaled canonically (199); the in-stream `seen` dedup is bounded (172-175); method restricted to GET; event-type allow-list enforced per event (189-196).
- P3: no dead code; `streamErrorCode` and `writeStreamError` are the two required rendering forms (problem response vs control event), not divergence beyond F2.
- P4: infrastructure delivery plane over the `notify` store; no domain branches.
- P5: `Last-Event-ID` then `?cursor` resolution documented and bounded (150-163); exported symbols documented.
- P6: `sse_test.go` covers resume, dedup, authz, and error mapping.
- P7: cursor arithmetic is integer and monotonic; `cfg.Now` injectable; delivery is at-least-once by contract and documented as such (47).
