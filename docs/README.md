# Working documentation archive

This directory is the engineering archive and working source area for Agentic
Stream. The curated public reading path is
[`../documentation/README.md`](../documentation/README.md).

## Archive classes

| Area | Classification | How to use it |
| --- | --- | --- |
| `design/` | Current authoritative design record, machine-facing contract sources, examples, implementation notes | Use with current code/tests; label design-only material when it is not implemented |
| `design-v0/`, `design-v0.1/` | Historical design iterations | Read for rationale and critique, not current behavior |
| `contracts/` | Repository-level notification contract mirror | Prefer embedded runtime files under `internal/notifycontract/contracts/` for code behavior |
| `research/` | Research reports and generated working artifacts | Reference material; generated variants are not public entrypoints |
| `runbooks/` | Engineering/operator working notes | Public summaries live under `documentation/operations/` |
| dated audits and plans | Implementation evidence or historical planning | Check date and status before relying on a claim |

## Authority rules

- Code and tests define implemented behavior.
- `migrations/`, embedded schemas, and the Protobuf source define machine
  contracts as documented in [`../documentation/contracts/README.md`](../documentation/contracts/README.md).
- `docs/design/` is the current design record, but it contains both normative
  intent and planned/deferred surfaces; it is not a substitute for the current
  CLI/API reference.
- `design-v0*` is historical and must not be presented as the current roadmap.
- Research DOCX/PDF/converted variants are working outputs, not required
  reading for a new contributor.

## Keeping the archive usable

When adding a plan, audit, research output, or generated artifact, include its
date/status and link it from an appropriate archive index. Do not place
machine-local paths, secrets, or private agent transcripts in a public-facing
page. If an archive document becomes the current authority, update the public
documentation authority matrix and root navigation in the same change.

## Next reads

- [Public documentation](../documentation/README.md)
- [Current design record](design/README.md)
- [Documentation plan](../documentation/PLAN.md)
