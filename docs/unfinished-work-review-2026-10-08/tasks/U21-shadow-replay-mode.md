# U21 — Shadow replay mode

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U20 (shared mode plumbing)

## Finding

Paired shadow replay runs a deterministic baseline and a candidate executor on
the same immutable snapshot for each replayed episode, validates both outputs
against the spec's intent catalog, and seals a comparison into
`shadow_comparisons` (51 symbols: `NewDeterministicBaseline`, `BaselinePolicy`,
`ShadowRules`, `BuildComparison`, `Store.RecordShadowComparison`,
`applyPairedShadow`, …). The baseline exists; **no production
`ShadowExecutor`** does, and the CLI cannot select the mode.
`shadow_comparisons` has a writer but no reader.

Live shadow *dispatch* (`executor.dispatchPolicy: shadow`, `shadow_decisions`)
is separate and already reachable. This task is about evaluating a new model or
prompt against a recorded trace.

The MVP requires "run a new model or prompt against the same trace in
effect-disabled shadow mode". HIL Phase 02 (G3: "Tamoz shadow decisions compared
with a baseline") states that the paired replay shadow path is implemented; it
is, as a library, but an operator cannot run it.

## Decision and reasoning

Complete it, with the worker protocol as the only candidate adapter:

1. **`ShadowExecutor` over the remote executor.** Build the worker request from
   `ShadowInput` (snapshot, digests, synthetic attempt and fence), stream it to
   a worker over `--worker-socket`, and return the Decision. Use the existing
   handshake and stream validation in `executor/remote`.
   - No evidence-tools feature in replay: the replay database is isolated and
     the snapshot is the whole input. The handshake must not offer
     `EvidenceToolsFeature`. A worker that requires it fails the trial with a
     clear reason.
   - Use the episode budget from the spec; a budget breach fails the trial and
     is recorded, never retried.
2. **The CLI:**

   ```text
   agentic-stream run --spec <spec> --trace <trace> --mode shadow --worker-socket <path> [--worker-name tamoz]
   ```

   Prints per-episode agreement and differences and the comparison IDs, and
   exits 0 even when they differ: disagreement is a result, not an error.
3. **Read path:** `--json` output includes the sealed comparison documents, so
   the scorer (the Agent Research Lab per the HIL plan) does not read SQLite.

Why only the worker protocol: per ADR-016 (proposed) the candidate in
production is Tamoz, which runs as a worker. Wiring the in-process native
executor as a shadow candidate would add a second path for a model that is
documented as "not a production reasoner".

## Done when

- End-to-end test with the `workerfake` server (U06) as candidate: comparisons
  are sealed, no command, outbox or intent row exists in the replay database,
  and `EffectsAllowed=false`.
- The 51 shadow and baseline symbols are reachable from `main`; the U12
  allow-list is empty.
- HIL `02-shadow-path.md` and `PHASE-02-EXECUTION.md` cite the command as the
  G3 evidence path.
