# U13 — Operator command foundation

Status: done · Decision: **complete** · Priority: P1 · Size: S

## Finding

`release-status.json` lists "packaged operator inspection, approval,
reconciliation, and redrive tooling" as partial, and
`documentation/overview/limitations.md` says a deployment "needs approved
internal tooling" for them. U14–U18 and U22–U23 add those commands. They need
one shared way to open the runtime database and to avoid racing a live runtime.

## Decision and reasoning

One small helper in `cmd/agentic-stream`, with no new package:

- `--db <runtime.db>` (required) and `--tenant` (default `default`), opened with
  `storage.Open`, which runs migrations exactly as `serve` does.
- **Mutating commands claim the runtime owner lease** with their own epoch
  (`control.RuntimeOwner.Claim`), run in one transaction, then release it. If
  `serve` or `run-live` holds the lease, the command fails with "runtime is
  running; stop it or use its control API". This reuses the existing fence
  instead of adding a new one and keeps "one writer of runtime state" true.
- Read-only commands (`list`, `show`, `explain`) take no lease.
- `--json` prints the stable machine-readable form; the default is a short
  table, as TECHNICAL_DESIGN §15.2 requires.
- Every mutation records an audit row through the owning module's existing
  audit path. The CLI writes no SQL (ADR-017: `cmd` contains no SQL).

Exception: `interlock trip` (U14) needs no lease.

## Done when

- The helper exists with tests: lease conflict, lease released on error, read
  commands work while a lease is held.
- `documentation/reference/cli.md` has an "Operator commands" section stating
  the lease rule.
