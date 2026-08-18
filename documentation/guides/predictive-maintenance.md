# Predictive-maintenance walkthrough

This is the first domain fixture and stream-plane walkthrough. It shows how a
motor trace becomes durable Situation state. The repository's broader focused
and synthetic end-to-end tests cover bounded episodes, typed Intents, policy,
and the simulated effector; the one-event opening trace below does not reach
those later stages.

The SituationSpec fixture is intentionally stored in the current design
archive because tests load it directly. This page is the public explanation of
how to use that fixture and what its evidence does—and does not—prove.

## The assets

- Spec: [`docs/design/examples/predictive-maintenance.situation.yaml`](../../docs/design/examples/predictive-maintenance.situation.yaml)
- Opening trace: [`trace-opening.jsonl`](../../examples/predictive-maintenance/testdata/trace-opening.jsonl)
- Heartbeat trace: [`trace-heartbeat.jsonl`](../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl)
- Watch trace: [`trace-watch.jsonl`](../../examples/predictive-maintenance/testdata/trace-watch.jsonl)
- Synthetic runtime proof for the later action path: [`internal/runtime/pipeline_e2e_test.go`](../../internal/runtime/pipeline_e2e_test.go)

## Run it

```bash
make build
./bin/agentic-stream validate docs/design/examples/predictive-maintenance.situation.yaml
./bin/agentic-stream run-live \
  --db predictive-maintenance.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

The output report counts events, processed work, episodes, evaluated Intents,
and dispatched Commands. With the opening fixture, the expected result is one
ingested/processed event and zero episodes, Intents, or Commands because the
Situation remains in its initial candidate phase. The live batch uses the
deterministic native provider unless a worker or model endpoint is explicitly
configured.

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
