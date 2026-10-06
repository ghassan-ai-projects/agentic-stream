# One real run, start to end

This follows the predictive-maintenance example from a sensor event to a
verified effect, using the code as it is on this branch. Every number below
comes from an actual run of the files in this folder; nothing is invented.

## What the system is, in one paragraph

A **SituationSpec** (YAML, `docs/design/examples/predictive-maintenance.situation.yaml`)
is the program: it declares which events are accepted, how to compute **features**
from them (windows and operators), how a **Situation** moves between phases, when
reasoning is worth starting (**triggers**), and which **intents** an agent may
propose. The runtime turns a stream of events into versioned Situations
deterministically, and only starts a bounded **episode** (an agent run) when a
trigger says so. The agent returns a typed **Decision**; a separate deterministic
**policy** plane decides what may execute, and an **action** plane delivers it
once, idempotently, and verifies the result.

```text
sensor events (JSONL)
   │ ingress: parse, normalize, checkpoint
   ▼
event log ──► engine: watermark, dedupe (inbox), per-entity operators ──► features
                                                             │
                                  situations: reducers + guarded transitions
                                                             ▼
                               Situation versions (immutable) ──► notifications (SSE)
                                                             │
                              cognition: trigger score ≥ threshold? coalesce, debounce
                                                             ▼
                                              scheduler item ──► episode admission
                                                             │      (fence, cost reservation,
                                                             │       snapshot, tools, intent catalog)
                                                             ▼
                                   executor (native | remote worker | fixture) ──► Decision
                                                             │
                                          decisions: validate against the intent catalog
                                                             ▼
                                         intents ──► policy: verdict (automatic / approval / deny)
                                                             ▼
                          actions: command ──► outbox ──► effector (device, simulated)
                                                             ▼
                                              outcome ──► verification ──► notifications
```

## Reproduce it

```bash
python3 docs/walkthrough-end-to-end-2026-10-06/gen-trace.py /tmp/walk-trace.jsonl
AGENTIC_STREAM_WALKTHROUGH=/tmp/full.db AGENTIC_STREAM_WALKTHROUGH_TRACE=/tmp/walk-trace.jsonl \
  go test ./internal/runtime/internal/app -run TestWalkthrough -count=1 -v
sqlite3 -header -column /tmp/full.db "select count(*) from situation_versions"
```

The test (`internal/runtime/internal/app/walkthrough_test.go`) is skipped unless
`AGENTIC_STREAM_WALKTHROUGH` is set. It drives the same `Pipeline` the CLI uses,
with the in-process fixture executor and the simulated effector, so no model and
no hardware are needed.

`walkthrough.situation.yaml` is the shipped example with three changes, so one
batch reaches the end: `dispatchPolicy: active` (the default is shadow, which
scores a decision but never creates intents), no trigger `debounce`/`cooldown`
(real-time delays that a single batch would wait out), and a `reason` property on
`create_maintenance_ticket` (the fixture executor proposes it; the schema rejects
unknown parameters). `gen-trace.py` writes 60 minutes of motor-17 data: vibration
held at 6.2 mm/s, temperature climbing 0.2 degrees a minute, steady current, a
heartbeat every minute.

## The run, stage by stage

The pipeline report for the run: **240 events ingested and processed, 1 episode
admitted and executed, 1 intent evaluated, 1 command dispatched.**

