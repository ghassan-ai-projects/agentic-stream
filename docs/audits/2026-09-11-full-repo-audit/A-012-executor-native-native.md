# A-012 · `internal/executor/native/native.go`

LOC: 560 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- Cost accrued before a timeout/cancel terminal is carried into `Outcome.CostMicrounits` so `costcontrol.Settle` records real spend.
- The executor refuses to run an episode that has no finite ceiling at all (wall_time or model_calls).
- Provider retries apply bounded backoff, not a tight immediate loop.
- No swallowed errors on the decision-emission path.

## Findings
- **[HIGH] F1. Timeout/cancel terminals zero out accrued cost, under-counting aggregate spend** — `internal/executor/native/native.go:490-495`. `terminalForContext` calls `failed(req, "timed_out", Usage{})` and builds the canceled outcome with no cost, discarding the `usage` accumulated across completed model calls (`executeLoop` lines 317, 372, 498 all preserve it). The outcome cost flows to `costcontrol.Settle` (`internal/episodes/executor.go:492`), so a wall-time timeout settles actual spend as 0: `spent_micro` never rises, `reserved_micro` is released, and the aggregate ceiling/kill switch under-trips exactly for the episodes that burned the most provider budget. Violates P7 (budgets enforced). Fix: thread the current `usage` into `terminalForContext` and use it for both terminal branches.
- **[MED] F2. All-zero budgets are accepted, giving an unbounded loop** — `internal/executor/native/native.go:234-243,275-294`. `decodeRequest` requires only snapshot and decision schema; the spec does not require any budget (`internal/spec/spec.go:187,201`, no validation). With `wall_time == ""` and zero caps, `executeLoop` is bounded by nothing: model calls, tool calls, observation memory, and retries are all unlimited until an external ctx cancel. P7 says budgets are finite. Fix: in `Execute`, require `budget.WallTime != "" || budget.ModelCalls > 0` (or both) and reject otherwise.
- **[MED] F3. Provider retries are an immediate tight loop with no backoff** — `internal/executor/native/native.go:302-309`. On a `RetryableError` (429/5xx classified in `openai.go:63-65`) the loop `continue`s instantly, up to `ProviderRetries` times against an endpoint that is actively rate-limiting or failing. Each retry also consumes `model_calls` (line 293), which caps the blast radius, but the discipline is still hammer-and-retry. Fix: wait a bounded fixed delay (timer honoring ctx) before retrying.
- **[LOW] F4. `mustCanonical` swallows the marshal error** — `internal/executor/native/native.go:469-472`. `raw, _ := canonicaljson.Marshal(document)` returns nil on error while `DecisionSHA256` (line 368-372) is already computed from the pre-canonical document, yielding an outcome with a digest and empty `DecisionJSON` that fails downstream with an unrelated error. Unreachable for JSON-decoded maps today, but it is a swallowed error on the money path. Fix: propagate the error into `failed(req, "decision_digest_failed", usage)`.
- **[LOW] F5. Stale `Config` doc and anonymous-struct budget alias** — `internal/executor/native/native.go:130-137,263-273`. The `Config` comment claims "controls hard ceilings ... Zero means unlimited for that dimension", but `Config` has no ceiling fields — ceilings come from the request payload. `type budgetConfig = struct{...}` (alias to an anonymous struct) is used instead of a named type. Fix: rewrite the comment to describe Provider/Tools/ArtifactStore/ToolFactory, and name the struct.

## Checked, not an issue
- P1: errors wrapped `%w` throughout; ctx honored before each provider turn and tool call and re-checked after `Stream` returns (286, 314, 327); `DeterministicProvider`/`MemoryArtifactStore` mutex-guarded, no races.
- P2: tools are read-only by interface contract; unknown tool names rejected (`tool_not_allowed`, 332); intent types checked against the allowed list (447-457); decision identity bound to episode/attempt/fence/snapshot digest (444); full schema validation happens downstream in `internal/decisions/validator.go` and `internal/policy`, so partial local validation is defense-in-depth, not the boundary.
- P3: `Recover`-style dead code absent; duplicate tool-name rejection at construction (158-160); repeated-identical-tool-call detection (337-342) prevents provider loop pinning.
- P4: executor stays behind episodes, returns typed `episodes.Outcome`, never touches situations/actions; provider is a narrow port.
- P5: exported symbols documented; `slices.SortFunc`/`slices.Contains` used (410, 454).
- P6: `native_test.go`, `openai_test.go`, `batch_test.go` cover the loop, budgets, and repair.
- P7: token/cost budgets checked after every turn (317-320, 477-488); tool result bytes totaled and capped (355-358); wall-time applied via `context.WithTimeout` (250-252).

## Resolution (2026-09-12) — FIXED

- **F1 (HIGH)** fixed: accumulated provider usage is passed into timeout and cancellation terminals, including usage returned by a provider immediately before a late context check. `TestNativeExecutorEnforcesWallTimeAfterLateProviderResponse` and `TestNativeExecutorPreservesUsageOnCancellation` prove the resulting cost is retained.
- **F2** fixed: execution rejects a request with neither a positive `wall_time` nor a positive `model_calls` ceiling before making a provider call. `TestNativeExecutorRejectsUnboundedRequest` proves the provider is not invoked.
- **F3** fixed: retryable provider failures wait on a bounded, cancellation-aware backoff before retrying. `TestNativeExecutorBacksOffProviderRetries` proves the retry is delayed and remains bounded by the request budget.
- **F4** fixed: canonical decision marshaling errors now produce a failed outcome with the accumulated usage instead of being discarded.
- **F5** fixed: the `Config` documentation describes its actual dependencies and the anonymous budget alias is now a named type.
- Verified: `go test -race ./internal/executor/native` passes.
