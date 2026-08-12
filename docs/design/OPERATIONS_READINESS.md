# Operations readiness evidence

This document is the future Gate D evidence index for the single-node runtime.
Environment-level production rehearsals are postponed for the current build by
explicit scope decision.

## Proved by the repository

- SQLite migrations run transactionally and schema version is asserted by
  `internal/storage/storage_test.go`.
- Replay creates an exclusive private reservation, rejects normal opens while
  reserved, and never accepts production credentials or effectors.
- W3C `traceparent`/`tracestate` are validated at the contract boundary,
  retained in the event log, propagated to Situation versions, and included in
  episode requests. Asynchronous consumers must use the stored context as a
  span link.
- `/health/live` is process liveness. `/health/ready` returns RFC 9457 Problem
  Details and must fail closed when readiness is unavailable.
- `go test ./...`, `go test -race ./internal/...`, `go vet ./...`, and
  `git diff --check` are the clean-checkout correctness commands.

## Postponed production rehearsals

The future production release should record results for:

1. backup/restore equality across schema migrations;
2. unclean shutdown and WAL recovery;
3. disk-full refusal without dropping ingress evidence;
4. a 24-hour bounded soak with memory, queue, timer, WAL, and database-growth
   thresholds recorded; and
5. security review of worker sockets, token scope/expiry/rotation, API binding,
   event poisoning, secrets, and shadow-artifact retention.

These are environment-dependent acceptance gates, not claims inferred from
unit tests. They do not block the current build. No legacy status compatibility
is part of the release contract.
