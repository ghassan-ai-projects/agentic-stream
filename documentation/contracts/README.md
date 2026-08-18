# Contracts

These pages explain the boundaries that other components can rely on. The
machine-readable artifacts remain the authority; the pages below are the human
contract guide.

## Authority map

| Contract | Machine authority | Generated or mirrored artifact |
| --- | --- | --- |
| SituationSpec v1 | `internal/spec/schema.json` plus compiler semantics | `docs/design/contracts/situation-spec-v1.schema.json` is the design/archive copy |
| Normalized event envelope | `internal/contractsv1/envelope.go` and ingress validation | `docs/design/TECHNICAL_DESIGN.md` describes the projection |
| Snapshot/Decision/Intent/Command/Outcome | `internal/contractsv1/schemas/v1/` and `internal/contractsv1/` tests | schema pages and protocol payloads |
| Notification contract v1 | `internal/notifycontract/contracts/` and `internal/notifycontract/contract.go` | `docs/contracts/` contains the repository-level mirror |
| Worker protocol | `docs/design/contracts/runtime-v1.proto` | generated Go stubs under `proto/` |
| SQLite persistence | ordered files under `migrations/` | `docs/design/contracts/storage-schema-v1.sql` is a design baseline |

When two artifacts differ, the public documentation must say which one the
running code loads. Do not update a mirrored or generated artifact by hand and
call the contract changed.

## Versioning rules

The runtime reports:

- contract version: `situation-runtime-contracts/v1`;
- worker protocol: `agenticstream.runtime/v1`.

Documents that cross a process or repository boundary carry explicit schema or
protocol versions and canonical digests. A semantic change needs a reviewed
versioning decision, test vectors, and migration/compatibility notes.

## Pages

- [SituationSpec](situation-spec.md)
- [Event envelope and ingress](event-envelope.md)
- [Decision and Intent](decision-intent.md)
- [Notifications](notifications.md)
- [Worker protocol](worker-protocol.md)
- [Persistence and migrations](persistence.md)

## Next reads

- [CLI reference](../reference/cli.md)
- [Worker integration guide](../guides/build-a-go-worker.md)
- [Architecture invariants](../architecture/invariants.md)
