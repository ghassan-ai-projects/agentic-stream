# Core concepts

Start with [what a Situation represents](../learn/domain-model.md) for the
domain model, then use this page as a vocabulary and ownership reference.
The [learning path](../learn/README.md) explains the concepts in context.
The definitions separate observations, state, proposals, and permission so
each part of the runtime has a clear responsibility.

## Domain identity

| Term | Plain meaning | Implementation owner |
| --- | --- | --- |
| Tenant | The organization whose evidence and permission are being handled | Identity is carried across contracts and durable records |
| Entity | The thing evidence describes, identified by type and ID | Event envelope and Situation state |
| Situation type | The kind of condition followed for an entity | SituationSpec |
| Deployment | An activated compiled definition and its state namespace | `internal/spec` |
| Occurrence | The recorded instance of a condition lifecycle | `internal/situations`; a resolved Situation reopens as a fresh occurrence after `reopenCooldown` |
| Phase | The domain state, such as `candidate` or `warning` | Spec transitions evaluated by the stream |
| Severity | Domain-defined importance of a phase | Phase declaration |
| Confidence | Certainty field for the interpretation | Starts at `1.0`; no calibrated update mechanism in the current Situation engine |
| Hypothesis | A proposed explanation of the evidence | Reasoning output; no changing primary-hypothesis model in current Situation state |

A phase describes the condition, not an episode or Command lifecycle. See
[domain identities and current boundaries](../learn/domain-model.md).

## Observations and state

| Term | Plain meaning | Motor example |
| --- | --- | --- |
| Evidence / event | An observed fact with identity, time, source, and typed payload | One vibration reading |
| SituationSpec | The declared rules for interpreting a domain's evidence | The bearing-degradation spec |
| Window | The bounded set of readings used in a calculation | A fifteen-minute vibration window |
| Operator / feature | A repeatable calculation and its result | Vibration root mean square (RMS), a measure of signal magnitude |
| Reducer | A rule for incorporating a feature into current state | Keep the latest event-time value |
| Hysteresis | Different entry and exit conditions that reduce state oscillation | A lower recovery threshold than the opening threshold |
| Minimum duration | Time a transition condition must hold before changing phase | Sustained vibration evidence before escalation |
| Situation | The durable record of an evolving condition | Bearing degradation for one motor occurrence |
| Situation version / snapshot | One immutable published view of the condition | A candidate or warning snapshot with supporting evidence |
| Fact | A derived value retained in current state | Latest event-time vibration magnitude |
| Provenance | The links explaining where an interpretation came from | Supporting events, spec digest, and time boundary |
| Virtual partition | A unit whose state changes are processed in a deterministic serial order | State grouped by a configured event partition key |

Raw evidence is data, never an executable instruction. The stream owns the
Situation state; an agent reads a version rather than editing that state.
See [the worked flow](../learn/how-it-works.md).

## Time and uncertainty

| Term | Plain meaning |
| --- | --- |
| Event time | When the observation happened according to its source |
| Ingestion time | When the runtime received it |
| Watermark | A forward-moving boundary used to judge window progress and lateness |
| Completeness | A feature's evidence-processing status copied into Situation state; separate from confidence |
| Late correction | A new published result that incorporates older evidence without overwriting the earlier version |
| Canonical digest | A content identity calculated from consistently serialized data |
| Reconsideration | Bounded work reviewing an eligible prior succeeded Command after a correction |
| Compensation | A new governed proposal linked to an earlier Command; not an automatic reversal |

A watermark is not proof that every earlier event exists. Missing heartbeat
information and late readings remain explicit evidence concerns.
See [time and changing state](../learn/time-and-state.md).

## Attention and reasoning

| Term | Plain meaning |
| --- | --- |
| Cognitive opportunity | Queued work saying a version may deserve reasoning |
| Material delta | A meaningful change, such as a new phase or severity |
| Admission | Turning eligible queued work into an episode |
| Episode | A finite reasoning session bound to one immutable snapshot at a time |
| Attempt | One execution of the episode with a fixed request and its own identity |
| Rebinding | Repoint an episode to a validated live snapshot before an attempt starts |
| Trigger | A deterministic condition, score, and optional material-change test |
| Coalescing | Replace older open work for the same Situation and trigger |
| Executor / worker | The implementation that performs the reasoning session |
| Budget | Limits on time, model/tool use, result size, retries, and cost |
| Fence | An increasing attempt generation used to reject stale output |
| EvidenceTools | Capability-scoped read tools available to the episode |

A trigger passing does not guarantee an immediate model call. Timing, capacity,
expiry, and supersession still matter. See
[when an agent should reason](../learn/reasoning.md).

## Proposals and execution

| Term | Plain meaning | Authority |
| --- | --- | --- |
| Decision | Structured reasoning output tied to an episode and snapshot | Model proposes; runtime validates |
| Intent | One typed proposed effect within an allowed catalog | Policy evaluates |
| Policy result | Recorded permission, denial, deferral, approval need, or reuse | Deterministic policy |
| Command | A governed request created from an accepted Intent | Action plane owns dispatch |
| Outbox | Durable queued Commands waiting for dispatch | Policy publishes; actions lease and deliver |
| Effector | The adapter that performs a permitted change | Governed action boundary |
| Outcome | Recorded effect result, including uncertainty | Dispatch and reconciliation |
| Idempotency key | Identity for the same logical request across delivery attempts | Provider support determines the external guarantee |
| Reconciliation | Obtain evidence of what actually happened after an uncertain result | Controlled operational path |

An agent can propose an Intent. It cannot create a governed Command or call an
effector. See [from proposal to effect](../learn/safe-actions.md).

## Control and recovery

| Term | Plain meaning | Owner |
| --- | --- | --- |
| Target | The destination of an effect; it may differ from the observed entity | Intent/Command binding |
| Capability | An allowed operation with declared constraints | Action/device catalogs |
| Target authority | Durable control ownership and barriers for a device target | `internal/authority` |
| Device session | The communication lifecycle with a concrete device | `internal/device` |
| Interlock | A condition that must hold for readiness | Current-state checks at the action boundary |
| Lease | A temporary claim to perform work | The relevant lifecycle ledger |
| Policy epoch | The current generation of operational permission | `internal/control` |
| Runtime owner | The current runtime holding ownership for controlled work | `internal/control` |
| Durable ledger | The persisted lifecycle history used for acceptance and recovery | The ledger for each record type |
| Watch | An approved, scoped, expiring condition with bounded firings | `internal/watch` |
| Qualification | Evidence that a configuration/integration is suitable for its intended environment | Calibration, shadow reports, and release gates |

See [who owns state and authority](../learn/runtime-boundaries.md) for why
these boundaries remain separate during normal execution and recovery.

## Replay and evaluation

**Deterministic replay** rebuilds stream history with effects disabled.
**Recorded replay** reuses a worker ledger. **Shadow** evaluates an executor
without entering governance. The CLI's `run` command exposes deterministic replay; the other
modes are library/runtime capabilities, not separate CLI subcommands.

Sources: [stream design](../design/stream-processing.md),
[cognition design](../design/cognition.md),
[Decision/Intent contract](../contracts/decision-intent.md), and
[replay modes](../design/replay-and-shadow.md).

## Next reads

- [Understand the domain model](../learn/domain-model.md)
- [Learn the concepts in context](../learn/README.md)
- [Architecture overview](../architecture/overview.md)
- [Why these design choices](../learn/design-choices.md)
