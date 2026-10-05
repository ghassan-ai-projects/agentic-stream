# Human approval integration follow-up

Status: HTTP integration implemented on 2026-10-05; validated; see the linked design record.
The original findings below were recorded against `7f1cf94` and are historical.
The production path is defined in [APPROVAL_HTTP_DESIGN.md](APPROVAL_HTTP_DESIGN.md).
CLI client and deployment qualification remain separate follow-up work.

## Original problem and evidence

Production evaluates intents and can persist an approval request and its
notification. It does not provide an entrypoint that accepts a human decision
and invokes `Service.ResolveApproval`. The HTTP runtime handler exposes health,
events, metrics and drain/kill controls; the CLI and runtime do not call the
resolution API. Tests call it directly.

The implementation exists, but its production integration is incomplete. There
is no evidence that approval support was deliberately retired. The design still
requires human approval for default R2 actions and revalidation before command
creation. A pending approval cannot be completed through the current shipped
application surface. Calibration can permit some consequential automation;
that does not supply a human approval completion path.

Supporting sources:

- [Approval design, section 13.3](../../docs/design/TECHNICAL_DESIGN.md).
- [Production HTTP composition](../api/events.go).
- [Policy composition](../runtime/pipeline_composition.go).
- [Operator surface roadmap gap](../../docs/research/next-level-2026-10/README.md).
- [Signed resolution integration tests](internal/app/policy_test.go).
- [Stale approval precedence tests](internal/app/approval_order_test.go).

## Original reachability inventory

On the current host with default build tags, production analysis reports the
following 33 functions unreachable. Test-inclusive analysis reports zero
unreachable policy functions. The count is a chain below two disconnected public
entrypoints, not 33 independent obsolete features. Refresh this inventory before
implementation; reachability results depend on entrypoints and build settings.

```sh
deadcode -filter='/internal/policy($|/)' ./...
deadcode -test -filter='/internal/policy($|/)' ./...
```

| File | Functions unreachable from production |
| --- | --- |
| `api.go` | `ApprovalAssertionSigningBytes` |
| `policy.go` | `Service.ResolveApproval` |
| `internal/app/approval.go` | `Service.ResolveApproval`, `Service.resolvePendingApproval`, `Service.withdrawStaleApproval`, `Service.expireApproval`, `Service.resolveAuthorizedApproval`, `Service.denyUnauthorizedApproval`, `recordApprovalResolution` |
| `internal/app/approval_authority.go` | `Service.authorizeApproval`, `loadApprovalPrincipals`, `verifyApprovalAssertion`, `approvalSigningBytes` |
| `internal/domain/definition.go` | `ApprovalAssertionSigningBytes`, `canonicalApprovalAssertion` |
| `internal/domain/routing.go` | `ApprovalDisposition`, `DistinctPrincipals`, `VerifyAssertion`, `ActiveRelay`, `ValidApproverKey`, `AuthorizedApprover` |
| `internal/domain/rules.go` | `ApprovalDecision` |
| `internal/store/approval_lifecycle.go` | `Tx.ResolveApproval`, `Tx.WithdrawApproval`, `Tx.ExpireApproval`, `Tx.BindAssertion` |
| `internal/store/approval_lookup.go` | `Tx.AssertionBinding` |
| `internal/store/approval_reads.go` | `Tx.LoadApproval` |
| `internal/store/notifications.go` | `Tx.AppendApprovalWithdrawn` |
| `internal/store/principals.go` | `Tx.ApprovalEntity`, `Tx.RelayActivity`, `Tx.ApproverKey`, `Tx.ApprovalAuthority` |

## Integration checklist

1. Agree the operator/relay contract and document it before implementation:
   HTTP or CLI entrypoint, tenant-scoped request lookup, immutable assertion
   presentation, approve/deny input, authentication, authorization, errors and
   retry semantics. No route or protocol shape is selected by this record.
2. Define principal provisioning and identity binding. Bind authenticated
   identities to the submitted approver/relay, preserve their required separation,
   and check tenant/entity/risk scope. The current app authorizes signed approval
   grants; its decline branch does not call that authorization helper. Specify
   and test who may deny before exposing either operation.
3. Compose a configured policy service with real runtime ownership and epoch
   fences. Accept a decision in a caller-owned transaction and invoke the public
   facade. Obtain evaluation time from the trusted runtime clock rather than
   allowing clients to choose it. Do not write approval/intent status in handlers.
4. Preserve canonical durable assertion reconstruction, signature verification,
   nonce/digest binding and lifecycle ownership through `approvalledger`.
   Re-evaluate approved intents through all policy gates before command creation.
5. Deliver outcomes through the existing durable notification mechanism and
   expose actionable operator errors. Define retry behavior for already-resolved
   requests and concurrent decisions without duplicating commands or effects.
6. Add end-to-end tests through the chosen production entrypoint. Update public
   capability/limitation docs only after that path is implemented and verified.

## Acceptance checks

- A production entrypoint can retrieve a tenant-scoped pending request and submit
  an authenticated human decision; another tenant cannot inspect or resolve it.
- Valid approval yields at most one command/outbox handoff. Retries and concurrent
  decisions preserve the terminal result and do not duplicate effects.
- Invalid signatures, changed assertion fields, reused assertions, inactive
  principals and insufficient authority cannot produce a command. Denial access
  is explicitly enforced and tested.
- Stale Situation, expiry, incomplete evidence, interlock refusal and killed epoch
  continue to prevent command creation after a valid signature. Preserve existing
  error precedence and test the time-of-check/time-of-use boundary.
- Ownership loss or transaction failure rolls back lifecycle, intent, command,
  notification and audit writes together.
- Existing policy regression tests remain; add production-path tests rather than
  replacing direct use-case coverage with transport-only assertions.
- Run lint autofix, focused race tests, architecture/documentation checks and the
  full CI gate. Re-run production reachability and explain any remaining signing
  helper that is intentionally client-facing rather than called by runtime main.

The HTTP integration now implements this checklist. The old public signing
helper has been removed; clients sign bytes returned by the presentation route.
This record does not claim CLI completion or production qualification.
