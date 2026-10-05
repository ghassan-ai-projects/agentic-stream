# Device refactor validation and review

Scope: finish D6 and D7 on `general-improvements-1`. The unrelated
`docs/audits/2026-10-05-architecture-maintainability/` work is excluded.

## Evidence

- Device and architecture tests pass. An injected wire → app import fails
  `TestImportsOnlyPointToLowerArchitectureLayers` at the expected upward edge;
  the probe is removed and the full root suite passes again.
- Whole-tree lint reports zero issues with unchanged thresholds.
- `go test -race -count=1 -shuffle=on ./...` passes, including replay,
  actions, authority, runtime, CLI, device and worker integration tests.
- Full `make ci-check` passes with the cached pinned protoc 35.1: protocol
  regeneration, tidy, build, vet, lint, short race coverage and documentation.
  The host default protoc 36.0 cannot satisfy the protocol check; no repository
  pin is changed. `deadcode` and `govulncheck` are absent and the Makefile
  explicitly skipped those optional local checks.
- `git diff --check` passes. `pre-commit` is not installed.

The new regressions prove original state-document/digest preservation,
optional receipt/result code handling, schema rejection before typed parsing,
caller isolation of cached replies, final authorization precedence and
handshake/refresh/resolution fencing after epoch kill. Tests also prove a
failed state refresh prevents delivery and a refused resolution leaves its
durable reconciliation required.

Device coverage from the CI gate:

| Layer | Statement coverage |
| --- | --- |
| Facade | 93.3% |
| App | 83.0% |
| Domain | 84.0% |
| Transport | 87.7% |
| Wire | 90.4% |

## Self-review

The device facade remains thin. Domain matching and verification use typed
values. Wire validates before parsing and retains the full documents used by
digests, sealed evidence and provider results. Session operations retain
their locks, send/receive order, post-send claim assertions, cache timing,
partial-send classification, safe-stop latch and reconciliation fallback.

The removed runtime preflights performed a separate admission read before
authority's mutation transaction repeated the check. Authority remains the
owner of that transactional check; D7 changes the error context and precedence
of its input/owner validation, as recorded in the plan. Final authorization
still occurs once before effector work and preserves `errors.Is` causes.

Request values group the prepared command, claim and identity, and the
priority command's lifecycle facts. They add no service or persistence layer.
Architecture checks enforce wire → domain, transport → wire, and app → its
adapters; reasoning and replay remain unable to reach effect implementations.

## Assessment

These are maintainability assessments, not physical-device qualification.

| Dimension | Score / 10 | Evidence or remaining weakness |
| --- | --- | --- |
| Layering | 9 | Thin facade, pure domain, dedicated wire/transport, enforced imports. |
| Domain rules | 9 | Matching, bounded materialization and deterministic verification are pure. |
| Fail-closed safety | 9 | Required authority, transactional admission and unknown-outcome tests. |
| Ubiquitous language | 8 | Core types agree; some historical serial/barrier wording remains. |
| Tests | 9 | Race, replay, socket, fencing and regression coverage; no physical HIL proof. |
| Data encapsulation | 7 | Catalog maps and original documents remain mutable internal values. |
| Type safety | 8 | Protocol decisions are typed; catalog parameters and evidence handoffs remain JSON maps. |
| Simplicity | 8 | Named requests reduce long signatures; the safety protocol still has many failure stages. |

## Remaining work outside this refactor

1. Design production callers for priority safe stop and operator reconciliation
   clearing. The retained session mechanisms and tests do not complete that
   integration or qualify physical hardware.
2. Consider immutable catalog data to replace per-dispatch digest validation,
   if measurements justify the change.
3. Replace remaining historical serial/barrier wording when its callers and
   diagnostics can be updated together; evidence contracts still use maps at
   module boundaries.
