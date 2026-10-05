# Cognition reference-module migration

This record plans the migration of `internal/cognition` to a configured facade,
ordered application use cases, pure trigger and reconsideration rules, and an
opaque transaction store. It follows
the [reference-module prompt](../../.agents/prompts/reference-module-refactor.md).

```text
engine / admission
        │ caller-owned transaction
        ▼
cognition facade (layer 8)
        │
        ▼
application (layer 7) ───────► domain rules (layer 5)
        │                         ▲
        └─────────────────────────┘
        ▼
store (layer 6) ──► storage and same-transaction ledger/notification owners
```

The caller continues to own the transaction boundary. Cognition joins that
exact transaction through a private store value; the facade exposes neither the
SQL handle stored by the module nor persistence operations.

The [canonical package language](../../internal/cognition/UBIQUITOUS_LANGUAGE.md)
is maintained beside the package. This dated folder contains the survey,
design, round plan, audit and validation record.
