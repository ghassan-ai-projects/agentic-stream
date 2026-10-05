# Dead and test-only code audit

Used the repository-pinned tools:

```sh
go run golang.org/x/tools/cmd/deadcode ./cmd/...
go run golang.org/x/tools/cmd/deadcode -test ./...
```

The test-aware scan reports zero unreachable functions. The CLI scan reports
no evidence candidates after migration. Unrelated existing module APIs remain
outside this scope; CLI reachability alone is not a deletion authorization.

| Candidate | Evidence | Decision |
| --- | --- | --- |
| Mutable public Server, Issuer, Verifier and Ledger | Runtime/remote consumers now use New(Config) and Service | Remove public structs; private components own their use cases; update every consumer |
| Reserve, Complete and Fail public ledger methods | Only local tests called them directly; worker server calls private use cases | Keep behavior privately in app, move behavior tests there; remove public surface |
| Recover convenience method | Only module tests called it; live heartbeat uses ReclaimExpired and startup uses RecoverTx | Remove convenience; tests pass the explicit fixture time to ReclaimExpired |
| In-memory calls map, mutex, reserveInMemory and retry cleanup | Production server always configured RequireLedger; only tests used the alternative | Remove; completed calls now replay durable bytes in tests, failed calls remain terminal |
| RequireLedger flag and nil-owner bypass | Mutable switches could omit required safety | Replace with validated call/ledger configuration and required explicit owner assertion |
| Issuer.ClockSkew | Never read; skew belongs to verification | Remove unused issuer field; keep configured verifier skew |
| Reservation.ResultSHA256 | Assigned but never read | Remove the redundant carried field; persisted result_sha256 still verified before replay |
| Event result maps | Envelope fields are closed; payload is schema-owned arbitrary data | Use wire.EventRecord and a typed rows envelope; preserve exact lexical JSON field order and payload map |
| EventLogQuery | Runtime still uses it; delegates to eventlog.ReadEntityWindow | Keep public adapter construction; move source integration to transport and encoding to wire |
| NewRuntimeEpoch / IssueTime | Startup and remote attempt issuance need random identity/configured time | Keep facade delegation to preserve owner identity and clock-read ordering |

Database setup remains in test fixtures. No authorization rules, token codecs
or query executors were copied into test support. Behavioral tests exercise the
real configured service and durable ledger, including the private UDS path.
