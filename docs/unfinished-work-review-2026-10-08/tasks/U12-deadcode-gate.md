# U12 — Dead-code gate

Status: done · Decision: **complete** · Priority: P1 · Size: S · Depends on: U02–U07

## Finding

The 189 test-only functions accumulated because nothing checks reachability.
FOLLOW_UPS #17 asked for a make target. `golangci-lint unused` does not catch
them: they are exported or called from tests.

## Decision and reasoning

Add `make deadcode-check`, run by `make ci-check`:

1. Run `deadcode ./...` (no `-test`), the same method this review used.
2. Drop lines under allow-listed paths: `internal/testsupport/`, and packages
   whose last path element ends in `test` (`episodestest`, `spectest`,
   `contractstest`; see U07).
3. Compare the rest with `scripts/deadcode-allowlist.txt`, which holds only the
   symbols of tasks still in progress (U14, U16, U17, U20, U21 until they land),
   each line tagged with its task ID.
4. Fail on any symbol not in the list, **and** on any allow-list line whose
   symbol no longer appears, so the list only shrinks.

Pin the `deadcode` version in the Makefile like the other tools.

Do this after the cleanup tasks so the initial allow-list is short and every
entry has an owner. Once U21 lands, the allow-list should be empty.

## Done when

- `make ci-check` fails when a new production function is reachable only from
  tests, and when a stale allow-list line remains.
- `.agents/context/quality-bar.md` and `testing.md` describe the gate.
- The raw-file follow-up (FOLLOW_UPS #17) is marked done.

## Result

`make deadcode` (part of `make ci-check`, so CI runs it) calls
`scripts/check-deadcode.sh`: `deadcode ./...` without `-test`, minus
`internal/testsupport/...` and packages whose name ends in `test`. Any other
finding fails the build. The `deadcode` version is pinned in the Makefile
(`GO_TOOLS_VERSION`, matching CI).

No allow-list file: once U14–U21 landed, the only production findings left
were `replay.RunMode` and `replay.NewDeterministicBaseline`, which only the
package's own tests called. They moved to `internal/replay/export_test.go`, so
the list would have started empty, and an empty list with a "must only shrink"
rule is machinery without a use. If an in-progress task ever needs one, add it
then.

Verified: the gate passes on the tree and fails on a probe function called only
by a test.
