# Plan and documentation changes

Each row changes what the plans promise, so that the design describes what
exists plus an explicit deferred list. Rows marked "with Uxx" are applied in
that task's commit; the others are documentation-only and can land together.

Status values match the [task board](README.md#task-board).

## P01 — One SituationSpec schema, not two

Status: todo · with: none

**Finding.** `docs/design/contracts/situation-spec-v1.schema.json` and the
embedded runtime schema `internal/spec/internal/domain/schema.json` have drifted:

| Surface | Design contract | Runtime accepts |
| --- | --- | --- |
| top-level blocks | adds `retention`, `telemetry` | neither |
| `operator.kind` | `map`, `filter`, `aggregate`, `rate`, `slope`, `correlation`, `duration`, `missing_heartbeat` | `aggregate`, `slope`, `missing_heartbeat` |
| `operator.aggregate` | 15 functions incl. `variance`, `stddev`, `first`, `last`, `delta`, `rate`, `quantile`, `correlation` | `mean`, `rms`, `slope`, `count`, `sum`, `min`, `max`, `latest` |
| `reducer.strategy` | 7 incl. `min`, `max`, `bounded_append`, `weighted_confidence`, `source_priority` | `latest_event_time`, `set_union` |
| `window.kind` | `tumbling`, `sliding`, `count`, `decay` (+ `count`, `halfLife` fields) | `tumbling`, `sliding` |
| `executor`, `intent`, `input`, `actions` | — | add `dispatchPolicy`, `riskCeiling`, `skills`, `prompt`, `diagnosisCatalog`, `parameterSchema`, `presets`, `modelWritableFields`, `compensation`, `schema`, `watch_confidence_floor` |

**Decision: remove the design copy.** Point every link to the embedded schema,
or generate the design copy from it with a drift check. **Remove** the
unimplemented kinds from the plan rather than schedule them.

**Reasoning.** Two hand-maintained copies of one contract will drift again.
The runtime schema is what authors are validated against. None of the missing
kinds is needed by the predictive-maintenance proof or the HIL slice. Each is
an operator with its own determinism and late-data semantics, and adding them
should start from a domain that needs them. The limitations page already says
retention and telemetry controls are not exposed "until the runtime can enforce
it". Record the removed kinds in one line under TECHNICAL_DESIGN §24.

## P02 — TECHNICAL_DESIGN §15: the real CLI and API

Status: todo · with: U09, U13–U23

**Finding.** §15.2 lists 17 CLI forms; 4 exist. §15.1 lists 18 HTTP endpoints;
2 exist as written (`/health/live`, `/health/ready`). The runtime also serves
`/v1/events` (SSE), `/v1/approvals/{id}`, `/metrics` and `/control/{drain,kill}`,
which §15.1 does not list.

**Decision per promised surface:**

| Promised | Decision | Reason |
| --- | --- | --- |
| `validate`, `run`, `run-live` | keep | exist |
| `config effective` | remove (U09) | placeholder only |
| `situation list/show`, `explain situation/trigger` | complete (U22) | MVP explainability |
| `episode show` | complete (U23) | MVP explainability |
| `replay <range> --mode` | replace with `run --mode recorded\|shadow` and `--repeat` (U19–U21) | one replay command; replay always runs from a trace in an isolated database (§16.2), so "range over the production log" is dropped |
| `intent approve\|deny` | remove | approval is a signed relay/approver HTTP flow; a CLI shortcut would bypass the separation of principals |
| `compare <a> <b>` | remove | `--repeat` gives hash equality; shadow gives the model comparison; nothing consumes a generic diff |
| `init`, `doctor` | remove | no defined behavior; `validate` and `/health/ready` cover the checks |
| `ingest <events.jsonl>` | remove | `run-live` and the live socket are the ingest paths |
| `simulate predictive-maintenance` | remove | the simulator trace format is accepted by `run-live --trace-format simulator`; generating traces belongs to the Streams Simulator repo |
| HTTP `/v1/situations…`, `/v1/triggers/{id}/explain`, `/v1/episodes…`, `/v1/intents` | defer | CLI first (§24: UI after CLI workflows settle); each route needs a credential scope |
| HTTP `POST /v1/intents/{id}/approve\|deny` | replace with the implemented `/v1/approvals/{id}` | exists, signed |
| HTTP `POST /v1/events` | remove | ingress is file and Unix socket; network ingress is ADR-015 (proposed) |
| HTTP `/v1/replays`, `POST /v1/specs/validate`, `POST /v1/specs/deploy` | remove | replay is isolated and offline; specs are deployed by starting `serve --spec` |
| HTTP `/v1/events/stream` | rename to the implemented `/v1/events` | exists |

Also remove the "First divergence" requirement in §16.3 for deterministic
replay; keep the model-comparison bullets, which shadow mode covers.

## P03 — TECHNICAL_DESIGN §14.3 and §14.5

Status: todo · with: U05, U17

- **§14.3 Artifacts: remove.** No artifact store was built; oversized tool
  results fail closed (U05). Keep the reserved `artifacts` table note in the
  persistence contract (U11).
- **§14.5 Retention: rewrite** to what exists: notification retention by
  `notifications prune` (U17). Raw events, features, Situation versions,
  decisions, commands and outcomes are **not deleted in v1**, because replay
  and audit depend on them. **Remove** "legal-hold checks": there is no
  legal-hold model or requirement source. Deleting evidence becomes a §24
  deferred item that needs its own design (it interacts with replay).

## P04 — Counterfactual mode

Status: todo · with: U02

Remove counterfactual mode from: IMPLEMENTATION_PLAN M3.5 ("counterfactual
simulated effector"), BUILD_COMPLETION_BAR Gate C and the end-state paragraph,
`documentation/design/replay-and-shadow.md`, `documentation/overview/concepts.md`,
`documentation/overview/status.md`, `documentation/learn/safe-actions.md`,
`documentation/architecture/durability.md`, `docs/eval/01-measurement-model.md`
(A9) and `02-suite-catalog.md` (S5.2). Rename `docs/eval` K2/S7.4 "counterfactual
regret" to "regret against the deterministic baseline": it is computed from
shadow comparisons, not from a simulator. Invariant 9 keeps its wording: it
permits an explicit simulation mode but does not require one.

## P05 — IMPLEMENTATION_PLAN M3.5 and the MVP list

Status: todo · with: U19–U21

Rewrite M3.5 to: deterministic replay with `--repeat`; recorded replay against
a read-only source database; paired shadow replay over the worker protocol;
isolated replay database. Drop "CLI `replay` and `compare`" and "comparison
locates first Situation divergence" (P02). The design README MVP list stays
unchanged; U19–U23 are what make it true.

## P06 — Approval and calibration inputs

Status: todo · with: U15, U24

- `documentation/reference/http-api.md`: replace "must be provisioned by the
  deployment" with `principals apply` (U15).
- Remove calibrated automation of R2 intents from the plan and the public docs
  (U24). Add one line to §24: automated R2 needs signed calibration evidence
  from the evaluation suite.

## P07 — Status pages and release posture

Status: todo · with: each completing task

Keep `documentation/governance/release-status.json`,
`documentation/overview/status.md`, `limitations.md` and `roadmap.md` in step
with each task:

- `partial` → remove "complete Intent policy-mode enforcement" (U01), "runtime
  enforcement for spec retention/telemetry controls" (P01: the controls are
  removed, not partial), and "packaged operator inspection, approval,
  reconciliation, and redrive tooling" once U15–U18 and U22–U23 are done.
- `implemented` → "effect-safe replay and shadow library modes" becomes
  "deterministic, recorded and shadow replay from the CLI" after U21.

## P08 — Retire the earlier dead-code records

Status: todo · with: none

Add a one-line "Superseded by docs/unfinished-work-review-2026-10-08" status to
`docs/remaining-migration-2026-10-06/DEADCODE.md`, `DEADCODE_REPORT.md`,
`docs/reference-module-migration-status-2026-10-06/DEADCODE.md`, and FOLLOW_UPS
#1, #4, #13 and #17. Mark the HIL Phase 02 "implemented" claim as "library only
until U21". Link this folder from `docs/README.md`.
