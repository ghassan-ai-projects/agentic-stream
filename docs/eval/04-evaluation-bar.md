# 04 — Evaluation bar

When is this evaluation itself trustworthy? The same discipline the runtime applies to its own
effects applies here: a gate that can be satisfied by an artifact that does not prove the claim
is worse than no gate.

## The bar

1. **One command, one immutable artifact.** A documented entry point runs a suite and writes one
   artifact; a second command re-verifies it. No verdict exists without an artifact.
2. **Every axis is accounted for.** `coverage.json` names every axis as measured or as an
   explicit gap. An axis with no cell is stated, never omitted. Silence is a defect.
3. **Class D is exact and fail-closed.** One counterexample fails the run. Diagnostics explain a
   failure; they can never upgrade a failed counter to a pass. This is `soak`'s existing rule,
   extended to every deterministic cell.
4. **Class C is statistical.** `k >= 3` trials, a paired comparison against
   `replay.DeterministicBaseline`, a held-out scenario family recorded by digest, an interval
   that clears zero, and cost per cell. A single run is never a capability claim.
5. **Graders read durable records, bytes, contracts, and counters.** No grader reads the model's
   narration. If a grader needs the model to describe what it did, the cell is mis-designed.
6. **Every verdict carries its claim tier.** `plumbing`, `device_contract`, or `physical`. A
   device-reported `receipt` or `result` can never grade as `physical`; the ceiling is enforced
   by the grader.
7. **Blocked is not zero.** Missing provider, device, or artifact yields `blocked(reason)` and
   stays out of every rate. An unavailable measurement is never scored as failure or success.
8. **The artifact is independently verifiable.** `verify-run` covers the `eval/` files as well
   as the JSONL projections: a tampered cell, a stale digest, or a missing coverage entry fails
   verification.
9. **No fixture run yields a capability verdict, and no CI cell needs a provider.** Class C is
   off-CI by construction; a scripted or emulated run may enter the regression class only.
10. **Cost is reported per cell.** Model calls, tokens, tool-result bytes, and wall time. No
    number is quoted without them.

## Acceptance

The evaluation is **trustworthy** when bars 1–10 hold for a run: one artifact, every axis
accounted for, exact deterministic grading, statistical capability grading, no self-report, tier
ceilings enforced, blocked distinguished from zero, and independent verification.

The runtime is **measured** when every axis in
[01-measurement-model.md](01-measurement-model.md#2-axes) returns `pass`, `fail`, or
`blocked(reason)` with evidence — and the report names the axes it cannot yet reach. Until
then, the honest statements are bounded and specific: *"the deterministic class is green for
these axes, the dispatch fault matrix is unbuilt, and no capability claim has been made."*

The runtime is **releasable** when the deterministic class passes, no zero-tolerance counter is
non-zero, the operational cells (S8) pass, and any capability claim carries its tier — with the
release status citing the artifact digest.

## Explicitly out of scope

- **Production qualification.** The build-completion bar postpones environment-level rehearsals; this evaluation does not reverse that.
- **Benchmarking against other runtimes.** Comparing Agentic Stream to another agent framework is a separate study with its own matched-comparison protocol; a single-runtime evaluation cannot license a comparative claim.
- **Model leaderboards.** Which provider is "better" is not this evaluation's question; the question is whether the runtime's guarantees hold and whether its bounded cognition is useful.
- **A single scalar score.** The product's claim is a conjunction of invariants; collapsing it to one number would hide exactly the failure the invariants exist to prevent.
