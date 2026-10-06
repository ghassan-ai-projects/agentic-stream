# Run artifact ubiquitous language

| Term | Meaning | Code name | File name |
| --- | --- | --- | --- |
| Run artifact | An immutable directory that is a consistent, independently verifiable view of one database run. An evidence projection, never a write path. | `Export`, `Verify` | artifact directory |
| Export | Reads one snapshot in a single read-only transaction and publishes the directory. It never overwrites an existing directory. | `Export`, `Options` | — |
| Manifest | The operator-supplied and database-derived provenance header. An empty optional value means the owning system did not provide it; it is never replaced by a physical claim. | `Manifest` | `manifest.json` |
| Device identity | The physical or emulated device included in the run. | `DeviceIdentity` | `manifest.device` |
| Worker metadata | Non-secret worker provenance needed to interpret a decision: prompt version and digest, decision schema, provider, model, sampling. | `WorkerMetadata` | `manifest.worker` |
| Ledger file | One exported table projection, in JSON Lines. | `domain.LedgerFiles` | `observations.jsonl`, `situations.jsonl`, `decisions.jsonl`, `commands.jsonl`, `device-results.jsonl`, `feedback.jsonl`, `device-command-bindings.jsonl`, `authority-events.jsonl`, `safety-events.jsonl` |
| Bound definition | The canonical spec and policy the run executed under. | `app.addBoundDefinitions` | `spec.canonical.json`, `policy.canonical.json` |
| Soak report | The safety verdict derived from durable evidence only: zero-tolerance counters, evidence completeness and diagnostics. Telemetry is excluded. | `domain.SoakReport` | `metrics.json` |
| Zero tolerance | The counters that fail the run on any non-zero value: unsafe output, stale energizing effect, duplicate net energizing effect, unexplained actuator transition, false verified success, safe-state deadline miss. | `domain.ZeroTolerance` | `metrics.json` |
| Evidence completeness | The share of physical transitions that carried complete independent evidence; below one fails the run. | `domain.EvidenceCompleteness` | `metrics.json` |
| Verdict | `pass` or `fail` with sorted failure reasons. | `SoakReport.Verdict`, `domain.failureReasons` | `verdict.json` |
| Verification | Re-reads an artifact directory and checks digests, ledger consistency and the verdict against the files. | `Verify` | — |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Soak package, `soak.Compute` | Soak report | `soak` was merged into this package; only the tenant-scoped report remains. |
| Bundle, dump | Run artifact | An artifact is immutable and verifiable; a dump is neither. |
