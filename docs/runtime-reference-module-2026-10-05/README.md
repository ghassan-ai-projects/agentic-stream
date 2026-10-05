# Runtime reference-module migration

Baseline: `e8f33a0`. Migrate live pipeline orchestration, runtime readiness and
worker resource ownership to the authority/device/policy reference patterns.
Runtime owns composition and sequencing, not foreign lifecycle tables.

Target: thin facade → composition → app → domain and store/transport adapters.
Composition builds concrete dependencies; app coordinates domain operations;
store owns transaction plumbing; transport owns files, sockets, TLS and executor
connections. Package guides live under `internal/runtime/`.

Migration completed. The canonical [module guide](../../internal/runtime/README.md)
and [language](../../internal/runtime/UBIQUITOUS_LANGUAGE.md) live at the package
root; this folder retains the dated design, rounds and [validation](VALIDATION.md).
