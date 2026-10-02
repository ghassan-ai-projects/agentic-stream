# Deepening plan: from opportunity map to product decision

Status: completed research protocol, 2026-09-24. This plan responds to the assessment that the first report was not deep enough; the resulting analysis is in [DEEP_DIVE.md](DEEP_DIVE.md). Research only; no code, release claim, public roadmap, or product decision changes.

## Audit of the first pass

The earlier package established breadth and honest uncertainty, but its strategic ranking is too weak to bear weight. Its main evidence was official feature documentation and a current-tree read; it had no direct customer observations, no exact “build it from incumbent parts” comparison, little evidence about buying/installation friction, and a subjective score that can look more precise than it is. It proposed experiments without first showing enough workflow detail to design the right experiment.

This phase will replace the superficial ranking with deeper, inspectable evidence about the product boundary and the alternatives. It cannot create customer interviews or purchasing evidence; those absences must remain explicit.

## Decisions this phase must clarify

1. Is the unique unit a stream runtime, operational condition ledger, governed agent execution service, or a combination? Which buyer owns the pain?
2. For two concrete workflows, how would a customer solve the same job with their current platform plus Flink/Kafka, Temporal, an agent framework, rules, and human review? What glue and operational burden is actually avoided?
3. Which required capabilities exist in code, are exposed as usable operator/developer workflows, are only in design, or need deployment qualification? What is the shortest credible trial path?
4. Which product form can be adopted: standalone daemon, library/embedded component, installed-platform extension, or partner-delivered system? What evidence could discriminate these?
5. What economic value could be measured without assuming market size, failure cost, conversion, or willingness to pay?

## Method and evidence classes

- **A: operational evidence** — customer-provided trace/interview, field observation, signed pilot, or deployment result. None is available in this desk phase.
- **B: primary product evidence** — repository code and tests; official specifications, product docs, release notes, and current vendor pricing/packaging.
- **C: official positioning** — vendor use-case and marketing pages. These show how a vendor sells a problem, not customer outcome proof.
- **D: inference** — an explicit synthesis to test; never relabel as a market fact.

For every workflow: map actors, trigger, inputs, decision, current tools, handoffs, exception paths, authority, resulting action, and proof of outcome. Then compare the minimum plausible incumbent composition to Agentic Stream, identifying new infrastructure, custom code, data movement, operator steps, outages, and accountable owner. Missing source evidence is recorded as unknown, not filled with a feature-absence claim.

For the repository: use a bounded product-surface audit, not a code-quality or implementation review. Classify product requirements as implemented and publicly usable, implemented but hidden/internal, design-only, operationally unqualified, or absent. Point to code/docs and report conflicts without fixing them.

## Higher acceptance bar

- Deeply map **two named workflows** with event sequence, current process, decision rights, exception paths, and outcome metric.
- Compare **at least three realistic solution stacks per workflow**, including the customer's incumbent plus custom glue, with sourced primitives and explicit unsourced unknowns.
- Use six stakeholder conversations per lane only as the first screen; expand a promising lane to 8–12 conversations across at least four organizations, and require two independent organization/trace families for retrospective comparison.
- Build an **adoption critical path** from first download to trustworthy production use; identify each unbuilt integration, operational tool, qualification item, and buyer handoff.
- Replace a single opaque weighted score with an **evidence matrix and sensitivity analysis**. Do not numerically rank markets without buyer data.
- Give each product thesis a falsifiable economic equation and a trace-based measurement protocol. Leave monetary variables blank where no primary data exists.
- Align replay gates with the repository evaluation design: separate exact deterministic-invariant checks from model-dependent capability evidence; label screening evidence clearly; and require the documented paired, held-out, interval, cost, and matched-comparison bars before capability or competitor claims.
- Separate operational adoption intent from commercial proof: define a paid-pilot and repeat/renewal gate; do not infer avoided customer loss or downtime from effect-disabled shadow.
- Challenge the proposed ExecutionRail extension directly against the v1 control boundary; distinguish supervisory Intent/receipt semantics from owning continuous, safety-qualified actuation.
- Distinguish a good open-source engineering project from a sustainable standalone product. Analyze likely adopter, deployer, buyer, procurement/security blocker, route to users, recurring reason to renew, and support cost.
- Independently challenge source interpretation, exact workflows, and code/status claims. Record findings and corrections.
- Run docs-only validation and verify every repository path/claim. Do not run full CI or edit code.

## Deliverables

- `DEEPENING_PLAN.md` — this audit and method.
- `DEEP_DIVE.md` — two workflow maps, competitive build/buy comparison, code-to-product gap map, adoption/economic model, and conditional product decision.
- Extend `SOURCES.md` with additional current primary sources and confidence limits.
- Update `FINDINGS.md`/`README.md` only where the deep dive changes or sharpens earlier conclusions; retain dated first-pass review history.

## Stop rules

- Do not assert that an incumbent lacks a feature based only on a narrow docs page.
- Do not use a vendor TAM chart as proof of Agentic Stream's reachable market.
- Do not claim willingness to pay from published list prices, feature tiers, stars, downloads, or public case studies.
- If no primary evidence supports a value or budget claim, say what exact customer evidence would resolve it.
- Stop comparative expansion when three plausible stacks per workflow establish the key product boundary; avoid cataloging the whole agent/IoT market.
