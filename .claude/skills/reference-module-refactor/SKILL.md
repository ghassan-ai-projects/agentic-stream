---
name: reference-module-refactor
description: Rebuild a Go package to the repository's reference-module standard (thin facade, internal/app use cases, pure internal/domain, store or adapter layers), with a planning folder under docs/, ubiquitous language, round-by-round commits and architecture gates. Use when asked to "do the same" as internal/authority for another package, to separate a package's layers, or to make a package a reference implementation.
---

Follow the playbook in `.agents/prompts/reference-module-refactor.md` exactly.
It is the canonical, tool-neutral version of this workflow; this skill only
points to it. The finished examples are `internal/authority` (stateful module)
with its record in `docs/authority-reference-module-2026-10-05/`, and
`internal/device` (adapter module) with its record in
`docs/device-reference-module-2026-10-05/`.
