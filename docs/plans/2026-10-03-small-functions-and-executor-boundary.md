# Small functions and executor boundary rounds

Baseline: `6e6e2b8`, branch `code-improvments-3`. The untracked `agentic-stream`
executable is outside this work. No push is requested.

## Goal

1. Q2/Q7: every production function body is at most 15 lines and 15 statements,
   reads top-down as named domain steps, and keeps one level of abstraction.
2. A8: the episode lifecycle depends only on the `Executor` port; the streamed
   worker adapter moves to `internal/executor/remote`.
3. A9: every production package documents its responsibility, and the public
   module map lists every package.

Baseline measurement: 503 production functions exceed 15 body lines across 39
packages; 8 packages lack a package comment; `episodes` imports gRPC, protobuf,
the worker protocol and `internal/worker`.

## Module assessment

The packages already follow business capabilities (ADR-017). Two findings:

- `episodes` mixes lifecycle ownership with one concrete executor transport.
  This is a real module boundary, so it is extracted (A8).
- `device` is cohesive: the session consumes a `DeviceTransport` interface and
  the UDS transport implements it for the same gateway. No split is warranted.

No other package is split: package count is not a quality target.

### Second assessment: clean modules and one-way flow (after round 3)

Imports are strictly downward and pinned. Looking past imports, at callbacks,
interface use, SQL location and who calls whom at runtime:

| Finding | Evidence | Decision |
| --- | --- | --- |
| Composition root owns episode admission | `runtime/pipeline_admission.go`: drain gate, scheduler query, assembly, fixture refusal, epoch stamp, skip rules | Extract `internal/admission` (A10) |
| Composition root issues domain SQL | `runtime` reads `intents`, `cost_limits`, `scheduler_items`, `event_log` | Owning modules expose the reads (A10) |
| Duplicate evidence query | Identical `event_log` window query in `runtime` and `executor/native` | `eventlog` owns one entity-window read |
| Reverse runtime flow through the dispatcher | `actions` owns watches; the ingest loop calls `actions.WatchEffector.FireEvent` | Extract `internal/watch` (A11) |
| Effect adapter inside the dispatcher | `actions.SimulatedEffector` | Move to `device` with the other adapters (A11) |
| Unused, misplaced episode tool host | `policy.CapabilityHost` has no production caller | Reported; not removed in this work |

Callbacks are otherwise clocks, the control-built readiness check, and tool
factories injected downward by composition. Interfaces declared in lower modules
(`episodes.Executor`, `actionport.Effector`, `device.DeviceTransport`) are
implemented by adapters and invoked in the forward direction of the data flow.
Replay, soak and run-artifact read across tables as reporting modules; reads
are permitted, writes keep single owners.

## Rounds

- Round 0: tightened Q2 to 15 lines/statements, added A8/A9 and the ADR-017
  executor boundary refinement. Lint enforcement of 15 lines lands when the
  refactoring rounds reach zero findings.

- Round 1 (A9): added package comments for `authority`, `control`, `decisions`,
  `device`, `engine`, `episodeledger`, `qualification` and `scheduleledger`.
  `TestEveryPackageDocumentsItsResponsibility` and `TestModuleMapListsEveryPackage`
  pin the rule; both were checked to fail on a removed comment and a removed map entry.

- Round 2 (A8): moved the streamed EpisodeWorker adapter and the per-attempt
  evidence capability issuer from `episodes` to `internal/executor/remote`
  (`remote.Executor`). `episodes` exports `BudgetExceededError` and
  `BudgetTelemetryMissingError` as the port's failure contract and no longer
  imports gRPC, protobuf, `worker`, `evidence` or the protocol package. The
  adapter maps gRPC `Canceled`/`DeadlineExceeded` onto the matching context
  errors while keeping the original message and status, so durable failure
  reasons are unchanged. New tests pin the mapping and the import rule; the
  architecture allowlist, layers, forbidden edges and reasoning-reachability
  sources include the new package. Focused race tests pass for episodes,
  executors, runtime and replay (coverage: remote 72.1%, episodes 72.7%).

