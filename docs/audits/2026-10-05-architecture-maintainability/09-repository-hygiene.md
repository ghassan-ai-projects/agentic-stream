# 9. Clean up the repository and name tests by behavior

## Problem

Some files in the repository should not be there, and some test names describe
a project phase instead of the behavior they check.

## Evidence

- **A 40 MB compiled binary is tracked in git.** `agentic-stream` at the
  repository root is a Mach-O arm64 executable, committed in `9680506`.
  `.gitignore` covers `bin/` but not the root binary that `go build
  ./cmd/agentic-stream` creates. Every clone downloads it, and it is out of
  date as soon as the code changes.
- **Replay writes its database into `testdata`.** `agentic-stream run` defaults
  `--db` to `<trace>.replay.db` (`cmd/agentic-stream/main.go:124`). Running the
  example therefore creates
  `examples/predictive-maintenance/testdata/trace-watch.jsonl.replay.db`. It is
  ignored by git, but it sits in a fixtures folder, and an old database from an
  earlier run can be reused by mistake.
- **Some tests are named after phases:** `p8_mode_control_test.go`,
  `p8_calibration_test.go`, `p8_freshness_test.go`, `p8_shadow_test.go`,
  `p8_latency_test.go`, `p8_graph_version_test.go`, `phase04_test.go`, and
  `parity_p1_test.go`. Production comments also have `// P8:` and `// P4:`
  prefixes (18 comments in 13 files). New contributors do not know the
  P-series plan, so the name does not say what the test protects.
- `cmd/agentic-stream/main.go:155` exposes a `config effective` command that
  only prints "not yet implemented" (see
  [item 7](07-single-runtime-assembly.md)).

## Recommendation

1. Run `git rm --cached agentic-stream` and add `/agentic-stream` to
   `.gitignore`. Add a pre-commit `check-added-large-files` hook (for example
   with a 1 MB limit). Whether to rewrite history to remove the existing blob
   is a separate decision for the maintainers.
2. Make replay's default database a temporary file, or require `--db`. Never
   default to a path next to the input trace.
3. Rename phase-named tests after the behavior they check, for example
   `p8_freshness_test.go` → `episode_freshness_test.go` and `phase04_test.go` →
   `device_profile_admission_test.go`. Replace `// P8:` comment prefixes with a
   reference to the invariant or ADR they implement.
4. Until item 7 lands, hide `config effective`, or make it return an error
   instead of printing a placeholder and exiting 0.

## Done when

- `git ls-files | xargs du -ch` shows no binaries.
- No `_test.go` filename contains a phase number.
- Running the example leaves `testdata/` unchanged.
