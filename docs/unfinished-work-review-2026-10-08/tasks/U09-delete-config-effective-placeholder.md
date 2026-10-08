# U09 — Delete the `config effective` placeholder

Status: todo · Decision: **delete from code and plan** · Priority: P3 · Size: XS

## Finding

`cmd/agentic-stream/main.go:150` registers `config effective`, which prints
"config effective: not yet implemented". cobra registers it as `config` (the
first word of `Use`), so `agentic-stream config` prints the placeholder as well.
`documentation/reference/cli.md` documents it as a placeholder.

## Decision and reasoning

Delete it. A shipped command that always says "not implemented" is a false
surface. Configuration is flags and three environment variables, all listed by
`--help` and the CLI reference. If redacted effective configuration becomes
necessary (for example for support bundles), it should be designed with the
secrets it must hide.

## Done when

- The command and its registration are gone; `main_test.go` does not expect it.
- `documentation/reference/cli.md` drops the section; TECHNICAL_DESIGN §15.2
  drops the line ([PLAN_CHANGES](../PLAN_CHANGES.md) P02).
