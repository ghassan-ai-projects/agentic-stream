# Security hardening checklist

This checklist separates runtime controls from deployment controls. A green
unit suite is not a deployment approval.

## Runtime controls to preserve

- Bind `serve` to loopback or place it behind an authenticated proxy.
- Require and rotate `AGENTIC_STREAM_SUBSCRIBER_TOKEN`.
- Set `AGENTIC_STREAM_CONTROL_TOKEN` only through a protected secret source;
  audit drain/kill actions.
- Keep model keys in `AGENTIC_STREAM_MODEL_API_KEY`, never in traces or flags.
- Use private worker/evidence sockets with restrictive permissions.
- Use complete mTLS settings for a certificate-authenticated worker.
- Use 32-byte-or-longer HMAC keys for EvidenceTools.
- Keep SQLite permissions restrictive and backups encrypted/controlled.
- Use idempotent/reconcilable external effectors and monitor unknown outcomes.

## Deployment controls

- Patch and harden the host and Go runtime environment.
- Terminate remote TLS and enforce authentication/rate limits at the proxy.
- Rotate credentials, certificates, HMAC keys, and subscriber/control tokens.
- Restrict outbound model/provider networking to approved endpoints.
- Protect event traces, Situation snapshots, decisions, and database backups by
  classification.
- Test disk-full, WAL recovery, backup restore, process crash, owner takeover,
  worker loss, and provider timeout behavior.
- Run vulnerability checks and review dependency/workflow permission changes.

## Release caveat

Environment release evidence is not complete in the current snapshot. See
[`documentation/governance/release-status.json`](../governance/release-status.json)
and the archived [operations readiness](../../docs/design/OPERATIONS_READINESS.md).

## Reporting

Do not disclose a vulnerability in a public issue. Follow the repository
[security policy](../../SECURITY.md).

## Next reads

- [Security model](../architecture/security-model.md)
- [Recovery](recovery.md)
- [Compatibility](../overview/compatibility.md)
