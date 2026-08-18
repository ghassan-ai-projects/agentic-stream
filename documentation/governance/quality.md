# Quality and documentation governance

Agentic Stream treats determinism, safety, and evidence as release gates. The
quality bar for this public documentation is [BAR.md](../BAR.md); this page
connects it to the engineering workflow.

## Everyday checks

```bash
make build
go test ./...
go vet ./...
make docs-check
git diff --check
```

The repository CI-equivalent gate is:

```bash
make ci-check
```

It includes protocol drift, tidy, build, vet, lint, short race-enabled tests,
dead-code detection when installed, and vulnerability checks when installed.

## Change classes

| Change | Required evidence |
| --- | --- |
| Documentation only | `make docs-check`, `git diff --check`, link/content review |
| CLI/API/config docs | build, relevant tests, docs-check, exact surface audit |
| Contract/protocol docs | proto/schema/contract tests and compatibility review |
| Runtime behavior | proving tests, package tests, race/CI gate, security review |
| Migration/action/worker changes | focused recovery/conformance tests and design/ADR review |

## Documentation rules

- Write for a named audience and end with Next reads.
- Use implemented/partial/deferred/unsupported labels.
- Link to exact code, schema, migration, or test evidence for volatile claims.
- Keep `documentation/` curated and `docs/` classified as archive material.
- Do not copy machine-local paths, secrets, generated noise, or private context
  into public pages.
- Update status, limitations, roadmap, and release posture together.

## Review loop

The documentation handoff requires completeness, correctness, code alignment,
writing/diagram quality, and open-source readiness reviews. Findings use P0–P3
severity. P0/P1 findings block handoff; P2 findings require a fix or explicit
deferral; P3 findings are tracked as polish.

## Next reads

- [Documentation bar](../BAR.md)
- [Release evidence](release.md)
- [Roadmap](../roadmap.md)
- [ADRs](../adr/README.md)
- [Latest documentation review](review-2026-08-17.md)
- [Contributing](../../CONTRIBUTING.md)
