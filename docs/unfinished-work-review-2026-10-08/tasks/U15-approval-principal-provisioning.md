# U15 — Approval principal provisioning

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U13

## Finding

Human approval (`serve --spec` with `/v1/approvals/{id}`) checks that the relay
is an active principal, that the approver has an Ed25519 key, and that a role
grants authority for the tenant, entity and risk
(`internal/policy/internal/store/principals.go`). No production code writes
`principals`, `roles`, `principal_roles` or `approval_authorities`. The HTTP
reference says they "must be provisioned by the deployment. There is no public
provisioning endpoint", so in practice that means hand-written SQL against
undocumented tables.

The default intent policy is `approval`, and R2 intents always need approval
because nothing writes `calibration_artifacts` (see P06). The MVP's governed
maintenance-ticket path therefore cannot complete in a fresh deployment.

## Decision and reasoning

Complete it with one declarative, idempotent command instead of many imperative
ones:

```text
agentic-stream principals apply --db <db> --file principals.yaml [--dry-run]
agentic-stream principals list  --db <db>
```

The file lists principals (id, kind relay/approver, public key, status), roles,
role grants and authorities (tenant, entity pattern, max risk). `apply`
validates the whole file, then makes the tables match it in one owner-fenced
transaction (U13). Removal is a `status: disabled` row, never a delete, so the
audit history of past approvals keeps its principals.

Reasons for this shape:

- The data is configuration that belongs in reviewed files, like specs.
- Idempotent apply is safe to rerun from deployment automation.
- Keys are public keys only, so no secret is handled.

The SQL belongs to `policy` (it owns these tables); the CLI calls a policy
facade operation.

## Done when

- Fresh database + `principals apply` + one approval round trip over HTTP passes
  in an end-to-end test using the predictive-maintenance spec.
- The HTTP reference replaces "must be provisioned by the deployment" with the
  command, and the file format is documented with an example under
  `examples/`.
