# Architecture ownership improvement rounds

Baseline: `4b58e44`, branch `code-improvments-2`. Pre-existing untracked
`agentic-stream` executable is outside this work. No push is requested.

## Plan

1. Commit the accepted design and measurable architecture bar before extraction.
2. Move runtime control, device authority and qualification out of storage.
3. Separate effect ports, governed dispatcher and concrete device adapters.
4. Route episode/scheduler lifecycle mutations through owning ledger modules.
5. Enforce layer ordering, SQL ownership, contract isolation and transitive safety.
6. Review evidence against A1–A7, fix findings in committed rounds, and run full gates.

Mechanical moves carry existing regression suites with their owning modules.
Tests for changed boundary contracts are added in the same round; no behavior
change is intended, so unchanged acceptance fixtures remain the reference.

## Rounds

- Round 0: accepted ADR-017, design refinement and architecture bar. Implementation pending.

- Round 1: extracted `control`, `authority`, and `qualification`; `storage` now imports only migrations. Migrated authority/lease/epoch tests and added shadow transaction rollback, duplicate-evidence and inert-action checks. Focused race tests passed across nine affected packages; full-tree lint reports zero issues. New package coverage: control 79.9%, authority 63.5%, qualification 80.0%; storage 80.6%. Cross-package errors retain their original text and unwrap causes. A4 lifecycle ownership and A3 adapter isolation remain pending.

- Round 2: extracted the `actionport` boundary and `device` adapters; moved concrete route composition to `runtime`. Dispatch imports no device implementation. Added unknown-outcome cause preservation, denied/missing authorization, unavailable-device fail-closed routing and verification-routing tests. A typed nil device pointer is not boxed into the composition's interface. Focused race/coverage passed: actions 68.9%, actionport 100.0%, device 79.6%, runtime 67.8%, CLI 68.5%; final focused tests and whole-tree lint pass. Lifecycle ownership enforcement remains pending.

- Round 3: `episodeledger` exclusively owns episode, attempt and rejection mutations. Cognition supersession and epoch kill call its concrete transaction-scoped APIs. Execution retains its request assembly, budget, telemetry and provider responsibilities. Added admission/shadow-default and live-conflict tests, composed mutation rollback, cancellation recovery, and idempotent unknown-worker rejection audit. Focused tests/race tests pass across lifecycle, execution, control, cognition, runtime, native/worker conformance and replay; zero lint issues. Episode ledger coverage 63.2%, episodes 72.4%. Queue and approval lifecycle ownership remain pending.

- Round 4: `scheduleledger` owns queue admission, coalescing and skipped/cost-rejected transitions; `approvalledger` owns request/assertion/decision/expiry/supersession state. Approval permission and signature checks remain in policy. Runtime control now supplies the read-only final authorization capability, eliminating a dispatcher callback. Added queue conflict identity/rollback, pending-only skip, terminal approval/assertion binding, atomic withdrawal notification, and current-interlock/read-only/misconfiguration regressions. Focused race/final tests and full-tree lint pass; coverage schedule ledger 77.6%, approval ledger 69.4%, control 80.7%. ADR-017 records the discovered approval boundary. Next round pins ownership and transitive reachability mechanically.
