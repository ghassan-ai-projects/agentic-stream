# Approval HTTP integration design

Status: HTTP integration implemented and validated, 2026-10-05.
This makes the existing loopback approval requirement in
[implementation plan M3.3](../../docs/design/IMPLEMENTATION_PLAN.md) concrete.
The CLI approve/deny client is separate follow-up work.

## Production contract

Enable the routes only on a running `serve --spec` pipeline, with both
`AGENTIC_STREAM_APPROVAL_TOKEN` and `AGENTIC_STREAM_APPROVAL_RELAY` configured.
Reject partial configuration before startup. The credential is independent of
subscriber and control credentials. Bind it to the configured relay principal;
never take relay identity, tenant or evaluation time from request JSON.
Existing serve validation requires a loopback listen address. No new external
service, listener, model credential or direct effector access is introduced.

- `GET /v1/approvals/{id}?approver={principal}&approved={true|false}`:
  authenticate `Authorization: Bearer <approval-token>`, look up the request in
  the pipeline's tenant, require distinct active relay/approver principals and
  tenant/entity/risk authority, and return its immutable JSON plus base64-encoded
  exact signing bytes. Terminal requests return conflict.
- `POST /v1/approvals/{id}`: use the same relay authentication. Accept only
  `approver_id`, explicit boolean `approved`, base64 Ed25519 `signature` and a
  nonempty `reason`. Limit the body to 16 KiB and reject unknown fields or extra
  JSON documents. Resolve using the trusted pipeline clock and tenant.
- Return 401 for relay authentication failure, 403 for principal/signature
  refusal, 404 for both absent and foreign-tenant identities, 409 when a request
  cannot be signed, and 503 for infrastructure/fencing errors. Do not expose SQL
  or internal errors. Responses use `Cache-Control: no-store`.
- Existing resolved-request handling remains idempotent: resolution retries
  return the terminal disposition without another command. Command delivery is
  asynchronous; a successful response means governance was committed, not that
  an external effect completed. Delivery evidence remains in durable events.

Principal, key and role provisioning uses existing durable records; this change
will not create an unauthenticated provisioning endpoint or install credentials.
A relay deployment remains responsible for presenting evidence to the human,
protecting its token and obtaining the approver signature.

## Deliberate policy changes

The canonical assertion will include the `approved` boolean alongside the
existing durable identity, digests, expiry, nonce and principals. Both approval
and denial require a valid approver signature and role authority. This changes
signing bytes: clients must fetch and sign the new presentation, rather than
reuse older signatures. The submitted reason is relay-supplied audit metadata;
it is not part of the signed assertion in this proposal.

A bad fresh submission must not deny or consume a pending request. Return an
authorization error and roll back. Preserve resolved/stale/expiry precedence
and full re-evaluation before approved command creation. Lifecycle writes remain
with `approvalledger` on the caller's original transaction.

HTTP uses transport callbacks bound at composition, preserving its existing
foundation dependency layer. The public policy facade stays thin. Store owns tenant-scoped SQL; app owns
presentation/authorization; domain owns canonical signing; runtime owns the
transaction, tenant, clock and fences; API owns bounded parsing and credential
binding. Runtime maintenance will drain committed outbox work independently of
new sensor events, including after restart. Dispatch retains existing leases,
interlocks, ownership checks and effect idempotency.

## Test and review plan

Add tests in the same change as implementation. The entrypoint does not exist
yet, so expectation tests and code will be developed together against the real
storage migrations and existing policy fixtures.

1. Through the mounted HTTP handler, sign presented bytes and resolve both
   approval and denial. Check durable status, command/outbox, audit and events.
2. Refuse missing/wrong relay tokens, foreign tenants, inactive or identical
   principals, absent authority, invalid signatures and flipped decisions.
   Verify rejected fresh submissions leave approvals pending and create no work.
3. Validate explicit decision, signature length, unknown fields, excessive body,
   malformed/trailing JSON, methods, absent IDs and terminal presentations.
4. Exercise repeat/concurrent submissions and rollback on notification/audit
   failure. Verify at most one command/outbox and durable terminal state.
5. Exercise stale/expired requests, interlock/health refusal, killed epoch,
   ownership loss and clock tampering. Verify every path prevents command creation
   when required, including after a valid signature.
6. Verify production composition mounts routes only with a configured pipeline
   and credential; maintenance delivers committed commands without new input.
7. Measure coverage for policy layers before/after, add meaningful missing-path
   regressions, run lint autofix, focused race tests, architecture/doc checks,
   full CI and production reachability. Do not lower existing quality thresholds.

This change is security-sensitive: it exposes previously test-only authorization
logic and changes the signed decision contract. It must pass the above tests
before being presented as a completed capability.

## Validation

Full CI passes with pinned protoc 35.1. Lint autofix reports zero issues.
Uncached race tests pass for policy, API, runtime, CLI and architecture gates.
Policy coverage: facade 100%, app 82.9% (previously 69.5%), domain 94.3%,
store 82.0%. Production deadcode analysis reports no unreachable policy
functions after removing the obsolete public signing facade.

Tests cover approval/denial and repeated replies; concurrent replies; signatures
and flipped decisions; scoped reads; active/distinct principals and authority;
input limits; stale/expired/health/interlock/epoch/owner refusals; publication
rollback; original clock/tenant binding; and committed outbox delivery without
new input. CLI approval commands and deployment qualification remain open.
