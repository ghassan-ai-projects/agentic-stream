#!/bin/sh
# Fails when production code is reachable only from tests.
#
# `deadcode ./...` (without -test) lists every function that no main package
# can reach. Test-support packages exist to be used only by tests, so they are
# exempt: internal/testsupport/... and any package whose name ends in "test"
# (controltest, spectest, contractstest, ...). Everything else it reports is a
# finding: wire it into production, delete it, or move it to test support (an
# export_test.go for a package's own tests).
set -eu

report=$(deadcode ./...)
findings=$(printf '%s\n' "$report" | grep -v -E '^internal/testsupport/|^([^:]*/)?[a-z0-9_]*test/[^/:]*\.go:' | grep -v '^$' || true)
if [ -n "$findings" ]; then
	echo "deadcode: production code reachable only from tests:"
	printf '%s\n' "$findings"
	exit 1
fi
echo "deadcode: no production function is reachable only from tests"
