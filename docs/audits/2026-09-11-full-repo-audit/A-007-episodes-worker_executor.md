# A-007 · `internal/episodes/worker_executor.go`

LOC: 682 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- `budget.wall_time` is parsed and validated once per `Execute`, not twice with different failure modes.
- Wall-clock reads go through an injectable clock consistent with the rest of the runtime.
- No privileged side effect (capability token issuance) happens before a fail-closed validation that can reject the request.
- Standard-library helpers are preferred over hand-rolled equivalents.

## Findings
- **MED F1. `wall_time` parsed twice per `Execute` with divergent semantics; drifts from the Runner's deadline gate** — `internal/episodes/worker_executor.go:91, 326-346, 533-539`; mirrors `internal/episodes/executor.go:689-703` and `internal/episodes/assembler.go:322-331`. `boundedExecutionContext` hard-fails an invalid `wall_time`; 40 lines later `episodeRequest` re-parses the identical field with its own hard-fail; the Runner's `deadlineExceeded` silently no-ops on the same input. These are parallel implementations of one budget contract and can diverge (e.g., one side tightening validation). Fix: parse the budget once into typed `Request` fields at bind time; all three consumers read that.
- **LOW F2. `time.Now()` used directly for the wire deadline** — `internal/episodes/worker_executor.go:540-543`. `WorkerExecutor` has no clock injection while `Runner`, pipeline, and dispatchers all take `clock.Clock`; the `Deadline` written to the worker is uncontrollable in tests and inconsistent with the runtime's clock discipline. Fix: accept a `clock.Clock` (or the dispatch timestamp already carried by `Runner.startedAt`).
- **LOW F3. Capability token issued before the negotiated-feature check** — `internal/episodes/worker_executor.go:108-118`. The ephemeral evidence capability is minted (109-115) and attached to the wire request before the feature check at 116-118 fails the request for a non-negotiated `EvidenceToolsFeature`. A credential-bearing side effect runs on a path that is about to fail closed; the check also lives after the endpoint/factory validation at 96-103 where it belongs. Fix: move the feature check above token issuance.
- **LOW F4. `containsFeature` reimplements `slices.Contains`** — `internal/episodes/worker_executor.go:380-387`. `go.mod` declares go 1.26.5; the repo style mandates stdlib `slices`/`maps`/`cmp`. Fix: `slices.Contains(e.requestedFeatures, worker.EvidenceToolsFeature)` and delete the helper.

## Checked, not an issue
- Not a duplicate execution path: `Runner` (executor.go) is durable orchestration/fencing/persistence; `WorkerExecutor` is the gRPC transport adapter implementing `Executor`. Complementary layers, not parallel runtimes (the only real duplication is F1).
- P2/P1: stream consumption is fail-closed — protocol/contract version, worker identity, per-event identity+sequence, started-first, no post-terminal events, negotiated size limits, digest-verified decision before acceptance (150-266); handshake and recv errors are wrapped and returned unchanged for cancellation/timeout classification.
- P7: budgets enforced from server-side counters (`budgetUsage.observe`), worker budget updates cross-checked (`validateBudgetUpdate`), missing budget telemetry fails closed (`budgetTelemetryMissingError`), cost ceiling requires usage evidence (231-238).
- P5: exported symbols documented; digests over canonical JSON (`verifyDecisionDigest`); untrusted worker bytes are copied, never executed as parameters.
- P6: worker_executor tests cover handshake, stream limits, duplicate events, budget exhaustion, and evidence capability issuance; conformance suite exercises the client end-to-end.
