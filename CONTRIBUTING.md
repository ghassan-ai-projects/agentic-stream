# Contributing

Thanks for improving Agentic Stream. The goal is simple: keep the Situation Runtime correct, deterministic, and honest about what it knows.

## Before You Start

1. Read [AGENTS.md](AGENTS.md).
2. Read the [public documentation](documentation/README.md), especially the
  [architecture](documentation/architecture/overview.md), [testing reference](documentation/reference/testing.md), [quality gates](documentation/governance/quality.md), and [limitations](documentation/overview/limitations.md).
3. Check existing issues and pull requests for related work.
4. Keep changes focused. Avoid mixing documentation, tooling, and behavior changes unless they must ship together.
5. Run `git status --short` before editing so you do not overwrite someone else's work.

## Development Workflow

Use this loop for non-trivial changes:

1. Think: clarify the goal, constraints, affected files, and risks.
2. Plan: choose the simplest correct path and the checks that will validate it.
3. Review plan: get human review before broad, ambiguous, security-sensitive, destructive, architectural, or dependency-changing work.
4. Write tests: for production-code behavior changes, write the proving test before implementation when feasible.
5. Implement: keep the diff cohesive and reviewable.
6. Validate: prove the change meets the Definition of Done.
7. Handoff: summarize changes, checks, skipped checks, and remaining risk.

## Definition of Done

A change is done when:

- The requested scope is complete and the diff avoids unrelated refactors.
- The implementation is the simplest correct change that fits current requirements.
- Production-code behavior changes include meaningful tests, and modified packages do not show 0% coverage.
- When feasible, production-code behavior changes start with a failing or expectation-setting test.
- `make ci-check` passes, unless the change is documentation-only and a narrower check is clearly sufficient.
- Documentation changes pass `make docs-check` and `git diff --check`; command,
  API, contract, or status claims also run the relevant code/tests.
- `git diff --check` passes, and `pre-commit run --all-files` passes when `pre-commit` is installed.
- Documentation is updated when behavior, commands, setup, or agent expectations change.
- Secrets are not added, security-sensitive changes are called out, and dependency or workflow permission changes receive extra review.
- Makefile targets, CI, hooks, and documented commands agree.
- The PR or handoff lists checks run, skipped checks with reasons, and any residual risk.

```bash
make help
make ci-check
make docs-check
```

For code changes, include tests in every modified package. For documentation-only changes, run the narrowest useful checks and explain anything skipped.

## Pull Request Expectations

Every PR should include:

- A short summary of what changed and why.
- Tests or checks run.
- Whether test-first was used for behavior changes, and if not, why not.
- Any skipped checks and the reason.
- Any new dependency, generated file, security implication, or migration step.
- Any meaningful complexity added and why it was necessary.
- Updates to [AGENTS.md](AGENTS.md) when the change affects future agent behavior.

## Commit Style

Use Conventional Commits:

- `feat:` for user-visible features.
- `fix:` for bug fixes.
- `docs:` for documentation.
- `test:` for tests.
- `ci:` for CI changes.
- `build:` for build tooling.
- `chore:` for maintenance.

## Agent Contributions

Agent-authored changes are welcome, but the handoff must be reviewable:

- Keep diffs small enough for a human to audit.
- Prefer existing patterns over new abstractions.
- Default to the simplest correct implementation.
- Do not commit secrets, machine-local paths, or private customer data.
- Include command output summaries rather than pasting noisy logs.
- For documentation changes, identify the audience, source-of-truth files,
  implemented/partial/deferred status, and the next-read path.
