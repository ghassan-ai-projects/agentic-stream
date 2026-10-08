# U19 — `run --repeat N`

Status: todo · Decision: **complete** · Priority: P2 · Size: XS

## Finding

`replay.RunNTimes` replays the same trace N times against fresh isolated
databases, and `replay.AllHashesEqual` compares the results (6 symbols with
`repeatReplay` and `WithRunDirectory`). Only tests call them. Gate A requires
"three fresh runs over each golden trace produce byte-identical canonical
projections", and the MVP's first item is "identical Situation history for
repeated deterministic replay". Today an operator can only check this by running
`run` several times and comparing output by hand.

## Decision and reasoning

Complete it. This is the cheapest task in the review and it turns a release gate
into a command anyone can run:

```text
agentic-stream run --spec <spec> --trace <trace> --repeat 3
```

Prints each run's `versions_hash` and exits non-zero if any differ. `--db` is
refused with `--repeat > 1` (each run needs a fresh database); the runs use the
temporary directory `RunNTimes` already manages.

## Done when

- CLI test: `--repeat 3` on the predictive-maintenance trace prints three equal
  hashes and exits 0; a test double that returns differing hashes exits 1.
- The quickstart and CLI reference show the flag; `make ci-check` (or the golden
  replay target) uses it for the committed traces.
