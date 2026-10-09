# Predictive-maintenance walkthrough

This is the first domain fixture and stream-plane walkthrough. It shows how a
motor trace becomes durable Situation state. The repository's broader focused
and synthetic end-to-end tests cover bounded episodes, typed Intents, policy,
and the simulated effector; the one-event opening trace below does not reach
those later stages.

The SituationSpec fixture is intentionally stored in the current design
archive because tests load it directly. This page is the public explanation of
how to use that fixture and what its evidence does—and does not—prove.

## Understand the story first

[Follow one reading](../learn/how-it-works.md) for the plain-language flow.
The motor spec looks for a persistent bearing condition. Its phases and
duration rules decide how the condition develops; the diagnosis trigger
only applies in `warning` or `incident`.

## The assets

- Spec: [`docs/design/examples/predictive-maintenance.situation.yaml`](../../docs/design/examples/predictive-maintenance.situation.yaml)
- Opening trace: [`trace-opening.jsonl`](../../examples/predictive-maintenance/testdata/trace-opening.jsonl)
- Heartbeat trace: [`trace-heartbeat.jsonl`](../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl)
- Watch trace: [`trace-watch.jsonl`](../../examples/predictive-maintenance/testdata/trace-watch.jsonl)
- Synthetic runtime proof for the later action path: [`internal/runtime/internal/app/pipeline_end_to_end_test.go`](../../internal/runtime/internal/app/pipeline_end_to_end_test.go)

## Run it

```bash
make build
walkthrough_dir=$(mktemp -d)
./bin/agentic-stream validate docs/design/examples/predictive-maintenance.situation.yaml
./bin/agentic-stream run-live \
  --db "$walkthrough_dir/predictive-maintenance.db" \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

The output report counts events, processed work, episodes, evaluated Intents,
and dispatched Commands. With the opening fixture, the expected result is one
ingested/processed event and zero episodes, Intents, or Commands because the
Situation remains in its initial candidate phase. The live batch uses the
deterministic native provider unless a worker or model endpoint is explicitly
configured.

## Read the spec as a story

| Part of the fixture | Meaning |
| --- | --- |
| Inputs | Vibration, temperature, current, and heartbeat evidence for a motor |
| Windows and operators | Calculate vibration magnitude, slopes, current mean, and heartbeat absence |
| Opening and phases | Open a candidate occurrence, then require sustained evidence for later phases |
| Diagnosis trigger | Consider a warning/incident version, with heartbeat, score, and material-change checks |
| Episode objective | Diagnose the condition and propose safe inspection or maintenance steps |
| Intent catalog | Bound proposals to declared maintenance/recommendation types and risk classes |

These are example rules, not a diagnosis or recommended operating thresholds
for a real motor. The exact conditions and durations are in the
[fixture](../../docs/design/examples/predictive-maintenance.situation.yaml).

## What the trace exercises

The example declares motor sensor inputs and uses time windows/operators to
derive warning evidence. Cognition is gated by triggers, thresholds, and
debounce/cooldown behavior. The fixture also includes heartbeat and watch
traces for stream behavior. The bounded episode, maintenance-ticket Intent,
policy, and simulated-effector behavior are demonstrated by the focused tests
listed below, not by the opening trace alone.

## Verify the acceptance behavior

Run the focused proof suite:

```bash
go test ./internal/runtime ./internal/replay ./internal/cognition ./internal/episodes ./internal/policy ./internal/actions
```

The relevant tests cover deterministic completion, late correction and
reconsideration, duplicate/out-of-order and heartbeat behavior, scheduler
suppression, cancellation, Decision/Intent validation, policy denial,
idempotent effects, unknown outcomes, replay isolation, shadow mode, and worker
capability boundaries.

## Explain what happened

Every Situation field and trigger decision is explainable from the runtime
database. After a `run-live` batch:

```bash
agentic-stream situation list --db runtime.db
```

```bash
agentic-stream explain situation <situation-id> --db runtime.db
```

```bash
agentic-stream explain trigger <trigger-id> --db runtime.db
```

`explain situation` traces each field to its reducer and operator in the
deployed spec and lists the logged evidence events; it also lists the version's
trigger evaluations, whose ids `explain trigger` takes. See the
[CLI reference](../reference/cli.md).

## What this does not prove

This example does not certify a real maintenance system, a production model,
an external ticket provider, backup/restore, network exposure, or a stable
release. Those require deployment-specific evidence. Read the [limitations]
before adapting the example.

[limitations]: ../overview/limitations.md

## Next reads

- [Stream processing](../design/stream-processing.md)
- [Decisions and actions](../design/decisions-and-actions.md)
- [Operations](../operations/README.md)
