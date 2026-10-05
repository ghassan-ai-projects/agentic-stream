# Completion plan

This continues the user's partial app extraction. The earlier three-layer
implementation is superseded; a caller-owned transaction is compatible with
private app use cases through an opaque store unit of work.

| Round | Scope | Required proof | Status |
| --- | --- | --- | --- |
| C0 | Survey and corrected plan, preserve existing worktree | Read production surface, references, ownership and tests | Complete |
| C1 | Opaque store transaction; pure domain contracts/rules; private application use cases | Episode tests, transaction rollback tests, root architecture gates, lint | Pending |
| C2 | Validated public service; caller updates; fixture executor extraction; code audit | Facade construction/delegation tests, adapter/conformance/replay/runtime tests, production and test reachability | Pending |
| C3 | Enforcement, injection proofs, guide/map updates, final review | Full CI targets, uncached race suite, coverage, diff check | Pending |

Commit this plan separately before additional code changes. Include the user's
existing staged migration only in the implementation round, never in C0.
Test-first configuration and transaction regressions where feasible; relocating
existing behavior tests alongside the rule is the proof for pure moves.

## Preserve

Error precedence and sentinel wrapping; identity/digest inputs; canonical
request and Decision documents; clock reads; serial claim transactions; stale
rebind bound of three; retry bound of three; epoch checks before dispatch and
inside post-execution persistence; detached five-second persistence budget;
supersession cancellation; shadow effect isolation; original atomic ledger,
reservation and decision handoffs. Do not change golden fixtures or thresholds.

## Deliberate changes

- Replace mutable public Assembler/Runner constructors and setters with one
  validated Service configuration, updating consumers without compatibility shims.
- Reject missing execution safety dependencies at construction. Assembly-only
  services explicitly refuse RunOnce.
- Move the deterministic fixture executor to its own adapter; it remains
  available for the existing demo route.
- Remove confirmed dead functions and move helpers that only support tests
  into test files. Record each decision in CODE_AUDIT.md.

## Follow-ups

Keep cross-module read projections in the store on the caller transaction.
Replacing them with new read ports across every owning module is a separate
migration; the current change does not grant foreign mutation authority.

## Validation record

The first focused baseline attempt could not access part of the host Go build
cache. Retry with a task-local cache; do not count the failed attempt as proof.
