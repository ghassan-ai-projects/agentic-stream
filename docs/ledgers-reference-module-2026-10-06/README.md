# Lifecycle ledgers as reference modules — October 2026

Working record for migrating `episodeledger`, `scheduleledger` and `approvalledger` to the
reference-module standard, following the
[reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md), and for
deciding whether they should be merged. Backward compatibility is not a goal. The package
vocabulary lives beside the code, in `internal/<pkg>/UBIQUITOUS_LANGUAGE.md`.

- [Findings](FINDINGS.md)
- [Merge decision](MERGE_DECISION.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

The three ledgers are the shared, transaction-scoped writers of the pipeline's lifecycle tables:
`episodes`, `episode_attempts`, `episode_rejections` (episode ledger), `scheduler_items` (schedule
ledger) and `approvals` (approval ledger). Each operation runs on the caller's transaction so a
lifecycle change commits with the work that caused it.

Decision: **merge `scheduleledger` into `episodeledger`; keep `approvalledger` separate** (and make
it independent of `notify`). Then layer both as facade, app, domain and store. See
[MERGE_DECISION.md](MERGE_DECISION.md) for the evidence.

```
internal/episodeledger/                 facade: vocabulary aliases, operations on the caller's *sql.Tx
internal/episodeledger/internal/app/    use cases: admit, start attempt, transition, reject, recover,
                                        supersede, queue (upsert, admit, coalesce, next pending)
internal/episodeledger/internal/domain/ vocabulary and rules: statuses, identity and fence checks,
                                        transitions, rejection reasons, recovery terminal, queue item
internal/episodeledger/internal/store/  the only SQL for episodes, episode_attempts, episode_rejections,
                                        scheduler_items
internal/approvalledger/ (same shape)   approvals
```
