# A-031 · `internal/runtime/worker_runtime.go`

LOC: 286 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- Each validation rule is stated once; no flag combination is validated twice in one construction path.
- Identical constants are built once, not re-derived per branch.

## Findings
- **LOW F1. Evidence-key validation duplicated inside the same construction path** — `internal/runtime/worker_runtime.go:67-72` and `:144-150`. `ValidateWorkerRuntimeConfig` (called at :101) already hex-decodes `EvidenceKey` and enforces >=32 bytes; `NewWorkerRuntime` then decodes and re-checks the identical rule before using the secret. The copy can drift from the canonical check. Fix: delete the inline re-validation and reuse the decoded secret (or a small `decodeEvidenceKey` helper called once).
- **LOW F2. `evidenceIssuer` literal rebuilt per branch** — `internal/runtime/worker_runtime.go:161,185`. The identical issuer struct (issuer/audience/keyID/keys) is constructed twice — once for the gRPC verifier registration and once for the capability factory. Fix: build once before the evidence block and reuse.

## Checked, not an issue
- P1: errors wrapped `%w`; partially initialized instances cleaned up via the `cleanupOnError` deferred `Close`; `Close` nils fields so it is idempotent and joins all error paths via `errors.Join`; no data races (evidence server error channel is buffered and single-producer).
- P2: tamoz invariant enforced and test-pinned — the native executor is never constructed when `WorkerSocket` is configured (:122-140, TestExecutorSelectionSkipsNativeOnTamoz); evidence server binds only a private UDS path (worker.ListenEvidenceSocket) and requires ledger + HMAC capability (>=32-byte key, mTLS-capable worker dial, TLS 1.3 minimum); evidence queries are bounded by the capability's tenant/entity/window/MaxRows.
- P3: `newNativeExecutor` package var is a documented, test-used seam for the P1 gate-8 adversarial proof; `ValidateWorkerRuntimeConfig` is reused by serve for flag-time rejection; no dead exported symbols.
- P4: composition only — executors, ledger, and query wiring; the evidence SQL query is the declared `evidence.Query` extension point, not engine leakage.
- P6: worker_runtime_test.go covers config validation and both executor-selection routes.
- P7: one construction path shared by run-live and serve (`WorkerRuntimeConfig`), preventing executor/evidence boundary drift.
