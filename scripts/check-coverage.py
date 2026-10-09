#!/usr/bin/env python3
"""Enforce quality-bar rule Q4 and testing rule T7 from one run of the short,
race-enabled test suite, so `make ci-check` does not run the tests twice.

Q4: every package with statements has its own tests and at least
MIN_COVERAGE percent statement coverage. The floor is 70%, raised from 60%
when the test audit closed (rule T8).

T7: no top-level test takes more than TEST_BUDGET seconds and no package more
than PACKAGE_BUDGET seconds unless it is in the slow-test register
(scripts/slow-tests.txt). Timings under the parallel race suite swing by
about half between runs, so the budgets are checked at DEFAULT_SCALE times
their value; TEST_BUDGET_SCALE overrides it on a slower machine (CI sets it). Register entries that
are back under budget are reported so the register shrinks.

See .agents/context/quality-bar.md and .agents/context/testing.md.
"""

import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

MIN_COVERAGE = 70.0
TEST_BUDGET = 5.0
PACKAGE_BUDGET = 15.0
SUITE_BUDGET = 35.0
DEFAULT_SCALE = "2"
REGISTER = Path(__file__).with_name("slow-tests.txt")

# Generated protobuf stubs carry no hand-written logic to test.
EXEMPT_SUFFIXES = ("/proto/agenticstream/runtime/v1",)

COVERAGE = re.compile(r"coverage: (\d+(?:\.\d+)?)% of statements")
NO_STATEMENTS = "coverage: [no statements]"


def main() -> int:
    events, returncode, elapsed = run_suite()
    if returncode != 0:
        print("coverage-check: tests failed", file=sys.stderr)
        return returncode
    failures = coverage_failures(events) + budget_failures(events)
    print(f"coverage-check: suite took {elapsed:.1f}s (T7 reference budget {SUITE_BUDGET:.0f}s)")
    if failures:
        for failure in failures:
            print(f"  {failure}", file=sys.stderr)
        return 1
    print(f"coverage-check: every package is at or above {MIN_COVERAGE:.0f}% and within the T7 budget")
    return 0


def run_suite():
    command = ["go", "test", "-short", "-race", "-count=1", "-shuffle=on", "-cover", "-json", "./..."]
    started = time.monotonic()
    result = subprocess.run(command, capture_output=True, text=True, check=False)
    elapsed = time.monotonic() - started
    events, buffered = [], {}
    for line in result.stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            sys.stdout.write(line + "\n")
            continue
        events.append(event)
        echo_failures(event, buffered)
    sys.stderr.write(result.stderr)
    return events, result.returncode, elapsed


def echo_failures(event, buffered):
    key = (event.get("Package", ""), event.get("Test", ""))
    action = event.get("Action")
    if action == "build-output" or (action == "output" and not key[1]):
        sys.stdout.write(event["Output"])
    elif action == "output":
        buffered.setdefault(key, []).append(event["Output"])
    elif action == "fail" and key[1]:
        sys.stdout.write("".join(buffered.pop(key, [])))
    elif action == "pass":
        buffered.pop(key, None)


def coverage_failures(events):
    failures = []
    for event in events:
        if event.get("Action") != "output" or "Test" in event:
            continue
        line = event["Output"]
        package = event.get("Package", "")
        if NO_STATEMENTS in line or package.endswith(EXEMPT_SUFFIXES):
            continue
        match = COVERAGE.search(line)
        if match is not None and float(match.group(1)) < MIN_COVERAGE:
            failures.append(f"Q4 coverage: {package}: {float(match.group(1)):.1f}% < {MIN_COVERAGE:.0f}%")
    return failures


def budget_failures(events):
    scale = float(os.environ.get("TEST_BUDGET_SCALE", DEFAULT_SCALE))
    register = load_register()
    failures, under_budget = [], set(register)
    for package, test, elapsed in timings(events):
        budget = (TEST_BUDGET if test else PACKAGE_BUDGET) * scale
        entry = registered(register, package, test)
        if elapsed > budget and entry is None:
            failures.append(f"T7 budget: {package} {test or '(package)'} took {elapsed:.1f}s > {budget:.0f}s; "
                            f"make it faster or add it to {REGISTER.name} with a reason")
        if elapsed > budget and entry is not None:
            under_budget.discard(entry)
    for suffix, test in sorted(under_budget):
        print(f"coverage-check: register entry back under budget, remove it: {suffix} {test}".rstrip())
    return failures


def timings(events):
    for event in events:
        if event.get("Action") not in ("pass", "fail") or "Elapsed" not in event:
            continue
        test = event.get("Test", "")
        if "/" not in test:
            yield event["Package"], test, event["Elapsed"]


def load_register():
    entries = set()
    for line in REGISTER.read_text().splitlines():
        fields = line.split("#", 1)[0].split()
        if fields:
            entries.add((fields[0], fields[1] if len(fields) > 1 else ""))
    return entries


def registered(register, package, test):
    for suffix, name in register:
        if name == test and (package == suffix or package.endswith("/" + suffix)):
            return suffix, name
    return None


if __name__ == "__main__":
    sys.exit(main())
