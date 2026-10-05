# Design

Facade exposes New(Config), Scope/Call/QueryResult aliases, Issue, Verify, Call,
RecoverTx, ReclaimExpired and RuntimeEpoch. NewRuntimeEpoch delegates random
identity generation; EventLogQuery delegates the existing owner-provided read
adapter. Public operations contain no rules, codecs or SQL.

Configuration has explicit capability, ledger and call components. Capability
configuration validates signing/verifying keys and copies the key ring. Ledger
configuration requires DB, epoch, lease owner and an ownership assertion port.
Call configuration requires configured capability and ledger services, query
port and matching runtime epoch. Capability-only and recovery-only services
refuse operations they are not configured to perform.

| Layer | Owns | Must not |
| --- | --- | --- |
| app | Clocks, ID issuance, admission sequence, transactions through ports, deadlines/cancellation | SQL, raw DB handles, protobuf/gRPC |
| domain | Scope defaults/validity, envelope authorization, deadline/result bounds, attempt and reservation rules | I/O, clock reads, wire encoding |
| store | Private DB/Tx handles, SQL, ownership plumbing, eventlog read port | Permission/lifecycle decisions |
| wire | Typed v1 token claims, HMAC/base64, argument JSON, request fingerprint, protobuf conversion | Query/ledger orchestration or authorization |
| transport | gRPC request/result and status adaptation | Scope decisions, SQL, execution |

Preserve admission ordering: envelope, trace, token verification, numeric bounds,
identity/tool/trace/epoch, evidence time range, arguments/entity, deadline.
Preserve reserve ordering: ownership, live episode, attempt status, request
identity, token identity, stored result integrity. Completion repeats ownership
and live-attempt checks; providers run outside transactions. Detached completion
and failure persistence keeps the existing five-second budget.

Generic purity, application infrastructure, SQL-owner and dependency gates apply.
Add facade, transport isolation and opaque store tests; prove them with injected
violations. No schema, protobuf, token version or digest changes.
