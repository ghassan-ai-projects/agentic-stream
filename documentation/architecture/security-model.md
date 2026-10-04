# Security model

Agentic Stream assumes that event payloads, model output, tool descriptions,
and worker output can be untrusted. The system therefore separates data from
authority and validates again at each boundary where authority increases.

## Trust boundaries

Where does untrusted data enter?

```mermaid
flowchart LR
    E["Untrusted events"] --> V["Validation"]
    V --> S["Stream state"]
    S --> B["Scoped snapshot"]
```

Text equivalent: external data passes evidence validation before durable stream
state; episodes receive a scoped snapshot. Source:
[event log](../../internal/eventlog/) and [evidence boundary](../../internal/evidence/).

Where can a proposal gain execution authority?

```mermaid
flowchart LR
    W["Worker proposal"] --> V["Validation and policy"]
    V --> C["Command"]
    C -->|if permitted| E["Effector"]
```

Text equivalent: worker output is still an untrusted proposal. Runtime validation,
policy, and final current-authority checks govern its path to an effector.
Source: [decisions](../../internal/decisions/), [policy](../../internal/policy/),
and [actions](../../internal/actions/).

Production effect credentials remain outside the model/worker request and scoped
evidence tools. The proposal path carries data; it cannot mint authority.

## Security rules

- Raw events are evidence, never executable instructions.
- A model can read scoped evidence and propose typed Intents, but cannot call
  an effector or access production credentials.
- Every Decision and Intent is checked for schema, identity, snapshot binding,
  digest, expiration, allowed vocabulary, and risk.
- Policy revalidates against current durable state immediately before creating
  or dispatching a Command.
- Interlocks and the current policy epoch are checked again at the concrete
  action boundary.
- Unknown external outcomes stop automatic retry and require reconciliation.
- Replay has no credentials or production effectors by construction.
- Subscriber and control surfaces are authenticated; `serve` refuses every non-loopback
  listen address. Remote access requires an authenticated proxy to loopback.

## Secret handling

The native provider reads `AGENTIC_STREAM_MODEL_API_KEY` from the process
environment. Subscriber and control tokens are also environment-provided. Do
not put secrets in JSONL traces, SituationSpecs, logs, command arguments,
worker requests, or committed files. Rotate them through the deployment secret
manager and restrict process/database/socket permissions.

## Failure posture

Security validation is fail-closed. Malformed input is quarantined or rejected;
expired capability tokens and stale fences are refused; missing interlock
readiness blocks dispatch; a worker that emits an invalid or late Decision does
not get a retry path into governance.

## Threat model boundary

The runtime protects its semantic authority boundaries. It does not make an
arbitrary host safe: operators still own OS patching, filesystem permissions,
network policy, TLS termination, secret storage, database backup, and external
effector correctness. Read the [deployment hardening checklist](../operations/security-hardening.md)
before exposing a process beyond a local development machine.

## Source evidence

- Root policy: [`SECURITY.md`](../../SECURITY.md)
- Worker/evidence tests: [`internal/worker/`](../../internal/worker/), [`internal/evidence/`](../../internal/evidence/)
- Decision and policy tests: [`internal/decisions/`](../../internal/decisions/), [`internal/policy/`](../../internal/policy/)
- Action tests: [`internal/actions/`](../../internal/actions/)

## Next reads

- [Product invariants](invariants.md)
- [Security hardening](../operations/security-hardening.md)
- [Worker boundary](worker-boundary.md)
