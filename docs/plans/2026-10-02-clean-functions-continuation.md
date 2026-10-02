# Clean-function continuation

Date: 2026-10-02. Branch: `code-improvments-2`.
Starting HEAD: `57eb8bb`. Status: in progress.

## Bar and method

Continue the merged refactoring by reviewing remaining entry points against Q7.
The root cause is mixed abstraction: filesystem reservation, SQL predicates,
row decoding, and transport responses still appear beside domain orchestration.
Extract concrete owning steps within existing packages, without new interfaces,
dependencies, or changes to contracts or quality thresholds.

Each round follows characterization tests -> extraction -> focused race tests
and lint -> critical diff review -> commit. Preserve errors and their precedence,
clock reads, transactions, locks, ordering, partial results, identities, hashes,
and cleanup ownership. Existing replay and predictive-maintenance tests remain
unchanged. Completion requires the whole Q1-Q6 gate and Q7 review of changed
functions; lint alone does not establish Q7 across every untouched function.

## Rounds

1. Replay database reservation and SQLite contention retry.
2. Runtime/target ownership and reconciliation persistence.
3. Event-log append/read orchestration and row mechanics.
4. HTTP health/control orchestration.
5. Specification parsing and compiler reading order.
6. Artifact verification stages.
7. Aggregate cost reservation, settlement, and final whole-repository validation.

## Baseline

The previous delivery report records a passing gate; current evidence is
collected again. The pre-existing untracked `agentic-stream` executable is
excluded. Pinned protoc and optional analysis tools already exist in temporary
storage. No changes to generated files or domain data are planned.

## Evidence

### Round 1: replay reservation and SQLite retry

Extracted reservation ownership, sidecar inspection, exclusive file creation,
and cancellation-aware backoff. Tests cover file/sidecar/reservation collisions,
dangling symlink precedence, migration cancellation, preservation of existing
files, the six-attempt retry limit, and callback error precedence on cancellation.
Storage race tests pass. Focused lint and diff checks pass after fixing error
comparisons in the new tests. Review confirms reservation cleanup, error strings,
backoff delays, attempt order, and successful-close ownership are unchanged.

Baseline whole-tree lint passed. Restricted baseline CI could not write the Go
module cache and failed local socket tests. Final CI will run with those required
capabilities rather than skipping checks.

### Round 2: durable ownership and reconciliation barriers

Separated lease persistence from owner verification, target assertion/release
from their SQL steps, and barrier opening from state binding. Reused TargetClaim
for the private transaction step rather than introducing another state type.
Tests pin post-recovery lease expiry, identity/boot fences, release audit rollback,
barrier audit rollback, and old-boot rejection after authority loss. Storage race
tests, focused lint, and diff checks pass. Review retained both release clock
reads, final recovery assertion, authority-check precedence, and atomic audits.

### Round 3: event-log admission and reads

Separated transactional batch admission, envelope encoding/insertion, range
selection, row delivery, and row reconstruction by responsibility. Tests pin
whole-batch rollback, validation before duplicate suppression, tenant/partition
filtering before limits, stable position ordering, and connection cleanup when
the consumer stops. Event-log race tests, focused lint, and diff checks pass.
Review confirms the same SQL, duplicate sentinel, JSON/hash inputs, clock read
position, callback errors, and row cleanup.

### Round 4: HTTP health and operator control

Health construction now registers routes without embedding response logic.
Control delivery reads as access check, control application, state response;
control mechanics have their own file. Tests pin configuration/authentication/
method precedence, readiness call counts, problem responses, persistence errors,
and absence of mutations on rejection. API race tests, focused lint, and diff
checks pass. Review preserved exact-token constant-time comparison, status/body
shapes, action-before-state-read order, and readiness detail suppression.

### Round 5: specification parsing and compiler order

Placed compiler entry points before their steps and separated parsing, embedded
schema loading, and normalization into responsibility files. Parsing now names
duplicate-key validation, strict decoding, and identity checks. Tests pin error
precedence through reference/expression validation and compiler reuse after a
failed document. Equivalent YAML/JSON preserves canonical bytes and digest when
CEL text is preserved exactly (a generic numeric conversion changes `1.0` to
`1`, which is different expression source). Spec race tests, focused lint, and
diff checks pass. Review retained loader denial, schema caching, defaults,
normalization order, compile diagnostics, and identity inputs.

### Round 6: artifact verification

Artifact verification now reads as file integrity, JSON syntax, manifest binding,
and ledger verification. Checksum parsing and JSONL record validation each own
their concrete mechanics. Tests pin malformed/duplicate/path-bearing checksum
entries, integrity-before-syntax precedence, manifest version rejection, and
canonical/blank record checks even when checksums are refreshed. Run-artifact
race tests and focused lint pass. Test writes use os.Root rather than adding
security-linter suppressions. Review confirms verification order, line bounds,
error strings, digest inputs, and fail-closed behavior are unchanged.

### Continued review

The six-round checkpoint gate is running. A further Q7 review identified mixed
SQL/orchestration in aggregate cost-control entry points. Extend the same bar
to reservation and settlement, pinning global-before-tenant order, rollback,
repeated settlement, and signed-integer validation. No threshold is lowered.

### Round 7: aggregate cost control

Separated reservation insertion, settlement loading/idempotence, reservation
ledger updates, and scope accounting. A shared existence predicate preserves
mandatory global and optional tenant limits. Reused the existing range-checked
integer conversion instead of adding suppressions. Tests pin global-before-tenant
rejection, rollback after tenant rejection/write failure, identical/conflicting
settlement, kill-switch stability, and validation before transaction access.
Cost-control race tests and focused lint pass. Review retained SQL predicates,
error precedence, caller-owned transactions, amount checks, and update order.

The six-round checkpoint (`2a03c8a`) passed `make ci-check` and the full uncached
race suite with the pinned protoc and local socket access. Final evidence must
include this additional round before declaring completion.
