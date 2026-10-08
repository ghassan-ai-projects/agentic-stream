# X07 — Joined rehearsal: simulator, then bench

Status: todo · Decision: **run the proof** · Priority: P0 (experiment) · Size: M · Depends on: X03, X04, X05, X06, U15

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md).

## Steps

1. **Simulator pass** (RUNBOOK-G1 topology, X05 sim spec, **continuous**
   feed with no pauses): one Situation, Tamoz proposes
   `select_thermal_mode {mode: bounded_cooling}`, approval requested, human
   signs through the Tamoz relay after at least 30 s of non-material readings,
   command dispatched to the emulator, `query_state` verified, export and
   `verify-run` pass. Also the LED (R1) path under the same continuous feed.
2. **Negative pass**: a phase change before approval yields `superseded`, no
   command; an expired approval yields no command; a tripped interlock (U14,
   when available) yields no command.
3. **Bench pass** (RUNBOOK-HIL, X05 bench spec, X06 mapping): the same flow
   through the gateway; the operator records rotation; safe-stop returns the
   board to `safe_state: true`. The gateway allow-lists the digest from
   `validate --json`.
4. Record the three repositories' commits, the spec digests and the artifact
   paths in the run report.

## Done when

The run artifacts for steps 1 and 3 verify `pass`, and CURRENT-STATUS can mark
B5 (live closed loop) closed at the operator-observed evidence level.
