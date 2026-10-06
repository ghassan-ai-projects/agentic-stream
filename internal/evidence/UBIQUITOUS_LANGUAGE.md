# Evidence language

| Term | Meaning | Code | Storage/wire |
| --- | --- | --- | --- |
| Capability scope | Short-lived authorization for one fenced worker attempt | Scope | v1 token claims |
| Evidence call | Bounded request authenticated by the scope | Call | EvidenceToolCall |
| Reservation | Durable claim of one call identity | Reservation | evidence_call_ledger |
| Request fingerprint | Exact stable request identity hash | RequestSHA256 | request_sha256 |
| Bounded result | Exact response bytes and row count | QueryResult | result_json, row_count |
| Runtime epoch | Owner identity shared by token, ledger and worker | RuntimeEpoch | runtime_epoch |
| Recovery | Interrupt unfinished prior-epoch calls atomically | RecoverTx | interrupted/runtime_restart |
| Reclamation | Interrupt expired or foreign-epoch running calls | ReclaimExpired | interrupted/lease_expired |

| Retired words/API | Replacement | Reason |
| --- | --- | --- |
| Mutable Server/Issuer/Verifier/Ledger | Configured Service | Consumers use a validated facade |
| RequireLedger flag | Required call configuration | No optional durable safety on worker calls |
| In-memory calls map | Durable reservation | Old path only served tests |

The Service facade delegates operations; app sequences use cases; domain decides
scope and lifecycle validity; store owns evidence_call_ledger; wire encodes;
transport adapts gRPC and the eventlog-owned source. See [module guide](README.md).
