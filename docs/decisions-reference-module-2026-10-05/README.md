# Decisions reference-module migration

`internal/decisions` contains pure validation rules today, but its exported
facade and implementation share one package. This migration will keep the
public operations as a small facade, move parsing, schema checks, catalog
compilation and binding rules to `internal/domain`, and leave persistence and
transport outside the module. The work preserves worker contract bytes,
digests, rejection ordering and fail-closed checks.

```text
episodes / replay
       |
       v
internal/decisions                 public types and delegating operations
       |
       v
internal/decisions/internal/domain pure validation and compiled authority
       |
       +--> canonicaljson / contractsv1 / JSON Schema compiler

episode store owns Decision records; policy store owns Intent governance.
```

The dated findings, language, design and rounds are in this folder. The
canonical in-package vocabulary is
[`internal/decisions/UBIQUITOUS_LANGUAGE.md`](../../internal/decisions/UBIQUITOUS_LANGUAGE.md).
