# X02 — Repair the experiment's references to moved or removed surfaces

Status: todo · Decision: **fix references, no runtime change** · Priority: P0 · Size: XS

## Finding

Two breaks exist at `be37f6b`
([EXPERIMENT_COMPATIBILITY.md](../EXPERIMENT_COMPATIBILITY.md) B1, B2):

- **B1:** saved spec copies with `retention:`/`telemetry:` blocks fail
  `validate` since `1ebae6e` removed the unenforced blocks.
- **B2:** `RUNBOOK-G1.md` passes a catalog path that `98b84d6` moved.

`CURRENT-STATUS.md` also cites three files under `internal/actions/` that now
live under `internal/device/`.

## Decision and reasoning

Fix the references, not the runtime.

- B1: do not re-accept `retention`/`telemetry`. Accepting and ignoring them is
  the misleading surface the audit removed. The runbook already copies the
  canonical spec, which is valid. Saved copies drop the two blocks.
- B2: do not move the file back; that would be a third location. Point the
  runbook at the current path, and let X01's pin test keep it there. If the
  catalog ever needs a more stable home, move it once, together with the pin
  and the runbook.

These edits are in the research repository
(`agent-research-lab/real-world-sensor`), so they need the owner's go-ahead.
This repository only gains the X01 pins.

## Steps (in `agent-research-lab/real-world-sensor`)

1. `assessment/RUNBOOK-G1.md`: `--device-catalog
   "$STREAM/internal/contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json"`;
   add "copy the spec fresh from `$STREAM/docs/design/examples/` for every run;
   older copies with `retention`/`telemetry` blocks no longer validate".
2. `assessment/CURRENT-STATUS.md`: update the three `internal/actions/...` paths
   to their `internal/device/...` locations.
3. Re-run `agentic-stream validate` on the copied spec and record the commit in
   the run's `SHAS.txt`.

## Done when

The runbook commands run as written against the current `main`, and X01 pins
the catalog path they use.
