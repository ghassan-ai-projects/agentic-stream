# Phase 09 — Production qualification & 1.0 (M6 → roadmap Phase 9)

> **Longer-horizon skeleton.** Start only after Phase 08 (M5) is green and
> Checkpoint D clears. Indicative effort in the roadmap is *3–6 additional months
> after stable pilots.* This file records the bar; it is intentionally the least
> detailed, because the real design must be driven by pilot evidence and an
> independent assurance process, not pre-committed now.

**Goal:** production-ready **within the published non-safety-critical scope** —
fleet lifecycle, upgrade/rollback, compatibility policy, incident drills, and
**independent** assurance.

**Allowed claim after the 1.0 bar:** *production-ready within the published
non-safety-critical scope.* No level here implies certification for medical,
vehicle, life-safety, mains, heat, pressure, hazardous motion, or critical
infrastructure control — ever.

---

## Required capabilities (Agentic Stream slice)

Grouped by the durable/authority machinery this repo already owns, extended to
fleet scale:

**Identity & authority lifecycle**
- device enrollment, credential rotation, revocation, ownership transfer;
- durable configuration + authority migration across releases (build on
  `storage.EpochControl` / `RuntimeOwner`, the interlock, and the deployment
  versioning in `internal/spec/deployments.go`);
- multi-device isolation with bounded per-device resource behavior (roadmap
  Agentic Stream priority **6**).

**Upgrade & compatibility**
- firmware/gateway/runtime upgrade compatibility + rollback;
- a compatibility **deprecation policy** with conformance evidence for every
  supported adapter (from Phase 06 fixtures);
- HA decision for the gateway/runtime owner, **including explicit non-goals** (do
  not silently become a distributed control system).

**Assurance & operations**
- structured incident response + forensic export (extends the evidence pack);
- dependency/SBOM/vulnerability response process;
- independent threat model, penetration test, and safety review;
- long-duration endurance + power/network-partition tests;
- capacity model and tested limits;
- the platform can **refuse unsupported hardware/actions deterministically** —
  the closed capability catalog is the enforcement point.

---

## The 1.0 bar (all must hold)

- [ ] at least two successful external pilots and one independent operator;
- [ ] no unresolved critical safety/security finding;
- [ ] 30-day endurance on the supported reference topology;
- [ ] **all zero-tolerance invariants still zero** (the Phase 04 counters, at
      fleet scale);
- [ ] published SLOs and compatibility matrix backed by measurements;
- [ ] upgrade from the previous supported release **and** rollback both pass;
- [ ] every supported adapter has conformance evidence;
- [ ] documentation, release artifacts, and recovery procedures complete;
- [ ] legal review of product claims and excluded use cases.

---

## Cross-cutting workstreams (run through every phase, not just here)

From `MATURITY_ROADMAP.md` §Cross-cutting — these are not a final phase, they are
continuous and should already be alive from Phase 01:

- **A. Safety & authority** — threat-model the model, prompt, sensor metadata,
  gateway, operator, stale approvals, replay artifacts, and physical access.
  *Authority must always narrow as data crosses toward the device.*
- **B. Evidence & evaluation** — every claim needs a receipt; preserve the full
  chain (simulator truth → delivered evidence → Situation version → Decision →
  policy result → Command → receipt/result → independent observation →
  verification → fault schedule).
- **C. Developer experience** — golden examples that compile and run; every
  documented command executes in CI or is labelled conceptual; one complete
  reference rig beats many partial examples.
- **D. Operations** — reconciliation, safe disablement, upgrades, and retention
  designed before fleet scale.
- **E. Model value** — continuously compare Tamoz with the deterministic
  baseline; if the model does not improve exception handling, evidence
  acquisition, or explanation, keep it out of the actuation path.

---

## Governance loop (every phase, including this one)

1. acceptance bar + threat/safety delta;
2. implementation plan with exact repository ownership;
3. contract/schema review;
4. implementation + focused tests;
5. **independent** adversarial review;
6. real execution at the phase's evidence level;
7. correction loop;
8. release note stating passed evidence, failures, and prohibited claims.

*Do not start the next phase with an unresolved critical finding. A timeout,
blocked hardware run, or static-only review is incomplete evidence, not a pass.*
