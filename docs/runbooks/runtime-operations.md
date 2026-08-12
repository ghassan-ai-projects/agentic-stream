# Agentic Stream runtime operations

These procedures are the operator checklist for the single-node Go runtime.
They are intentionally explicit about which steps are automated and which need
an environment rehearsal before a production release.

## Start and verify readiness

```bash
agentic-stream serve --db runtime.db --listen 127.0.0.1:8080
curl --fail http://127.0.0.1:8080/health/live
curl --fail http://127.0.0.1:8080/health/ready
curl --fail http://127.0.0.1:8080/metrics
```

The runtime creates a fresh owner epoch, recovers unfinished attempts and
evidence leases, and only then reports ready. A readiness problem is RFC 9457
`application/problem+json`; do not route traffic to an unready process.

## Backup and restore

1. Stop writes or use SQLite's online backup API from the deployment wrapper.
2. Copy the backup to a separate host and open it read-only with the same
   migration binary.
3. Run `PRAGMA integrity_check` and compare the highest event position,
   notification cursor, and schema migration version with the source.
4. Restore by replacing the stopped process's database, then start and verify
   `/health/ready` before replaying any ingress checkpoint.

Never copy an active WAL database with ordinary file-copy tools.

## Unclean shutdown and WAL recovery

After a process crash, start the same database normally. The new owner epoch
must reclaim expired evidence leases and abandon/reissue unfinished attempts
according to the durable lifecycle tables. Check the recovery report in the
structured startup log and verify that stale epochs cannot write afterward.

## Disk-full response

Stop ingress, preserve the database and WAL files, and free capacity on the
same filesystem. Do not delete the WAL or event log to recover space. Restart
only after `PRAGMA integrity_check`, then resume from the connector checkpoint;
duplicate envelopes are rejected by the durable inbox.

## Notification consumers

Connect to `/v1/events?tenant=<tenant>` and persist the last numeric `id`.
Reconnect with `Last-Event-ID`. A `cursor_expired` problem requires an audited
resnapshot before reconnecting. A `subscriber_too_slow` event means the client
must reconnect from its last acknowledged cursor; the server never grows an
unbounded subscriber buffer.

## Worker and secret boundaries

Use a private Unix socket for local Go workers. Use TLS 1.3 with a client
certificate for remote worker connections. Capability tokens are per-attempt,
short-lived, and scoped to evidence; they are not subscriber credentials and
must never be logged. Configure model credentials through
`AGENTIC_STREAM_MODEL_API_KEY`, not command-line arguments.

## Release rehearsal record

Before Gate D sign-off, attach measured results for backup/restore, unclean
shutdown, disk-full refusal, a 24-hour bounded soak, and the security review
covering sockets, tokens, poisoning, API binding, secrets, and artifact
retention. Unit tests prove the semantics; they do not replace these
environment-level rehearsals.
