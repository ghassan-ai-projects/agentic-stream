# Agentic Stream as a standalone product: research plan

Status: research protocol, 2026-09-24. This plan precedes the findings. It authorizes no implementation or design change.

**Revision during research:** the initial method proposed a weighted score. The deeper pass found no buyer evidence to support stable weights, so that score was withdrawn and replaced with a qualitative evidence matrix, sensitivity conditions and staged gates. See [the deepening plan](DEEPENING_PLAN.md) and [research challenge record](REVIEW.md).

## Decision to support

Determine whether Agentic Stream can become a strong standalone product, for whom, around which job, with what defensible boundary and proof. Explore multiple promising roads before recommending an order of experiments. The attached Open-RAIL/Tamoz discussion is a prompt for examining continuous observation, bounded cognition, governed action, and physical execution; it is not evidence that robotics should be the product.

## Five-whys starting hypothesis

1. Why might users need another runtime? Existing event and agent tools leave a gap between streams and decisions.
2. Why is that gap costly? Alerts, models, and actions can refer to different versions of reality.
3. Why can integrations alone fail? Time, provenance, policy, and recovery semantics cross product boundaries.
4. Why is Agentic Stream a candidate? Its Situation, episode, policy, and replay contracts address those seams together.
5. Why might it still fail as a product? A coherent architecture is not proof of an urgent buyer problem, easy adoption, or differentiated outcomes.

The research must try to falsify steps 2–5.

## Research questions

1. Which specific user and recurring job would pay the adoption cost? What is their current workaround and failure cost?
2. Which neighboring categories overlap: stream processors, IoT/edge platforms, workflow engines, observability/incident tools, agent frameworks, and safety/control systems? Where are the exact boundaries?
3. Which standalone shapes are plausible: embedded/edge Situation Runtime, governed decision automation for operations, replay/evaluation product, physical supervisory bridge, or integration component? What would each exclude?
4. What does the current repository actually implement, what is design-only, and what needs release qualification?
5. What evidence would show product value, willingness to adopt/pay, and technical advantage? What result would kill or pivot each road?
6. How could a sustainable open-source/commercial package work without promising capabilities that do not exist?

## Method

1. Establish current-state baseline from the repository's README, public status/limitations, release manifest, technical design, ADRs, and relevant executable interfaces. Record date and distinguish code evidence from design intent.
2. Search current primary sources: official product docs, project repositories, papers/specifications, and vendor pricing only when clearly dated. Prefer specific feature pages to marketing homepages. Use secondary material only as context and label it.
3. Build a comparison by *job and user workflow*, not by feature count. Include at least one serious substitute and one reason the proposed product could lose in every promising segment.
4. Synthesize opportunity hypotheses with assumptions, integration burden, switching costs, defensibility, distribution, safety limits, and unknowns. Compare evidence and disconfirming cases explicitly; do not numerically rank markets without buyer data.
5. Specify interview, prototype, and pilot experiments with measurable success and stop/pivot thresholds. These are proposals, not implementation commitments.
6. Run an adversarial review: search for category errors, unsubstantiated differentiation, stale feature claims, impossible safety promises, missing buyer, and recommendations inconsistent with the v1 invariants.

## Quality bar and acceptance gates

- **Traceability:** Every material external capability/market claim has a direct source URL and access date in a source ledger. Every repository status claim links to a current repository path. Facts, inference, and hypotheses are visibly separated.
- **Breadth with focus:** Cover at least five neighboring categories and at least four credible product roads, then deeply analyze the best two. Include a do-nothing/partner alternative.
- **Buyer specificity:** For each top road, name user, buyer, trigger, workflow, measurable outcome, incumbent workaround, and adoption blockers. Do not assert market size or willingness to pay without primary evidence.
- **Technical honesty:** Maintain documented/implemented/qualified distinctions; preserve all ten invariants; separate supervisory decisions from real-time physical safety control. State how the attached discussion maps and where it does not.
- **Decision usefulness:** Provide an explicit comparison matrix, counterarguments, uncertainties, disconfirming evidence, staged experiment gates, stop rules, and a decision tree. No broad implementation backlog masquerading as research.
- **Evaluation alignment:** Reconcile any proposed runtime or cognition claim with the repository's evaluation design. Keep deterministic guarantees separate from model-dependent judgement, and do not let a small trace screen license a capability or competitor claim.
- **Reviewability:** Keep an evidence ledger and a findings report with stable sections, links, dates, and a short executive recommendation. Check source URLs, internal links, and `git diff --check`; no code edits or full CI needed for research-only changes.

## Deliverables

- `RESEARCH_PLAN.md` — this protocol and quality bar.
- `SOURCES.md` — dated primary-source ledger and claim boundaries.
- `FINDINGS.md` — current-state baseline, landscape, opportunities, comparisons, experiments, recommendation, and adversarial review.
- `README.md` — short navigation and status.

## Constraints

Research only. Do not change the runtime, contracts, product roadmap, or public claims. Do not treat the attached response's citations as usable source links; verify relevant underlying claims directly. Existing worktree modifications belong to other work and must remain untouched. Token budget favors a bounded source set and high-value comparison over exhaustive cataloging.
