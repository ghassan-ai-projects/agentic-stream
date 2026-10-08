# Quality and Modularity Bar

Every Go change must keep the repository at this bar. Each rule is enforced by a
named check in `make ci-check`, so the bar is reviewable and cannot drift into
opinion. Tightening a threshold is welcome; loosening one is a reviewed decision
recorded in the same change, never a way to get a diff green.

| ID | Rule | Enforced by |
| --- | --- | --- |
| Q1 | `golangci-lint` reports 0 issues across the whole tree, with no "new issues only" baseline. | `make lint-ci` (`.golangci.yml`) |
| Q2 | Production functions stay short and flat: cognitive complexity ≤ 15, cyclomatic complexity ≤ 20, nested-`if` complexity ≤ 3, at most 15 body lines (comments excluded) and 15 statements. These are the mechanical floor for Q7, not its target. | `gocognit`, `gocyclo`, `nestif`, `funlen` in `.golangci.yml` |
| Q3 | Production Go files stay under 300 lines. Generated files are exempt. | `TestProductionFileSize` (`architecture_test.go`) |
| Q4 | Every package with statements has its own tests and at least 60% statement coverage, measured with `-short`. Generated protobuf stubs are exempt. | `make coverage-check` (`scripts/check-coverage.py`) |
| Q5 | Package imports follow the declared layering. Foundation packages import no domain package; cognition and episodes never import policy or actions; replay never imports actions or the runtime; nothing under `internal/` imports `cmd/`. | `TestPackageLayering` (`architecture_test.go`) |
| Q6 | The existing gates stay green: proto, tidy, build, vet, race tests, deadcode (no production function reachable only from tests), vulncheck, docs. | `make ci-check` |
| Q7 | Functions read top-down as intent, following the clean-function rules below. | Code review, checked against [review-checklist.md](review-checklist.md) |

## Q7: clean functions

1. **A function name states its intent.** Name the outcome, not the mechanism:
   `claimEpisode`, `checkIntentAuthority`, `skipUnadmittable`, not `process`,
   `handle`, `doStep2`, or `helper`. A boolean function reads as a question
   (`leaseExpired`, `grants`). If you need "and" in the name, it does two things.
2. **A function is short and does one thing.** Its body is at most 15 lines.
   "One thing" means you cannot
   extract another function from it whose name is not just a restatement of
   its code. Error handling for that one thing belongs inside it.
3. **A function stays at one level of abstraction.** Do not mix orchestration
   (`claim`, `execute`, `conclude`) with mechanics (SQL text, JSON field
   access, byte arithmetic, loop bookkeeping) in the same body. When a body
   mixes them, push the mechanics down into a named step.
4. **Public, top-level functions read like a small domain-specific language.**
   An exported entry point is a short sequence of domain verbs over domain
   nouns, so a reviewer can read the policy without reading the mechanics:

   ```go
   func (r *Runner) RunOnce(ctx context.Context, tenantID string) (bool, error) {
       claim, err := r.claimEpisode(ctx, tenantID)
       // ...
       outcome, executionErr := r.executeClaim(ctx, claim)
       return true, r.recordExecution(persistCtx, claim, outcome, executionErr, deadlineExceeded)
   }
   ```

5. **Each function calls functions one level below it (the stepdown rule).**
   Read a file top to bottom: the entry point first, then the steps it calls,
   then their steps, until the remaining operations are small and concrete
   (a query, a field check, an append). Place a callee below its first caller.

These rules never justify a behavior change, and they do not ask for
abstraction layers, interfaces, or packages without a current need (see
[go-style.md](go-style.md)). Extracting a named step is the default tool;
introducing a type is warranted only when several steps share state.

## Rules for meeting the bar

- Refactors made to meet Q2, Q3, and Q7 must not change behavior. The golden replay
  and predictive-maintenance suites are the proof, so run them unchanged.
- Split by responsibility: extract a named step or a type that owns one concern.
  Do not split a function mechanically into `partA` and `partB`.
- Do not use `//nolint` for `gocognit`, `gocyclo`, `nestif`, or `funlen`. Fix
  the code instead. Any other `//nolint` names its linter and gives a reason
  that is true of the code. `//nolint:wrapcheck` is allowed only for a gated
  facade delegation or a protocol error (AGENTS.md, Go Standards); otherwise
  wrap the error.
- Do not delete or weaken tests, and do not exclude packages, to meet Q4.
- A new package edge must follow the data flow in
  [architecture.md](architecture.md). Add it to the allowlist in
  `architecture_test.go` in the same change, with the reason in review.

## Architecture ownership and directed flow

Q5 is supplemented by [architecture-bar.md](architecture-bar.md), A1–A7.
New module boundaries require explicit business ownership, lower-layer imports,
and durable mutation ownership; passing the import allowlist alone is insufficient.
