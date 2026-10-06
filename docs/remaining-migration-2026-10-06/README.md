# Remaining migration, merges and dead code (2026-10-06)

Plan for finishing the move of every package to the reference-module standard
([playbook](../../.agents/prompts/reference-module-refactor.md)), for merging or
deleting packages that do not earn their own module, and for resolving the
production-unreachable code. Nothing here has been implemented yet; this folder
is round 0 and the place to track progress.

| File | Contents |
| --- | --- |
| [SURVEY.md](SURVEY.md) | Every package not yet layered, with evidence and a decision: migrate, merge, delete, or leave |
| [DEADCODE.md](DEADCODE.md) | Fresh `deadcode ./...` run (182 symbols) and a decision per item: delete, move to test support, wire, or keep |
| [PLAN.md](PLAN.md) | Rounds, proof per round, behavior that must not change, owner decisions, and the status table to fill in |
| [deadcode-production-unreachable.txt](deadcode-production-unreachable.txt) | Raw output at this commit |

Companion records: [migration status](../reference-module-migration-status-2026-10-06/README.md)
(what is already migrated), [package merge review](../package-merge-review-2026-10-06.md).

## Result in one paragraph

19 packages are migrated or dissolved. Four unmigrated packages carry real
structure and should be layered: `executor/native`, `executor/remote`,
`runartifact` (absorbing `soak`) and `worker` (split: its reference server is
test support). Two small changes remove test-only code from production packages:
`contractsv1` conformance fixtures and the `executor/native` batch runner. One
package merged (`soak` into `runartifact`, done); a second candidate
(`eventschema` into `spec`) was examined and rejected. Nothing is deleted wholesale. Of the 182 unreachable symbols, 17 are plain
deletions, 49 are test support that moves out of production packages, and 116
are operator levers with no production caller (93 are `replay` modes, 14
`eventlog` quarantine/gap, 7 `notify` pruning, 2 `interlock`), which need an
owner decision to wire or remove.

## Target shape

```text
Layered (done)          Layered (this plan)          Leave as is
authority device ...    executor/native              interlock  api  clock  storage
(19 packages)           executor/remote              telemetry  actionport  ids
                        runartifact (soak merged)    duration  situations  operators
                        worker (protocol + sockets)  contractsv1  eventschema (pure)
                                                     executor/fixture, conformance, spec
```
