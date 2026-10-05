# Actions reference-module migration

`internal/actions` owns approved-command delivery and its durable outcome, verification, and reconciliation records. This migration separates the current mixed dispatcher into a public configured facade, ordered use cases, pure decisions, exact document codecs, and a private SQL owner while preserving the policy-to-effector boundary and caller-owned transaction checks.

```text
internal/actions (configured facade, layer 7)
  └── internal/app (dispatch and reconciliation use cases, layer 6)
        ├── internal/domain (decisions and document checks, layer 2)
        ├── internal/store (opaque transactions and owned SQL, layer 5)
        └── actionport (effector boundary, layer 0)
```

The package language is canonical in [`internal/actions/UBIQUITOUS_LANGUAGE.md`](../../internal/actions/UBIQUITOUS_LANGUAGE.md). This folder records findings, design, and round proofs for the 2026-10-05 migration.
