#!/usr/bin/env python3
"""Enforce quality-bar rule Q4: every package with statements has its own
tests and at least MIN_COVERAGE percent statement coverage.

The floor is 70%, raised from 60% when the test audit closed
(docs/test-audit-2026-10-09/, rule T8). Runs the short, race-enabled test
suite once and reads the per-package coverage lines, so `make ci-check` does
not run the tests twice.
See .agents/context/quality-bar.md.
"""

import re
import subprocess
import sys

MIN_COVERAGE = 70.0

# Generated protobuf stubs carry no hand-written logic to test.
EXEMPT_SUFFIXES = ("/proto/agenticstream/runtime/v1",)

COVERAGE = re.compile(r"coverage: (\d+(?:\.\d+)?)% of statements")
NO_STATEMENTS = "coverage: [no statements]"


def main() -> int:
    command = ["go", "test", "-short", "-race", "-count=1", "-shuffle=on", "-cover", "./..."]
    result = subprocess.run(command, capture_output=True, text=True, check=False)
    sys.stdout.write(result.stdout)
    sys.stderr.write(result.stderr)
    if result.returncode != 0:
        print("coverage-check: tests failed", file=sys.stderr)
        return result.returncode

    failures = []
    for line in result.stdout.splitlines():
        fields = line.split()
        if not fields or NO_STATEMENTS in line:
            continue
        package = fields[1] if fields[0] == "ok" else fields[0]
        if package.endswith(EXEMPT_SUFFIXES):
            continue
        match = COVERAGE.search(line)
        if match is None:
            continue
        percent = float(match.group(1))
        if percent < MIN_COVERAGE:
            failures.append(f"{package}: {percent:.1f}% < {MIN_COVERAGE:.0f}%")

    if failures:
        print("coverage-check: packages below the Q4 coverage floor:", file=sys.stderr)
        for failure in failures:
            print(f"  {failure}", file=sys.stderr)
        return 1
    print(f"coverage-check: every package is at or above {MIN_COVERAGE:.0f}%")
    return 0


if __name__ == "__main__":
    sys.exit(main())
