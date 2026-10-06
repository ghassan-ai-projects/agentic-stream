# Package merge review

Rule applied while migrating packages to the reference-module layers: when a
layer or package would be tiny, merge it instead of creating it, unless a
dependency rule or a cross-repository contract forbids the merge.

## Done

| Decision | Evidence |
| --- | --- |
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
| `scheduleledger` into `episodeledger` | Distinct tables and owners; `cognition`'s pure layers import only the scheduler item types. |
| `approvalledger` into `policy` | `cognition` withdraws approvals on supersession and would have to import policy, which reasoning layers are forbidden to reach. |
| `qualification` into `control` or `episodes` | Different owners and tables; `replay`, `runtime` and `episodes` import `qualification` without `episodes`. |
| `admission` into `episodes` | Admission imports `cognition`, which sits above `episodes` in the layer graph. |
| `api` | Already a thin HTTP/SSE adapter with no tables; the playbook exempts thin adapters. |

## Not migrated, and why

`control`, `qualification`, `episodeledger`,
`scheduleledger` and `approvalledger` are shared, transaction-scoped stores:
stateless functions over a caller's transaction that other modules' stores call.
They already are the store layer of their tables. Giving each facade/app/domain
layers would lift them above 20 dependants and renumber the layer table for no
behavior gain. If that is wanted later, re-level the table from the import
graph first. `spec`, `storage`, `telemetry`, `worker`, `runartifact`, `soak`,
`executor/*`, `operators` and `situations` are pure rule sets, infrastructure or
adapters.
