# Findings

## Mixed responsibilities

`internal/evidence/server*.go` mixes gRPC records/status, argument parsing,
authorization decisions, query deadlines and durable orchestration.
`capability*.go` mixes clocks/random IDs, pure scope rules, JSON/base64 and HMAC.
`ledger*.go` mixes unit-of-work sequencing, lifecycle rules and SQL writes.

## Leaking public surface and optional safety

Public mutable Server, Issuer, Verifier and Ledger structs expose dependency
fields and key bytes. Ledger.Owner is attached later by runtime recovery; nil
silently skips ownership checks. Replace them with immutable configured facade
instances and required explicit ownership ports. Runtime recovery still joins
the original owner transaction.

Reserve, Complete, Fail and Recover have only module-test callers; public
consumers use RecoverTx and ReclaimExpired. In-memory Server deduplication has
only test callers; production server always requires a durable ledger. Remove
that test-only production path and test the actual durable path instead.

## Cross-module data access

Ledger reads episodes and episode_attempts to check the live fenced attempt.
Evidence alone writes evidence_call_ledger. No other production module writes
its table. Preserve existing read projections in store on the same transaction;
an owner read API is a future coordinated migration. EventLogQuery already
uses eventlog.ReadEntityWindow rather than foreign SQL.

## Records and vocabulary

Scope, Call and argument records are typed; request fingerprints intentionally
pin JSON field order rather than canonicalize a new representation. Event-log
result envelopes/rows can be closed records while retaining payload maps and
exact output bytes. Reservation status and lifecycle checks become explicit
domain vocabulary. No compatibility shims are needed.
