# Core concepts

For readers who need a quick vocabulary reference. For explanations in context,
follow the [learning path](../learn/README.md). Each term below has one job;
keeping those jobs separate is how the runtime controls state and authority.

## Observations and state

| Term | Plain meaning | Motor example |
| --- | --- | --- |
| Evidence / event | An observed fact with identity, time, source, and typed payload | One vibration reading |
| SituationSpec | The declared rules for interpreting a domain's evidence | The bearing-degradation spec |
| Window | The bounded set of readings used in a calculation | A fifteen-minute vibration window |
| Operator / feature | A repeatable calculation and its result | Vibration RMS, a measure of signal magnitude |
| Reducer | A rule for incorporating a feature into current state | Keep the latest event-time value |
| Situation | The durable record of an evolving condition | Bearing degradation for one motor occurrence |
| Situation version | One immutable publication of that condition | A candidate or warning snapshot with supporting evidence |
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
| Completeness | The recorded state of expected evidence and source health |
| Late correction | A new published result that incorporates older evidence without overwriting the earlier version |
| Canonical digest | A content identity calculated from consistently serialized data |

A watermark is not proof that every earlier event exists. Missing heartbeat
information and late readings remain explicit evidence concerns.
See [time and changing state](../learn/time-and-state.md).

## Attention and reasoning

| Term | Plain meaning |
| --- | --- |
| Cognitive opportunity | Queued work saying a version may deserve reasoning |
| Material delta | A meaningful change, such as a new phase or severity |
| Admission | Turning eligible queued work into an episode |
| Episode | A finite reasoning session bound to one immutable snapshot |
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

## Replay and evaluation

**Deterministic replay** rebuilds stream history with effects disabled.
**Recorded replay** reuses a worker ledger. **Shadow** evaluates an executor
without entering governance. **Counterfactual** evaluates through an explicit
simulator. The CLI's `run` command exposes deterministic replay; the other
modes are library/runtime capabilities, not separate CLI subcommands.

Sources: [stream design](../design/stream-processing.md),
[cognition design](../design/cognition.md),
[Decision/Intent contract](../contracts/decision-intent.md), and
[replay modes](../design/replay-and-shadow.md).

## Next reads

- [Learn the concepts in context](../learn/README.md)
- [Architecture overview](../architecture/overview.md)
- [Why these design choices](../learn/design-choices.md)
