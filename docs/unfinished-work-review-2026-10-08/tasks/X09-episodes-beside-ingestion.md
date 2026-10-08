# X09 — Episodes run beside ingestion

Status: todo · Decision: **complete, as TECHNICAL_DESIGN §11.4–11.5 specify** · Priority: P0 (experiment core) · Size: M · Depends on: X03 (land together)

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md) G2, G9.

## Finding

`advanceBatch` → `executeAdmittedEpisodes` → `episodes.Service.RunOnce` calls
the worker synchronously. Every live event (and, since X08, every clock tick)
runs that batch under the batch lock, so while a worker reasons nothing else
advances: no ingestion, no engine, no timers, no supersession, no dispatch.

## Decision and reasoning

Run admitted episodes on their own loop in the runtime module, bounded by the
existing global worker permits, outside the batch lock:

- the episode loop claims due attempts and executes them (the episode ledger
  already fences attempts, so concurrent claiming is safe by design);
- the batch keeps everything deterministic (engine, admission, policy,
  dispatch) and stays serial; it picks up concluded episodes' intents on its
  next run, which the X08 clock guarantees;
- cognition's existing supersession can now cancel a running attempt's context
  when a material version arrives (§11.5, an MVP acceptance item).

Must land with X03: once ingestion continues during reasoning, completeness
flips publish new versions during every episode, and strict version freshness
would stale almost every R1 intent. `TestExperimentClosedLoopUnderAContinuousFeed`
is the guard: it passes today because ingestion stalls, would fail with X09
alone, and must pass with X09 + X03.

## Done when

- A test shows readings ingested and a phase transition published while a slow
  worker reasons, and a material phase change cancels the running attempt.
- The continuous-feed X01 test passes with a 5 s worker and 2 s slides.
- Replay is unchanged (it uses its own loop and the fixture executor).