- Round 3 (Q2/Q7): foundation and ledger packages (`approvalledger`, `clock`,
  `costcontrol`, `duration`, `eventschema`, `executor/conformance`, `interlock`,
  `notifycontract`, `qualification`, `scheduleledger`, `storage`, `telemetry`,
  `migrations`) now have no production function over 15 lines. Long SQL moved to
  named constants; cursor loops delegate one row to a named step; the metrics
  snapshot is a named counter table. Error text, ordering and transactions are
  unchanged. Focused race tests and lint pass.

- Round 4: recorded the second module assessment, added A10/A11 and the ADR-017
  composition and effect-adapter refinement.

- Round 5 (A10): extracted `internal/admission` from the runtime pipeline. It
  owns the drain stop, queue-order selection (now `scheduleledger.NextPending`),
  owner-fenced assembly, fixture refusal, single epoch stamp and the recorded
  skips for cost refusal, live reconsideration conflict and fixtures. Error text,
  transaction boundaries and the order of clock reads are unchanged. New
  admission tests cover admission, both fixture modes, drain and cost refusal
  (coverage 75.3%). `TestAllowedImportsHaveNoStaleEdges` now keeps the reviewed
  import graph exact; runtime no longer imports `cognition` or `scheduleledger`.

- Round 6 (A10): the composition roots now contain no SQL
  (`TestCompositionRootsContainNoSQL`, checked to fail on a probe literal).
  Pending-intent selection moved to `policy.NextPendingIntent`; cost-ceiling
  merging to `costcontrol.ApplyCeilings` (its table test moved with it); the
  evidence server's event query to `evidence.EventLogQuery`. Both evidence
  readers (`evidence` and the native executor tool) share one
  `eventlog.ReadEntityWindow`, removing a duplicated `event_log` query. Error
  texts of the shared read are unified under `eventlog`; the ceiling update now
  reads the clock once per configuration transaction. Focused race tests pass.

- Round 7 (A11): extracted `internal/watch` (install, fire, expire; sole owner
  of `watch_conditions` and `watch_fires`) and moved the simulated effector to
  `device`. `actions` is now governed dispatch only and is forbidden from
  importing `device` or `watch`; `watch` is forbidden from reaching dispatch,
  policy, episodes or cognition, and is an effect implementation for the
  reasoning/replay reachability check. The ingest loop now calls the watch
  module instead of the action plane. Behavior and tests moved unchanged; race
  tests pass (coverage: watch 67.0%, actions 70.0%, device 80.5%).

- Round 8: moved Server-Sent Events delivery (HTTP transport) from `notify`
  to `api`. `notify` now owns only the durable outbox, paged reads and
  retention; `api` owns every HTTP surface. The SSE problem/stream_error wire
  details are unchanged. `FakeExecutor` stays in `episodes` for now: an
  in-package episode test depends on it, so moving it to `internal/executor`
  needs a test change and is recorded as follow-up work.

- Round 9 (Q2/Q7): `canonicaljson`, `contractsv1`, `eventlog` and `admission`
  have no production function over 15 lines. Canonical encoding is split into
  scalar, integer, object-member, float-placement and string-escape steps;
  validation into token, key, escape and surrogate steps; CloudEvent and
  envelope checks keep their original error precedence. Golden digest, replay
  and ingress suites pass unchanged under race.

- Round 10 (Q2/Q7): `spec`, `situations` and `operators` have no production
  function over 15 lines. Lifecycle evaluation reads as close → transitions →
  open; reducers moved to `situations/reducers.go` (file cap); window emission,
  boot admission, heartbeat timers and aggregates are named steps; the aggregate
  table replaces a switch. ID-generation order, side-effect order (boot
  admission after event admission) and canonical documents are unchanged; the
  full (non-short) replay/golden suites and runtime/CLI suites pass under race.

