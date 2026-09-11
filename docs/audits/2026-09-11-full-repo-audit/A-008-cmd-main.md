# A-008 · `cmd/agentic-stream/main.go`

LOC: 648 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- run-live observes asynchronous worker-runtime failures during the batch, not only errors already buffered when the batch returns.
- run-live wires `EpochControl` into `PipelineConfig` the same way `serve` does, so drain/kill admission guards apply on every live route.
- Worker/evidence/model flags are registered in exactly one place shared by run-live and serve.
- No dead conditions in address validation; telemetry wiring symmetric between run-live and serve.

## Findings
- **MED F1. run-live can mask an evidence-server failure with a successful report** — `cmd/agentic-stream/main.go:192-200,234-236,522-532`. `NewWorkerRuntime` starts the evidence gRPC server whose failures land on `WorkerRuntime.Errors()` (worker_runtime.go:166-170). Unlike serve (main.go:359-368, which drains the channel in a goroutine), run-live registers no drainer and only peeks non-blockingly after the batch finishes; a mid-batch failure that has not yet reached the channel is lost and `events_...=...` success is printed. Fix: start the same select-drain goroutine used by serve in run-live, feeding a local error channel checked before printing the report.
- **MED F2. run-live skips the P8 epoch admission guard it already constructs** — `cmd/agentic-stream/main.go:183,208-211` vs `:372`. `epochControl` is created and passed to `profileOptions.open`, but `PipelineConfig.EpochControl` is unset, so `assemblePending`'s drain/kill refusal (pipeline.go:461-465) is inert on the run-live route while serve wires it. Fix: pass `EpochControl: epochControl` in the run-live `PipelineConfig`.
- **MED F3. ~15 worker/evidence/model flags and vars duplicated between run-live and serve** — `cmd/agentic-stream/main.go:142-148,247-256` vs `:261-266,450-459`. Two hand-maintained copies of flag names, defaults, and usage strings; a change to one silently diverges from the other (already true: only serve validates `--poll-interval`, only serve has `--demo-mode` legitimately, but the worker/evidence/model blocks are identical copies). Fix: extract `addWorkerRuntimeFlags(cmd, *runtime.WorkerRuntimeConfig-flag-targets)` next to `addEffectProfileFlags` in effect_profile.go.
- **LOW F4. Dead `"[::1]"` comparison** — `cmd/agentic-stream/main.go:519`. `net.SplitHostPort("[::1]:8080")` returns `::1` without brackets, so `host == "[::1]"` can never match. Drop it; `::1` is already checked.
- **LOW F5. run-live configures OTLP but no pipeline telemetry** — `cmd/agentic-stream/main.go:163-167,208-211` vs `:328,371`. run-live builds a tracer provider yet never constructs `telemetry.NewRuntime` nor passes `Telemetry` to `PipelineConfig`, so pipeline metrics/watch telemetry are silently absent on that route. Fix: construct the runtime telemetry in run-live and pass it, or drop the tracer setup there.

## Checked, not an issue
- P1: all errors wrapped `%w`, none swallowed; signal context propagates; server.Shutdown driven by runCtx.Done with 5s timeout; deferred closes ordered correctly.
- P2: subscriber/control tokens via env not flags; non-loopback listen refused; effect-profile and worker-config validation run before sockets open.
- P3: `config effective` is a documented design placeholder (TECHNICAL_DESIGN.md, cli.md), not dead code; `validateServeSources` logic verified truth-table-correct.
- P4: main wires via `runtime.NewService`/`NewPipeline`/`NewWorkerRuntime`; no business logic in handlers.
- P6: main_test.go covers serve source validation, loopback check, version; effect_profile_test.go covers profile validation.
- P7: run-live and serve both drive the same `runtime.Pipeline`; no second pipeline implementation.
