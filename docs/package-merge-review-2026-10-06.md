# Package merge review

Rule applied while migrating packages to the reference-module layers: when a
layer or package would be tiny, merge it instead of creating it, unless a
dependency rule or a cross-repository contract forbids the merge.

## Done

| Decision | Evidence |
| --- | --- |
| `scheduleledger` merged into `episodeledger` | Same transactions and callers; 220 lines cannot carry four layers. See [ledgers record](ledgers-reference-module-2026-10-06/MERGE_DECISION.md). |
| `qualification` dissolved | Three unrelated parts with one user each: shadow decisions (`episodes`), shadow comparisons (`replay`, which already had an identical `Comparison` type) and the calibration check (`policy`); the unused `Activate` was deleted. |
| `costcontrol` merged into `control` | One control plane: `control.Kill` already settled cost, and every cost caller also asks `control` about the epoch. The cycle through `episodeledger` was cut with a `CostSettler` port. See [control record](control-reference-module-2026-10-06/README.md). |
| `notifycontract` merged into `notify/internal/domain` | The Go package was `internal/` and only `notify` imported it; the JSON contract files moved with it and docs were relinked. Merging removed a duplicated type list and a layer. See [notify record](notify-reference-module-2026-10-06/README.md). |

### Earlier decisions

| Decision | Evidence |
| --- | --- |
| No `wire` layer for `actions`, `watch`, `engine` or `ingress`; document and payload checks live in `domain` | Each codec is a few schema, digest or integer checks over `canonicaljson`/`contractsv1` with no I/O; `policy` already keeps its documents in domain. `device` and `evidence` keep `wire` because they encode protocol frames and protobuf. |
| `watch` keeps its own module | `actions` may not reach concrete effectors; reasoning and replay layers may not reach effect implementations. |

## Considered and rejected

| Candidate | Reason |
| --- | --- |
| `interlock` into `control` | 100-line layer-0 leaf imported by `policy`, `actions` and `watch`, which would otherwise inherit control's transitive imports (episodes ledger, cost control, storage). |
| `approvalledger` into `policy` | `cognition` withdraws approvals on supersession and would have to import policy, which reasoning layers are forbidden to reach. |
| `admission` into `episodes` | Admission imports `cognition`, which sits above `episodes` in the layer graph. |
| `api` | Already a thin HTTP/SSE adapter with no tables; the playbook exempts thin adapters. |

## Not migrated, and why

None of the shared transaction-scoped stores remain in their old shape: `notify`, `control` (with
`costcontrol`), `episodeledger` (with `scheduleledger`) and `approvalledger` were layered, and
`qualification` was dissolved. `spec`, `storage`, `telemetry`, `worker`, `runartifact`, `soak`,
`executor/*`, `operators` and `situations` are pure rule sets, infrastructure or adapters (see the
follow-ups for the shape review of the executors, `runartifact`, `worker` and `soak`).
