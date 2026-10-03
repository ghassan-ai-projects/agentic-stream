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

- Round 1 (A9): added package comments for `authority`, `control`, `decisions`,
  `device`, `engine`, `episodeledger`, `qualification` and `scheduleledger`.
  `TestEveryPackageDocumentsItsResponsibility` and `TestModuleMapListsEveryPackage`
  pin the rule; both were checked to fail on a removed comment and a removed map entry.

- Round 2 (A8): moved the streamed EpisodeWorker adapter and the per-attempt
  evidence capability issuer from `episodes` to `internal/executor/remote`
  (`remote.Executor`). `episodes` exports `BudgetExceededError` and
  `BudgetTelemetryMissingError` as the port's failure contract and no longer
  imports gRPC, protobuf, `worker`, `evidence` or the protocol package. The
  adapter maps gRPC `Canceled`/`DeadlineExceeded` onto the matching context
  errors while keeping the original message and status, so durable failure
  reasons are unchanged. New tests pin the mapping and the import rule; the
  architecture allowlist, layers, forbidden edges and reasoning-reachability
  sources include the new package. Focused race tests pass for episodes,
  executors, runtime and replay (coverage: remote 72.1%, episodes 72.7%).

- Round 3 (Q2/Q7): foundation and ledger packages (`approvalledger`, `clock`,
  `costcontrol`, `duration`, `eventschema`, `executor/conformance`, `interlock`,
  `notifycontract`, `qualification`, `scheduleledger`, `storage`, `telemetry`,
  `migrations`) now have no production function over 15 lines. Long SQL moved to
  named constants; cursor loops delegate one row to a named step; the metrics
  snapshot is a named counter table. Error text, ordering and transactions are
  unchanged. Focused race tests and lint pass.
