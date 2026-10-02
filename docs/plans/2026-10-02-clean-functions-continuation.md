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
6. Artifact verification stages and final whole-repository validation.

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
