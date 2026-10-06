# Notify as a reference module — October 2026

Working record for rebuilding `internal/notify` (and merging `internal/notifycontract`
into it) to the reference-module standard, following the
[reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md).
Backward compatibility is not a goal.

- [Findings](FINDINGS.md)
- [Ubiquitous language](../../internal/notify/UBIQUITOUS_LANGUAGE.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

`notify` owns the durable Channel-B notification outbox: five tables, tenant-local
gapless cursors, event deduplication with retention tombstones, bounded paged
reads with lag and poison handling, and the versioned lifecycle contract that
every producer's event must satisfy. One package mixed SQL, rules, contract
validation and audit writes; `notifycontract` was a separate package only the
notify package imported. Eight call sites passed ten positional arguments.

The 2026-10-06 status review left `notify` unmigrated because the layer table
would have to be re-levelled. This round does the re-levelling: only the
`approvalledger` → `policy` chain moves (the importer graph of `notify` is five
packages), see [PLAN.md](PLAN.md).

```
internal/notify/                  facade: Append, AppendLifecycleEvent, SourceForTenant, Service{ReadPage, Prune}
internal/notify/internal/app/     use cases: append, lifecycle append, read page, prune
internal/notify/internal/domain/  rules: lifecycle contract (schema + binding), sealing, dedupe/tombstone
                                  decisions, retention, resume/lag, poison policy, audit details
internal/notify/internal/store/   the only SQL for notifications, cursors, tombstones, poison attempts, audits
```
