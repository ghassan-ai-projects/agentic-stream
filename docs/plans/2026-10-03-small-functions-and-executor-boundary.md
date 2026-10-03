# Small functions and executor boundary rounds

Baseline: `6e6e2b8`, branch `code-improvments-3`. The untracked `agentic-stream`
executable is outside this work. No push is requested.

## Goal

1. Q2/Q7: every production function body is at most 15 lines and 15 statements,
   reads top-down as named domain steps, and keeps one level of abstraction.
2. A8: the episode lifecycle depends only on the `Executor` port; the streamed
   worker adapter moves to `internal/executor/remote`.
3. A9: every production package documents its responsibility, and the public
   module map lists every package.

Baseline measurement: 503 production functions exceed 15 body lines across 39
packages; 8 packages lack a package comment; `episodes` imports gRPC, protobuf,
the worker protocol and `internal/worker`.

## Module assessment

The packages already follow business capabilities (ADR-017). Two findings:

- `episodes` mixes lifecycle ownership with one concrete executor transport.
  This is a real module boundary, so it is extracted (A8).
- `device` is cohesive: the session consumes a `DeviceTransport` interface and
  the UDS transport implements it for the same gateway. No split is warranted.

No other package is split: package count is not a quality target.

## Rounds

- Round 0: tightened Q2 to 15 lines/statements, added A8/A9 and the ADR-017
  executor boundary refinement. Lint enforcement of 15 lines lands when the
  refactoring rounds reach zero findings.
