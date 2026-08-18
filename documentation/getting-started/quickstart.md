# Quickstart

This path exercises the deterministic stream plane first. It does not require
an external model, worker, broker, or credentials.

The commands use the current example fixture kept under `docs/design/examples`;
that archive path is a machine-facing source artifact, while this page is the
public runnable guide.

## 1. Build the binary

```bash
make build
```

## 2. Validate the example SituationSpec

```bash
./bin/agentic-stream validate docs/design/examples/predictive-maintenance.situation.yaml
```

The command prints the spec name, version, canonical digest, and schema
version. To inspect the canonical JSON instead:

```bash
./bin/agentic-stream validate --json docs/design/examples/predictive-maintenance.situation.yaml
```

The compiler rejects undeclared payload fields, invalid units, duplicate YAML
keys, invalid CEL, unknown operators, and schema violations.

## 3. Replay a trace

Use a fresh database path for each deterministic replay:

```bash
./bin/agentic-stream run \
  --db predictive-maintenance.replay.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

The output includes the number of processed events, Situation versions, and a
hash of the version history. Replay opens a fresh database and never dispatches
external effects. Re-running the same trace with another fresh database should
produce the same version-history hash.

The `run` command accepts normalized JSONL. The simulator adapter is selected
by `--trace-format simulator` on `run-live` and `serve`.

## 4. Exercise the governed live batch

The safe local live path uses the deterministic native provider and simulated
effector:

```bash
./bin/agentic-stream run-live \
  --db predictive-maintenance.live.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

The summary reports ingestion, processing, episode admission/execution, intent
evaluation, and command dispatch. The simulated effector is idempotent and is
the proof surface used by the end-to-end tests; it is not a production device
adapter.

## 5. Inspect the source evidence

The example and its supporting traces live in:

- [`docs/design/examples/predictive-maintenance.situation.yaml`](../../docs/design/examples/predictive-maintenance.situation.yaml)
- [`examples/predictive-maintenance/testdata/`](../../examples/predictive-maintenance/testdata/)
- [`internal/eventschema/registry_data.json`](../../internal/eventschema/registry_data.json)
- [`internal/ingress/simulator_data.json`](../../internal/ingress/simulator_data.json)
- [`internal/episodes/testdata/aquaculture_intents.json`](../../internal/episodes/testdata/aquaculture_intents.json)

The data files are loaded by the runtime and protected by digest/parity tests.
Domain data must not be re-authored as Go literals.

## What this quickstart proves

It proves the local semantic path, not production readiness. For the full
acceptance story, follow [the predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
and read [the limitations](../overview/limitations.md).

## Next reads

- [Live runtime](live-runtime.md)
- [Predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
- [Compatibility](../overview/compatibility.md)
- [CLI reference](../reference/cli.md)
