# Recovery runbook

This is the safe recovery sequence for the current local runtime. It is a
runbook outline, not a replacement for a deployment-specific disaster-recovery
plan.

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

Malformed/schema-invalid input is quarantined. A released row is validated
again before redrive. Gap records preserve a discontinuity and must be resolved
from the source of truth; they are not synthetic events.

The current public CLI and HTTP surface does not expose quarantine release or
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
resuming ingress. The repository does not provide a turnkey backup/restore CLI.

## 7. Notification recovery

SSE consumers resume with `Last-Event-ID` while the cursor remains within
retention. An expired cursor needs an audited resnapshot. Consumers must
deduplicate by CloudEvent source/id because delivery is at-least-once.

## Source evidence

- Runtime recovery: [`internal/runtime/recovery.go`](../../internal/runtime/recovery.go)
- Owner fencing: [`internal/storage/runtime_owner.go`](../../internal/storage/runtime_owner.go)
- Quarantine/redrive: [`internal/eventlog/quarantine.go`](../../internal/eventlog/quarantine.go)
- Episode recovery: [`internal/episodes/recovery.go`](../../internal/episodes/recovery.go)
- Action recovery: [`internal/actions/dispatcher.go`](../../internal/actions/dispatcher.go)

## Next reads

- [Durability](../architecture/durability.md)
- [Migration reference](../reference/migrations.md)
- [Troubleshooting](../guides/troubleshoot.md)
