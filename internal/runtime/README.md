# Runtime module

Runtime owns live orchestration and process resource lifetimes. It composes
existing table-owning modules; it does not own their domain tables. The
reference-module migration separates public delegation, composition, app use
cases, pure domain rules, transaction plumbing and transport adapters.

Migration is in progress. The [design](../../docs/runtime-reference-module-2026-10-05/DESIGN.md)
and [plan](../../docs/runtime-reference-module-2026-10-05/PLAN.md) describe the
boundaries and preservation requirements. [Runtime language](UBIQUITOUS_LANGUAGE.md)
records the domain vocabulary. Public approval routes are documented in the
[HTTP reference](../../documentation/reference/http-api.md).
