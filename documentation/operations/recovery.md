# Recovery runbook

Follow this sequence when recovering the local runtime. A production
deployment also needs a disaster-recovery plan tested in its own environment.

## 1. Preserve evidence

Record the runtime version/commit, database path, spec digest, trace source,
owner epoch if available, UTC time, and the exact error. Preserve the database
and input trace before attempting redrive or migration. Never overwrite the
only copy.

## 2. Confirm ownership

The runtime uses a durable owner lease and epoch. Stop or fence the prior
process before starting a replacement against the same database. A second
unexpired owner should be refused. An expired owner can be replaced, while
the old epoch cannot renew or mutate state.

## 3. Restart and inspect readiness

Start with the same database and compatible spec/configuration. Check:

```bash
curl http://127.0.0.1:8080/health/live
curl http://127.0.0.1:8080/health/ready
```

If readiness fails, inspect migrations, owner recovery, evidence ledgers,
episode attempts, and the runtime logs. Do not bypass the readiness check by
deleting durable rows.

## 4. Handle input quarantine and gaps

Malformed input and input that fails schema validation are quarantined. A released row is validated
again before redrive. Gap records identify missing input. Resolve them using authoritative source
data rather than inventing replacement events.

The current public CLI and HTTP API do not expose quarantine release or
redrive. The underlying release path is an internal API; use only approved
deployment tooling and preserve the audit trail. Do not invent a public
endpoint or edit the database directly.

## 5. Handle episodes and effects

Recovery can abandon prior-epoch attempts and resume eligible work with a new
attempt/fence. Late output from the old attempt must be rejected. For commands,
inspect outbox lease, command status, idempotency key, outcome, and
reconciliation state. Unknown outcomes require provider-side reconciliation;
never blindly replay them.

## 6. Restore and backup

**Deployment responsibility, not a packaged command:** stop writes or use the
SQLite online backup API from the deployment wrapper; never copy an active WAL
database with ordinary file-copy tools. In an isolated restore directory, open
the backup read-only, run `PRAGMA integrity_check`, compare the migration
version and highest event/notification cursors with the source, then restore
only while the runtime is stopped. Start the runtime and verify readiness before
resuming ingress. The repository does not provide a backup or restore CLI.

## 7. Notification recovery

SSE consumers resume with `Last-Event-ID` while the cursor remains within
retention. An expired cursor needs an audited resnapshot. Consumers must
deduplicate by CloudEvent source/id because delivery is at-least-once.

## Source evidence

- Runtime recovery: [`runtime recovery store`](../../internal/runtime/internal/store/recovery.go)
- Owner fencing: [`internal/control/owner.go`](../../internal/control/owner.go)
- Quarantine/redrive: [`internal/eventlog/quarantine.go`](../../internal/eventlog/quarantine.go)
- Episode recovery: [`internal/episodeledger/internal/app/recovery.go`](../../internal/episodeledger/internal/app/recovery.go)
- Action recovery: [`internal/actions/internal/app/lease.go`](../../internal/actions/internal/app/lease.go)

## Next reads

- [Durability](../architecture/durability.md)
- [Migration reference](../reference/migrations.md)
- [Troubleshooting](../guides/troubleshoot.md)
