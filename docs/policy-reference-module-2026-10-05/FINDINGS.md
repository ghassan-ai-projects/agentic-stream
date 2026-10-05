# Findings

Baseline: `387fd41`, branch `general-improvements-1`. Policy has 1,616
production lines in 13 files; the initial policy and root tests pass.

## Mixed responsibilities

- `policy_evaluate.go` chooses risk outcomes while querying approvals,
  expiring ledger rows and recording evaluations.
- `policy_pending.go` mixes schema/digest/identity rules with compensation SQL,
  lifecycle/freshness rules and routing.
- `policy_command_store.go` builds command identities, serializes documents and
  writes SQL in the same package.
- Approval principals, signature verification, assertion binding and authority
  lookup are mixed in `policy_approval_authority.go`.
- `policy_approval_notice.go` loads snapshot/delta context and builds notification
  policy; notification and approval evidence must retain their exact inputs.

## Safety configuration and vocabulary

`Gateway` is mutable after construction through three `With` setters. Missing
owner, epoch and interlock dependencies silently skip their checks. Production
composition supplies them, but tests routinely omit them. Require explicit
checks in `New(Config)`; keep intentionally unowned simulation configuration
explicit at composition, rather than nil branches inside governance.

`Result`, `intentRow`, `approvalRow`, `approvalResolution` and `commandDocument`
are useful values but live next to SQL. Approval resolution has nine inputs;
internal operations commonly have six to nine. Use named request values.

## Surface and ownership

Production uses the gateway evaluator, pending-intent read, policy document and
digest exports. Approval resolution/signing remain required by the approval
contract and integration tests, although they have no external production caller.
`CapabilityHost` is only used by its own tests; the earlier executor-boundary
plan reports it as unused. It belongs to neither governance nor a live executor.
Remove it without manufacturing another host abstraction.

Policy owns `intents` governance fields, `policy_evaluations` and
`intent_dispatch_counts`. Episodes produce intents. Policy produces immutable
commands/outbox; actions consumes them. Policy may delete only its exact pending
prepared command before publication. Approval lifecycle mutations already go
through `approvalledger`; keep that ownership and transaction.

The existing intent projection joins accepted decisions, episodes and current
Situations. Approval notices read the bound snapshot and trigger delta;
compensation reads a command's tenant. These are read-only handoff projections
in the same snapshot, not foreign lifecycle mutations. Keep their SQL in store
and document the contract rather than create speculative repository services.
