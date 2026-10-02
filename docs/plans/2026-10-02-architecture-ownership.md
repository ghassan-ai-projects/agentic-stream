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
