# Who owns state and authority?

A useful diagnosis and permission to change the world are separate results.
Agentic Stream gives each kind of state a clear owner so a worker response,
restart, or delayed provider reply cannot silently become new authority.

## Three planes, three responsibilities

| Plane | Owns | Can produce | Motor example |
| --- | --- | --- | --- |
| Stream | Evidence interpretation and Situation state | Published versions | Persistent readings move the condition into `warning` |
| Cognition | Eligibility, queueing, admission, and bounded reasoning | Validated Decisions and proposed Intents | Diagnose a useful change within a fixed budget |
| Policy and action | Permission, dispatch, and effect evidence | Commands and outcomes | Permit a ticket request, then record what the provider did |

The planes connect through durable records. A Decision does not directly edit
Situation facts, and a Command's success does not itself prove that the
condition has improved. New observations must establish that change through
the stream rules. Source: [architecture](../architecture/overview.md)
and [ownership boundaries](../architecture/modules.md).

## An entity and an action target can differ

An **entity** is what the evidence describes. A **target** identifies where a
permitted effect would go. A motor's condition might lead to a ticket-provider
request or, in another qualified integration, a device operation. The model's
proposal cannot turn an arbitrary target string into permission.

A **capability** describes an allowed operation and its constraints. A device
**binding** connects a governed Command to the concrete operation and target.
A device **session** owns communication with that device. **Target authority**
records which owner may control it, with boot, reconciliation, and safety
barriers. Policy approval and device authority are separate requirements.

For physical effects, the concrete adapter, runtime readiness checks, and
authority records must agree. Having adapter code is not proof that a particular
piece of hardware is qualified. Source: [device adapters](../../internal/device/doc.go),
[target authority](../../internal/authority/doc.go), and
[limitations](../overview/limitations.md).

## Recovery needs identities, leases, and fences

| Mechanism | Purpose | What it does not establish |
| --- | --- | --- |
| Durable ledger | Record lifecycle changes and their reasons | That an external effect happened |
| Inbox/outbox | Persist receipt or delivery work across restarts | Exactly-once external delivery by itself |
| Lease | Give a worker a temporary claim to work | Permanent ownership after expiry |
| Fence | Reject output from an older ownership or attempt generation | Cancel an already accepted provider effect |
| Policy epoch | Identify the current generation of operational permission | Make an earlier approval valid forever |
| Runtime owner | Establish the current runtime's ownership for controlled work | Grant a model device credentials |

These mechanisms operate at different boundaries. An episode fence protects
reasoning acceptance; dispatch and device ownership have their own checks.
A lost provider reply requires outcome evidence and reconciliation, even when
the queue and lease records are intact. Source: [durability](../architecture/durability.md),
[episode lifecycle](../../internal/episodeledger/lifecycle.go),
[runtime control](../../internal/control/doc.go), and
[recovery](../operations/recovery.md).

## A watch is a bounded follow-up condition

An approved Command can install a **watch**: a scoped condition that observes
later evidence, expires, and has a maximum number of firings. Its purpose is
to preserve a limited follow-up question, such as watching for a declared
change after an action.

The watch owner records matching events and spends the remaining allowance;
it does not call policy, dispatch, or reasoning itself. A firing is a durable
match record, not permission to execute another effect or proof of a model
call. Source: [watch ownership](../../internal/watch/doc.go)
and [firing rules](../../internal/watch/fire.go).

## Explainability and qualification answer different questions

**Explainability** asks why the runtime published, waited, reasoned, refused,
or dispatched. Durable evaluations, episode records, policy results, and
outcomes provide that history. Notifications expose committed lifecycle events;
telemetry helps observe execution but does not replace those records.

**Qualification** asks whether the configuration and integration are suitable
for the intended operating environment. Calibration activation and report-only
shadow comparisons are separate evidence mechanisms. Passing a synthetic motor
test does not qualify a physical installation.
Source: [observability](../design/observability.md),
[qualification owner](../../internal/qualification/doc.go), and
[current release posture](../overview/status.md).

## Next reads

- [Why these design choices](design-choices.md)
- [From proposal to effect](safe-actions.md)
- [Durability and recovery](../architecture/durability.md)
