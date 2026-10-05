# Plan

| Round | Scope | Proof |
| --- | --- | --- |
| R0 | Survey, layer design, this folder | Full CI baseline green on `24ab153` |
| R1 | Domain layer: vocabulary, ports, and every pure rule extracted with table tests; layers registered in gates and repository map | Domain tests, untouched facade golden/thermal tests, lint, architecture tests |
| R2 | Store (all SQL and transactions), transport (trace files, isolated DB, ingestion), app (sessions, modes, capability phases), facade reduced to aliases and delegation; boundary tests move to their owners | Full replay test suite unchanged at facade level, each new package ≥ 60% coverage, SQL/store and app/infrastructure gates |
| R3 | Gates extended to replay layers, module README and language guide, AGENTS.md and architecture context updates, self-review | Injected-violation proof for each new gate, `make ci-check`, rating report |

## Behavior that must not change

- Error strings and precedence: capability-set count check first, then
  deterministic shortcut, then unsupported-mode, then capability validation;
  recorded verification order (completeness → provenance → canonical JSON →
  schema → digest → metadata → snapshot → attempt); shadow binding order
  (manifest → canonical decision → decision digest → catalog validation).
- Identity and digest inputs: episode keys, versions hash input order,
  canonical decision bytes, comparison documents and digests, baseline decision
  and intent construction, `comparison_id = "cmp_" + hex`.
- Clock semantics: epoch derivation (ingress-valid lines only, Unix origin
  fallback), record-time advancement rule, wall time only in comparison
  `created_at`.
- Transaction boundaries: episode materialization in one transaction; one
  transaction per shadow comparison; database closed per session.
- Fail-closed behavior: missing capabilities are constructor-time errors before
  any replay work; an existing database or WAL sidecar is rejected.
- Golden fixtures: predictive-maintenance traces, thermal-chamber traces,
  quarantine counts, duplicate-command retention, shadow reproducibility.

## Deliberate behavior changes

None. The migration is behavior-preserving; the public API keeps every symbol
(test-only production surface included — it is the documented shadow-mode
acceptance contract).

## Deferred follow-ups

- Wire worker-aware modes into the CLI (design acceptance item, separate change).
- Type `Result.SimulatedResults` beyond the simulator port contract.
- Remove the duplicated snapshot entity decode during shadow input load once a
  typed snapshot record exists in domain.

## Status

| Round | Status |
| --- | --- |
| R0 | Accepted, `514200d` |
| R1 | Accepted: domain layer extracted with table tests; facade aliases domain types and delegates moved rules; layer registered in gates and repository map. Facade golden/thermal tests unchanged and green; domain coverage 70.9% |
| R2 | Accepted: all SQL and transactions moved to store (worklist, digests, snapshots, materialization, comparison persistence); trace files, isolated databases and ingestion to transport; session sequencing, mode dispatch and capability phases to app; facade reduced to aliases and one-line delegation. Facade golden/thermal tests unchanged and green. Coverage: facade 100%, app 75.5%, domain 70.9%, store 81.1%, transport 92.3% |
| R3 | Pending |