- Round 11 (Q2/Q7 + duplication): `engine` and `cognition` have no production
  function over 15 lines. Record application reads as owner check → dedupe →
  operators/situations → operator state → commit; timer firing moved to
  `engine_timer_firing.go`; scheduler admission reads as save → build →
  enqueue-or-defer. Removed duplicated business logic: the CEL `features` view
  is now `situations.CELFeatures`, shared by the situation engine and cognition,
  and CEL bool conversion is `spec.CELBool`. Added `storage.CollectRows` for the
  repeated scan loop (with tests). Preserved clock-read order, ID order, error
  text and transaction scope; caught and fixed an operand-order hazard in
  `buildItem` before commit. Full replay/runtime suites pass under race.

- Round 12 (partial): `ingress` has no production function over 15 lines; file
  replay and the live socket now share one line-admission rule
  (`admitEnvelopeLine`). `evidence` started: capability scope completion split
  into prepare/issuable checks (explicit `IsZero` defaults kept). Remaining
  evidence functions and the packages below are still over 15 lines.

## Status at pause

Done: A8–A11 met and enforced (executor/remote, admission, watch, SSE in api,
no SQL in composition roots, package docs, exact import allowlist). Q2 at 15
lines: done for foundation, ledgers, canonicaljson, contractsv1, eventlog,
admission, spec, situations, operators, engine, cognition, ingress.
Remaining (~330 functions): episodes, device, replay, runtime, evidence,
authority, actions, cmd, executor/remote, executor/native, runartifact,
policy, decisions, episodeledger, worker, watch, notify, control, soak, api.
Then set `funlen` to 15 lines/statements in `.golangci.yml`, run `make ci-check`,
and record final acceptance. Follow-up: move `episodes.FakeExecutor` under
`internal/executor/` (needs an in-package test change); remove the unused
`policy.CapabilityHost`.

## Continuation on 2026-10-03

Refreshed baseline at `b71f06f`: 331 production declarations exceed 15 body
lines (comments excluded); tests and generated files are excluded. Existing
A1–A11 remain the architecture acceptance bar. The root architecture suite
found one existing Q3 failure: `ingress/live_socket.go` was 303 lines.

- Round 13: separated live socket client acceptance/read loops from source
  configuration and lifecycle. No logic changed. The ingress race suite and
  root architecture suite pass with local Unix-socket access. The sandboxed
  ingress run could not bind sockets; this is an environment restriction.

- Round 14: completed the evidence package's 15-line production refactor.
  Capability decoding, HMAC verification, scope admission, query bounds and
  ledger reservation/completion are explicit steps. Fingerprint field order,
  errors, UTC formatting, live-attempt fences and transaction ownership remain
  unchanged. New regressions pin the exact fingerprint encoding, reject
  corrupted stored results, preserve identity-error precedence, and prove
  completion survives request cancellation. Evidence and root architecture
  race tests pass; focused lint passes without changing repository thresholds.

- Round 15: notification append reads as validate → canonicalize → deduplicate
  → publish; page admission delegates valid records and poison accounting.
  Poison retries still update/audit/clear in one transaction, and result sets
  close before accounting writes. Retention keeps its original operation and
  error order. A new regression proves pruning preserves cursor highwater,
  refuses expired events and resumes new events at the next cursor. Notification
  and architecture race tests pass; focused lint at 15 lines/statements passes.

- Round 16: watches now install, validate, match, spend allowances and expire
  through bounded steps. CEL compilation shares the same environment without
  introducing caches or changing evaluation. Event candidates close their read
  cursor before firing transactions; duplicate/event identities, partial counts,
  guard ordering and retry cancellation remain unchanged. Added payload-error
  precedence and retry cancellation regressions. Watch and architecture race
  tests and focused 15-line/15-statement lint pass.

- Round 17: SSE admission, stream following and record delivery are separate
  bounded steps. Problem-response precedence, priming before header commit,
  per-event authorization, duplicate filtering and cursor advancement remain
  unchanged. A new regression proves filtered duplicate records authorize once
  and still advance beyond skipped poison rows. API and architecture race tests
  and focused 15-line/15-statement lint pass.
