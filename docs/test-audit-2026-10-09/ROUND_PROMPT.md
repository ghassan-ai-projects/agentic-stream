# Round prompt

The shared instructions given to every round worker, followed by round notes
specific to the modules. Paths to a scratchpad mean any directory outside the
repository.

You are a worker for the test audit in the Go repository at /Users/ghassan/my-projects/agentic-stream (branch `improve-cleanup-tests`). Read first: AGENTS.md, docs/test-audit-2026-10-09/README.md, docs/test-audit-2026-10-09/TEST_BAR.md, docs/test-audit-2026-10-09/WORKER_BRIEF.md, .agents/context/testing.md, docs/test-audit-2026-10-09/modules/architecture.md (round 1's report, as an example of the expected report quality), and each module's README.md / UBIQUITOUS_LANGUAGE.md for your modules. Follow WORKER_BRIEF.md exactly.

Concurrency: other workers are editing OTHER modules in the same worktree right now. Touch only your modules' files (and their report files under docs/test-audit-2026-10-09/modules/). Never run git checkout/stash/reset/commit/add. Do not run `make ci-check` or `go test ./...` over the whole repo; verify with the per-module commands in WORKER_BRIEF.md plus `go build ./...`, `go test -count=1 ./internal/architecture/...` and `make docs-check` (renaming or deleting a test file can break a documentation link: grep documentation/, AGENTS.md and module READMEs for every file you rename or delete, and update live links). If a failure is caused by another module's in-progress edits, note it and move on. Timing numbers are taken while other workers run: compare before/after with `go test -short -race -count=1 -json` on your packages, run back to back.

Scratchpad for temporary files, lint configs and go/ast programs: a scratch directory outside the repository (create it). Baseline numbers are in docs/test-audit-2026-10-09/BASELINE.md; a static investigation of slow tests is at [SLOW_TESTS.md](SLOW_TESTS.md) (unverified; measure first).

Quality expectations, beyond the brief:
- Be a strict auditor. Read every test. Coverage targets: every package ≥ 70%, aim ≥ 80% where the uncovered code is real behavior (error branches enforcing rules, boundary values, rejection paths). Do not add assertion-free tests; do not test generated code.
- Prefer deleting and merging over adding. Collapse repetitive tests into tables. Rename phase/round-numbered files and tests (T3).
- Every top-level test and subtest t.Parallel() unless it truly cannot (then it must use t.Setenv/t.Chdir or be explained in the report).
- Production code: behavior-preserving changes only, each listed in the report with the reason.
- Write one report per module: docs/test-audit-2026-10-09/modules/<module>.md (module = the directory name under internal/, e.g. `kernel`, `executor-native`).

Return a concise summary (under 400 words): per module coverage before → after per package, time before → after, tests removed/renamed/added counts, production files touched, open items, and any check that failed.
