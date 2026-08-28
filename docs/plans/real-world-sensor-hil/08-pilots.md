# Phase 08 — Supervised pilots (M5 → roadmap Phase 8)

> **Longer-horizon skeleton.** Start only after Phase 07 (M4.5) is green **and**
> Round 2 **Checkpoint A** is settled favourably ("Does Tamoz add measurable
> value over deterministic control?"). If Tamoz does not beat the baseline on
> exceptions, keep the platform advisory and narrow the use case — do not run
> agentic pilots (roadmap §Checkpoint A / §Phase 3 product decision).

**Goal:** two supervised, non-critical, reversible pilots on **different device
ecosystems**, proving portability and real-world value — not repeating the
reference rig.

**Allowed claim after Gate P8:** *pilot-ready for bounded, supervised,
non-critical physical workflows within the published compatibility matrix.*
Never beyond the exact tested hardware and adapter versions.

This phase is mostly **operational and evidentiary**; Agentic Stream's code role
is small (hardening + evidence export), which is the point — a stable core is the
precondition for piloting.

---

## Pilot constraints (hard gates, encoded where possible)

Each constraint below should be *enforced or verified by the platform*, not left
to operator discipline:

- non-critical, low-energy, **reversible** action only (capability catalog +
  risk-class ceiling enforce this);
- the local **deterministic controller remains authoritative** — Agentic Stream
  stays supervisory (L2), never in a fast loop;
- a named human owner + **rapid physical disablement** (Phase 07 safe-disable +
  the physical e-stop from Phase 04);
- **no public-network exposure** without an authenticated gateway/proxy (the
  serve command already refuses non-loopback listen without an authenticated
  proxy — keep and test that);
- explicit data, retention, incident, and rollback agreements;
- no claim beyond the exact tested hardware and adapter versions.

---

## Recommended pilots (roadmap)

1. environmental/thermal lab or equipment-cabinet **advisory cooling** (reuses
   the thermal reference domain, now over a real ecosystem adapter);
2. non-critical **greenhouse or water-model rig** with a bounded actuator proxy.

Use **different device ecosystems** across the two pilots so they test
portability (Phase 06 adapters A and B), not a repeat of the Arduino path.

---

## Task 8.1 — Pilot evidence pipeline

Extend the run-manifest exporter (Phase 04 Task 4.4) into a **pilot evidence
pack** that a third party can audit without privileged internal knowledge:

- at least 30 days combined supervised runtime, chunked into verifiable run
  artifacts;
- availability / latency / intervention SLOs computed from the ledgers;
- decision quality vs. baseline and vs. human review (false-action, missed-action,
  abstention, manual-override rates — the §Program metrics set);
- an incident + near-miss register bound to the durable evidence;
- upgrade + rollback performed *during* the pilot, with authority preserved or
  safely invalidated;
- pilot-owner sign-off capturing limitations and unresolved risks.

**Exit:** an external reviewer can reconstruct any decision→verified-effect chain
from the pack alone.

---

## Task 8.2 — Longitudinal hardening

Real pilots surface issues short soaks don't. Reserve capacity in this phase for:

- reconciliation-age and unknown-outcome backlog behavior over weeks;
- resource high-water marks / backlog age under real duty cycles;
- operator-truth gaps found in the field feeding back into Phase 07 surfaces.

No new capability — this is stability + observability under real load.

---

## Gate P8 exit checklist

- [ ] Both pilots complete without a safety-invariant violation.
- [ ] Every incident is reproducible or bounded with a corrective action.
- [ ] The agent demonstrates value beyond deterministic automation on named cases
      (else fall back to the Checkpoint-A decision).
- [ ] Operational load is measured and acceptable.
- [ ] A third party can audit the evidence without privileged internal knowledge.
- [ ] `make ci-check` green; independent adversarial review recorded.

**Checkpoint D (after P8):** do external operators receive enough value to justify
the operational complexity? If no, do **not** declare 1.0 — reduce scope or
reposition as a research/evaluation platform.
