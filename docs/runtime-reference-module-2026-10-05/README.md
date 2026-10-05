# Runtime reference-module migration

Baseline: `e8f33a0`. Migrate live pipeline orchestration, runtime readiness and
worker resource ownership to the authority/device/policy reference patterns.
Runtime owns composition and sequencing, not foreign lifecycle tables.

Target: thin facade → composition → app → domain and store/transport adapters.
Composition builds concrete dependencies; app coordinates domain operations;
store owns transaction plumbing; transport owns files, sockets, TLS and executor
connections. Package guides live under `internal/runtime/`.