| # | Stage | What happened | Evidence (table → rows) | Code |
| --- | --- | --- | --- | --- |
| 1 | Ingest | The JSONL file is read, each line validated against its registered event schema and appended. Duplicates and bad schemas never get in; they are quarantined | `event_log` 240 | `Pipeline.RunJSONL` (`runtime/internal/app/pipeline_run.go`), `ingress`, `eventlog` |
| 2 | Engine | Events are applied once, in log order (`event_inbox` 240 proves each was applied); watermarks and per-entity operator state advance | `event_inbox` 240, `operator_state` 3 (`vibration_rms`, `vibration_slope`, boot admission) | `engine.RunGlobal` (called from `advanceBatch`), `operators` |
| 3 | Features and situations | Windows emit features (`vibration_rms_15m`, slopes). The Situation opens when `vibration_rms_15m > 4.5`, then moves only through guarded transitions that must hold for a minimum duration | `situation_versions` 126: `candidate` v1–20, `watch` from v21, `warning` from v53 to v126 | `operators`, `situations` |
| 4 | Notifications | Each version and trigger evaluation is also published to a durable outbox that SSE clients read by cursor | `notifications` 166: 126 `situation.trigger.evaluated`, 37 `situation.superseded`, plus command and outcome events | `notify`, `api` SSE |
| 5 | Trigger | After every version the trigger `warning_needs_diagnosis` is scored (`severity*0.5 + novelty*25 + uncertainty*25`, threshold 45). While the phase is below `warning` the condition is false | `trigger_evaluations` 126: 88 `ignored` (`trigger condition false`), 38 `admitted` | `cognition` |
| 6 | Scheduling | Each admitted evaluation becomes a scheduler item, but a newer Situation version supersedes the pending one: 1 item ends `admitted`, 37 are `coalesced`. This is why 74 warning versions cause one episode, not 74 | `scheduler_items` 38 (1 admitted, 37 coalesced) | `cognition`, `episodeledger` |
| 7 | Episode admission | The runtime assembles an immutable request (snapshot of Situation version 126, tools, the intent catalog, a budget), claims a fence and reserves cost | `episodes` 1 (`epi_…01`, version 126, policy `active`) | `Pipeline.admission.AdmitPending`, `episodes`, `episodeledger`, `control` |
| 8 | Reasoning | The executor returns a typed Decision. It cannot execute anything, only propose | `episode_attempts` 1 (`produced`), `decisions` 1 | `executor/*` behind the `episodes.Executor` port |
| 9 | Decision validation | The Decision is checked against the compiled intent catalog and bound to the attempt (episode, attempt, fence, snapshot digest). A mismatch is a rejection, never an effect | `decisions.raw_json` carries one `create_maintenance_ticket` intent with an intent digest | `decisions` |
| 10 | Policy | The intent is persisted and evaluated by deterministic rules, independent of the agent: risk class R1, automatic policy, rate limit 2 per hour | `intents` 1 (`policy_status approved`), `policy_evaluations` 1 (`approved`, `automatic_r0_r1`) | `policy` |
| 11 | Action | An approved intent becomes a command with an idempotency key and is written to the outbox in the same transaction | `commands` 1 (`create_maintenance_ticket` for `motor-17`), `outbox` 1 (`command`, `delivered`, 1 attempt) | `actions` |
| 12 | Effect | The effector (simulated device) accepts the command; the outcome is recorded | `outcomes` 1 (`succeeded`, provider result `accepted: true`) | `actionport`, `device` |
| 13 | Verification | The result is observed independently of the effector's own answer and reconciled | `verifications` 1 (`observed`) | `actions`, `authority` |

### What each stage guarantees

- **Replay is deterministic.** Time comes from injected sources, the engine reads the
  log in order, and digests use canonical JSON. `agentic-stream run` on a trace
  prints a `versions_hash`; the same trace always prints the same hash.
- **Duplicates and out-of-order events are handled before state changes**
  (`event_inbox`, watermark, allowed lateness).
- **Reasoning is optional and bounded.** 126 evaluations produced one episode.
- **An agent never reaches an effect.** The Decision passes validation, then policy,
  then the action plane, each of which re-checks it. Fences stop a stale attempt
  from acting after a newer Situation version.
- **Effects are delivered once.** Commands carry idempotency keys; the outbox
  records delivery attempts; an unknown outcome is reconciled, not retried blindly.
- **Everything is explainable from tables.** Each arrow above is a foreign key you
  can follow from the command back to the events that caused it.

### Inspect it yourself

```sql
-- how the Situation moved
select version, phase from situation_versions where version in (1,20,21,52,53,126);
-- why reasoning started once
select outcome, count(*) from trigger_evaluations group by outcome;
select status, count(*) from scheduler_items group by status;
-- from effect back to cause
select c.command_id, c.status, o.status, p.result, p.reason
from commands c join outcomes o using (command_id)
join policy_evaluations p on p.command_id = c.command_id;
```

## The other entry points

| Command | Runs | Reaches |
| --- | --- | --- |
| `agentic-stream run --spec … --trace …` | engine only, in an isolated database | Situation versions and a `versions_hash`. On the shipped `trace-watch.jsonl`: 11 events, 2 versions (`candidate`, `watch`) |
| `agentic-stream run-live …` | one bounded batch through the whole pipeline | Whatever the executor and effect profile allow. Without a model endpoint the native executor uses its deterministic provider, which proposes nothing |
| `agentic-stream serve …` | the same pipeline continuously, with HTTP health, SSE and controls | Same; needs `AGENTIC_STREAM_SUBSCRIBER_TOKEN` and `AGENTIC_STREAM_CONTROL_TOKEN` |

## Defect found while running it

Running `serve --demo-mode` on the example spec with a trace that reaches
`warning` (and the default **shadow** dispatch policy) **crashes the process**:

```text
panic: runtime error: index out of range [0] with length 0
internal/episodes/internal/domain.highestRiskIntent        (shadow.go:55)
internal/episodes/internal/domain.ScoreShadowDecision      (shadow.go:42)
internal/episodes/internal/app.(*Runner).recordShadow      (runner_shadow.go:30)
```

Cause: without a model endpoint the native executor returns its deterministic
Decision, `decision_type: need_more_evidence`, which has no intents. In shadow mode
`ScoreShadowDecision` reads `Intents[0]`. The comment in `runner_shadow.go` says
`decisions.Validate` rejects a decision with zero intents; it does not. A legitimate
"need more evidence" Decision therefore takes the runtime down in shadow mode.

Not fixed here: the right score for a decision with no intents is a design choice
(for example a `would_take_no_action` score, or skip the shadow row). Suggested
test: a shadow episode whose Decision has no intents completes and records its
shadow row without panicking.
