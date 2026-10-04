# Quickstart

For evaluators: build the binary, validate a spec, and replay one motor reading.
Run these commands from the repository root with the prerequisites in
[install and build](install.md). No model account, worker, broker, or credentials
are needed for this local path.

The spec and trace are committed fixtures. The spec stays under
`docs/design/examples` because tests also load it; this page owns the public
instructions.

## 1. Build

```bash
make build
```

Expected result: `bin/agentic-stream` is available. If the build fails, resolve
its error before continuing.

## 2. Validate the motor spec

```bash
./bin/agentic-stream validate docs/design/examples/predictive-maintenance.situation.yaml
```

Expected result: `ok: motor_bearing_degradation`, version `0.1.0`, a
`sha256:` digest, and schema `agentic-stream/v1`. The digest identifies the
normalized spec; it is not a runtime outcome.

To inspect the normalized JSON, use:

```bash
./bin/agentic-stream validate --json docs/design/examples/predictive-maintenance.situation.yaml
```

Validation checks the schema and meaning of the spec, including input fields,
units, duplicate keys, rule expressions, and declared operators. It does not
prove every action policy field is enforced; read
[the current limitations](../overview/limitations.md) before adapting actions.

## 3. Replay twice and compare

Replay requires a fresh database. Create a temporary directory so these commands
can be copied and run again without colliding with a previous run:

```bash
demo_dir=$(mktemp -d)
./bin/agentic-stream run   --db "$demo_dir/replay-a.db"   --spec docs/design/examples/predictive-maintenance.situation.yaml   --trace examples/predictive-maintenance/testdata/trace-opening.jsonl   | tee "$demo_dir/replay-a.txt"

./bin/agentic-stream run   --db "$demo_dir/replay-b.db"   --spec docs/design/examples/predictive-maintenance.situation.yaml   --trace examples/predictive-maintenance/testdata/trace-opening.jsonl   | tee "$demo_dir/replay-b.txt"

cmp "$demo_dir/replay-a.txt" "$demo_dir/replay-b.txt"
```

Each replay should finish successfully and print:

```text
events_processed=1 situation_versions=1 versions_hash=<same hash in both runs>
```

`cmp` produces no output and succeeds when the reports match. This checks the
stream history for the same trace and spec; replay dispatches no external
effects. If either replay prints an error, stop and inspect it before comparing.

The `run` command reads normalized JSONL. The simulator adapter is selected
with `--trace-format simulator` on `run-live` or `serve`, not `run`.

## 4. Run the same trace as a local live batch

Use the same terminal, where `demo_dir` is still set:

```bash
./bin/agentic-stream run-live   --db "$demo_dir/live.db"   --spec docs/design/examples/predictive-maintenance.situation.yaml   --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

Expected report:

```text
events_ingested=1 events_processed=1 episodes_admitted=0 episodes_executed=0 intents_evaluated=0 commands_dispatched=0
```

The one reading opens a `candidate` Situation. The diagnosis trigger needs a
later `warning` or `incident` phase, so zero episodes and Commands is correct.
The batch uses the deterministic native provider and simulated effect profile
by default; it is not a physical-device or external-ticket integration.

If you run another batch, use a new database path or begin again with a new
`demo_dir`. Preserve the directory while investigating an unexpected result.

## What you have learned

The spec compiles, the reading produces versioned state, and the stream history
repeats. You have not proved the later reasoning or effect stages with this
one-event trace. The [motor walkthrough](../guides/predictive-maintenance.md)
points to the separate tests for those stages.

If a step fails, start with [troubleshooting](../guides/troubleshoot.md).
Deployment qualification remains separate from this local proof.

## Next reads

- [Understand what happened to the reading](../learn/how-it-works.md)
- [Predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
- [Author your first SituationSpec](first-situation.md)
- [Live runtime](live-runtime.md)
