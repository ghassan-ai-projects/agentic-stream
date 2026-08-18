# Documentation quality bar

Audience: maintainers and reviewers. Scope: acceptance criteria for the
curated public documentation set and its release handoff.

This is the acceptance bar for the public Agentic Stream documentation. A
documentation pass is complete only when it makes the project understandable,
safe to evaluate, and practical to run without relying on private context.

## 1. Audience journeys

The set must support these readers from the repository root:

| Reader | They need to answer | Required path |
| --- | --- | --- |
| Curious user | What is this, and why does it exist? | README → overview → concepts |
| Evaluator | Does the runtime work, and what is actually implemented? | README → quickstart → compatibility → limitations |
| Operator | How do I run, observe, recover, and secure it? | install → operations → security |
| Integrator | What crosses the runtime boundary? | architecture → contracts → API/CLI reference |
| Contributor | Where do I change code, add a domain, and prove behavior? | contributing → architecture → testing → quality |
| Maintainer | What is the release posture and what remains? | quality → roadmap → ADRs → release evidence |

Every page must identify its audience, state its scope, and link to the next
useful read. The entrypoint must never require a reader to understand the
internal design archive first.

## 2. Truth and freshness

Documentation is evidence-backed. Before a release, reviewers must verify:

- every command, flag, environment variable, endpoint, schema, migration,
  protocol field, package name, and file path against the current repository;
- implementation status against code and tests, not against an old plan;
- safety claims against the ten product invariants and the current policy/action
  path;
- all links from the new documentation tree, the root README, and the
  contribution/security entrypoints;
- examples by running the documented validation or replay commands where the
  environment permits.

When implementation and design disagree, the page must say so explicitly and
link to the issue or decision that resolves the difference. “Planned” and
“implemented” are never interchangeable.

## 3. Minimum public coverage

The curated set must cover:

- product purpose, non-goals, mental model, terminology, and capability status;
- installation, quickstart, deterministic replay, live serving, and the first
  predictive-maintenance example;
- architecture, data flow, persistence, runtime ownership, worker boundaries,
  and the separation between cognition, policy, and actions;
- the ten invariants, trust boundaries, threat model, secrets, authentication,
  replay isolation, and safe failure behavior;
- CLI commands and flags, HTTP/SSE endpoints, configuration, environment
  variables, versioning, schemas, Protobuf, migrations, and compatibility;
- operations: readiness, telemetry, backup/restore, recovery, quarantine,
  idempotency, retention, and known operational limits;
- contribution workflow, test and CI gates, adding a SituationSpec/domain, ADRs,
  release evidence, support, and vulnerability reporting;
- roadmap, limitations, deferred integrations, and a clear current release
  posture.

## 4. Writing and diagrams

Pages should be direct, specific, and short enough to scan. Use concrete names
from the repository, explain jargon at first use, prefer tables for reference
material, and include runnable examples only when they are valid.

Diagrams must:

- show one relationship or flow at a time;
- have a nearby prose explanation and a source-of-truth reference;
- distinguish evidence, durable state, model proposals, policy decisions, and
  external effects;
- make trust boundaries and replay/effect isolation visible;
- remain readable in plain Markdown through Mermaid source.

Decorative diagrams, screenshots that encode no additional information, and
diagrams that contradict code fail the bar.

## 5. Review gates

Before handoff, the documentation must pass five independent reviews:

1. **Completeness:** every required topic and audience journey exists.
2. **Correctness:** claims, links, examples, and status match the repository.
3. **Code alignment:** CLI, API, protocol, schema, migration, and package
   references match implementation and tests.
4. **Writing and diagrams:** prose is consistent, navigable, and technically
   legible; diagrams are accurate and useful.
5. **Open-source readiness:** a new reader can install, evaluate, contribute,
   report a vulnerability, and understand limitations without private context.

Each review records findings by severity:

- **P0:** unsafe or materially false; blocks release;
- **P1:** missing or incorrect path that blocks a normal reader journey;
- **P2:** confusing, stale, or incomplete detail with a practical workaround;
- **P3:** polish, consistency, or discoverability improvement.

The loop ends only when all P0/P1 findings are closed, every P2 has an owner or
explicit deferral, and the final validation record is reproducible.

## 6. Definition of done

The documentation change is done when:

- `documentation/README.md` is the public documentation map;
- the root README points to it and no longer sends new readers into an
  uncurated archive as their first step;
- the working archive remains clearly labeled and linked where necessary;
- the new set contains no broken internal links or unsupported promises;
- the documented quickstart and validation paths have been checked;
- the completeness, correctness, code-alignment, style/diagram, and gap
  reviews have all been performed;
- `git diff --check` passes and the final report lists remaining limitations.
