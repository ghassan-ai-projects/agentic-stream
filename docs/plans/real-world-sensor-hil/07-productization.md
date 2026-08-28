# Phase 07 — Productization & operator experience (M4.5 → roadmap Phase 7)

> **Longer-horizon skeleton.** Start only after Phase 06 (M4) is green. This is
> the roadmap's Agentic Stream priority **4 (physical-path operator APIs)** plus
> the packaging that pilots (Phase 08) depend on.

**Goal:** an operator can install the runtime, see the whole physical path, and
safely intervene — **without direct SQLite access**. "Physical systems need
operator truth more than dashboard decoration" (`MATURITY_ROADMAP.md` §D).

Scope note: build **read-only views + explicit safe operations**, not a dashboard
builder (an explicit deferral). Reuse the existing HTTP/SSE surface
(`internal/api`: today `/v1/events`, `/health/*`, `/metrics`, `/control/kill`,
`/control/drain`). Everything here is additive to that.

---

## Task 7.1 — Physical-path read APIs

Add read-only endpoints (JSON, plus SSE where a live feed helps) that expose the
existing durable ledgers for the physical loop:

- Situations, Decisions, Commands, Outcomes, Verifications — filterable by tenant,
  entity, and time.
- Current **authority**: which runtime epoch/owner holds the target
  (`storage.EpochControl` / `RuntimeOwner`), and the current device boot id.
- **Last verified effect** per target, and the current **safe-state reason** if a
  target is disabled or interlocked (`internal/interlock`).

These must answer, without a database query, the roadmap SLO: *the operator can
identify current authority, last verified effect, and safe-state reason.*

**Exit:** every physical transition from an M2/M3 run is inspectable through a
supported endpoint; no operator task requires reading SQLite.

---

## Task 7.2 — Explicit operator operations

Wrap the operations pilots need as reviewed, authenticated endpoints/CLI verbs —
each one a deliberate, logged action, never a bulk mutation:

- **Approval** — resolve an R2 approval (already modeled in
  `internal/policy`); expose it through a supported surface with approver
  identity and expiry, replacing any test channel from Phase 03.
- **Reconciliation** — trigger/inspect `Dispatcher.ReconcileUnknown` for an
  `outcome_unknown` command, supplying operator evidence; never a blind retry.
- **Safe-disable** — one operation that disables the physical action path and
  **proves** disablement (trips `interlock.Set` + confirms no effector can reach a
  live port). Roadmap SLO: *one command disables and proves disablement.*

**Exit:** approval, reconciliation, and safe-disable all run through supported
surfaces; each emits durable audit evidence.

---

## Task 7.3 — Config validation, preflight, and health of the whole path

- `agentic-stream validate` extended with a **preflight** for the physical profile:
  capability catalog well-formed, bounds present, deployment-profile mutual
  exclusion (Phase 03 Task 3.6) satisfied, interlock reachable.
- Health/readiness (`/health/*`) extended to cover the **complete physical path**
  — device session up, authority held, interlock ready — not just the process.
- Metrics/alerts (`/metrics`) surface the safety and reliability counters from
  Phase 04 Task 4.5 as first-class series.

**Exit:** a misconfigured physical deployment fails preflight with an actionable
message; readiness is red whenever the physical path is not safe to command.

---

## Task 7.4 — Packaging, artifacts, and lifecycle

- Supported install/packaging for the gateway+runtime combination (documented,
  reproducible); **no actuator is live by default.**
- Device/capability **inventory API**.
- Backup/restore + evidence-retention policy over the durable stores
  (`internal/storage`), aligned with the spec's `retention` block.
- A **version/compatibility matrix** across firmware, gateway, contracts, Agentic
  Stream, and Tamoz; signed release artifacts + SBOMs; documented upgrade,
  rollback, and decommission procedures that **preserve or safely invalidate
  device authority**.

**Exit:** a fresh-machine install from released artifacts reaches first simulated
observation in <30 min; upgrade and rollback preserve/invalidate authority
correctly.

---

## Gate P7 exit checklist

- [ ] Fresh-machine install succeeds from released artifacts.
- [ ] Recovery and reconciliation require no direct SQLite editing.
- [ ] Operator can inspect every physical transition through supported surfaces.
- [ ] Upgrade and rollback preserve or safely invalidate device authority.
- [ ] Documentation + diagnostics localize common setup failures (tested by a
      second operator).
- [ ] No actuator live by default; safe-disable proves disablement.
- [ ] `make ci-check` green; security review of the new surfaces recorded.
