# Policy validation and review

Scope: P0–P3 on `general-improvements-1`; authority/device patterns compared
against their code and records. Unrelated architecture audit files are excluded.

## Completed round evidence

| Round | Commit / state | Evidence |
| --- | --- | --- |
| P0 | `c9bc465` | Baseline policy/root tests, findings, language, target design and plan. |
| P1 | `0c6feaf` | Pure domain regression tests, policy/root tests, lint. |
| P2 | `4cea525` | Facade/app/store and runtime tests; original transaction/rollback and epoch refusal; full lint; every layer above 60%. |
| P3 | Complete | Typed original-document tests, pure routing/principal/signature rules, named attempts/publication/outcome values; focused policy/runtime/root tests, full CI and uncached race suite pass. |

Injected violations were rejected: domain → app import, domain wall-clock read,
app SQL/database import, store approval lifecycle mutation, and reasoning →
policy domain import. Every probe was removed; the root suite passed again.
Lint autofix (`golangci-lint run --fix ./...`) reports zero issues. Thresholds and
lint policy are unchanged.

## Final validation

- `PATH=/private/tmp/agentic-stream-protoc-35.1/bin:$PATH make ci-check` passes:
  pinned protocol regeneration, tidy, build, vet, lint, short race coverage and
  documentation checks. No repository pins or thresholds were changed.
- `go test -race -count=1 -shuffle=on ./...` passes, including replay, actions,
  authority, runtime, worker and CLI integration suites.
- Final focused policy/root tests and documentation checks pass after moving
  package guides, renaming/strengthening calibration tests and splitting facade
  configuration. Lint autofix reports zero issues.
- `git diff --check` passes. `deadcode` and `govulncheck` are unavailable and were
  explicitly skipped by the Makefile; those optional checks are unverified.

| Policy layer | Statement coverage from CI |
| --- | --- |
| Facade | 95.5% |
| App | 69.5% |
| Domain | 94.4% |
| Store | 82.4% |

## Self-review

The facade contains configuration, aliases and delegation. `policy.go` itself
contains only the service type and two delegating operations; `config.go` owns
constructor validation. Durable guides live at the package root. It joins the caller's
transaction without acquiring transaction ownership. App never imports SQL or
storage. Domain owns risk, freshness, compensation, principal, signature and
approval disposition rules. Store owns SQL, record decoding for opaque trigger
context, and calls to lifecycle/notification owners on the exact transaction.

Decision/intent schemas, digest inputs, ID call order, error precedence,
owner/epoch checks, notification identities, approval single-use assertion
binding, command/outbox idempotency and dispatch-counter admission are retained.
The accepted constructor changes and removed dead capability host are recorded
in PLAN.md. Existing no-control simulation composition remains explicit.

Store tests use real migrations to prove insert conflicts cannot rewrite work,
rate counters stop at the limit, exact deletion binds command identity, lifecycle
reads agree with ledger mutations, closed transactions retain their error causes,
and caller rollback discards publication and audit together. App regressions
retain stale-before-expiry-before-authorization and approved-human re-evaluation.
Pure tests bind typed views to the original digest document and opaque parameters.
Tests were added with each extraction round before accepting its commit.

## Assessment

Maintainability assessments; these are not deployment qualification scores.

| Dimension | Score / 10 | Evidence / limit |
| --- | --- | --- |
| Layering | 9 | Thin facade, pure domain, I/O store, enforced downward imports. |
| Domain rules | 9 | Risk, lifecycle/health/expiry, compensation, principal and signature decisions are pure. |
| Fail-closed safety | 8 | Required checks and rollback/epoch refusals; callback semantics are composition's responsibility. |
| Ubiquitous language | 9 | Named intents, approvals, assertions, preparation, outcomes and lifecycle values. |
| Tests | 8 | Focused regression and transaction tests; app coverage leaves some infrastructure failure combinations open. |
| Data encapsulation | 7 | Original documents and effector parameters remain mutable internal maps. |
| Type safety | 8 | Closed governance views and named requests; opaque payloads/deltas remain JSON. |
| Simplicity | 8 | One store and transaction, short domain steps; approval governance has several necessary stages. |

## Remaining limits

1. Read-only joins still project foreign decision/episode/Situation/principal and
   command handoffs. Owner-provided read ports would require a multi-module change.
2. Original document maps and opaque parameters are mutable inside the module;
   stronger immutable values would improve encapsulation.
3. Production approval entrypoints and deployment qualification remain separate
   work. This refactor retains the existing approval contract and integrations.
