# Research quality and challenge record

Status: completed second-phase desk-research review, 2026-09-24. This is a review of the research package, not a product or release sign-off. The first section records the historical review of the initial pass; the second records the deeper challenge and current dispositions.

## First-pass review (historical)

The following audit records the initial report's quality check. Its opportunity score and evidence coverage were later found insufficient; the numerical ranking has been withdrawn and the report deepened in [DEEP_DIVE.md](DEEP_DIVE.md).

| Gate from [plan](RESEARCH_PLAN.md) | Evidence and disposition |
| --- | --- |
| Traceability | [Ledger](SOURCES.md) records 26 dated source entries and their limits. Repository capability claims link to current paths. Findings distinguish fact, inference and hypothesis. One official NIST overview is labeled secondary synthesis. |
| Breadth with focus | [Findings](FINDINGS.md) cover stream processing, durable workflows/agents, incident tooling, industrial/edge platforms, modeled business decisions and observability. Six opportunity/delivery roads are mapped; SRE and rotating-equipment supervision receive detailed workflow and pilot treatment. |
| Buyer specificity | Both leading roads name user, potential buyer, trigger, incumbent, measure and adoption obstacle. No user interview, budget, market-size or pricing claim is made. |
| Technical honesty | The report distinguishes code, dated public status, design, qualification and future hypotheses. Physical control remains outside the supervisory product boundary. |
| Decision usefulness | Weighted but low-confidence product-road rubric, separate delivery choices, alternative partner route, assumption/falsification register, ranked experiments and stop rules. |
| Reviewability | Internal links and whitespace checked; `make docs-check` passed. `git diff --check` passed, though it does not inspect untracked files, so those were checked separately. No code or existing worktree changes were made for this research. |

## Independent challenge and resolution

Focused read-only reviewers checked the initial landscape/source match, physical safety/roadmap boundary, and buyer/strategy logic. Their actionable findings were:

1. **AWS alarm overclaim:** SiteWise's IoT Events-backed alarm route depends on a service whose support ended in 2026. Corrected the landscape comparison, source ledger and adversarial review; limited SiteWise claims to current asset/anomaly capabilities and called for checking the customer's actual alarm path.
2. **Score wording:** A and B scored 72 and 71, not equal. Corrected to a one-point difference and added score sensitivity.
3. **OT source currency:** NIST Rev. 4 initial public draft appeared three days before this research date. Added it as a non-final source while keeping Rev. 3 final as the basis for the boundary claim.
4. **Robot alternative:** Added OpenRAL as a project-claimed closer robot-task alternative; avoided field-readiness assertions.
5. **Mixed strategic axes:** Removed packaging and partner-channel alternatives from the numeric product-road score and gave them a separate comparison; added partner-path discovery.
6. **Weak safety inference from two traces:** Changed the retrospective test to a preliminary screen with reported coverage/uncertainty, and required a pre-specified later non-inferiority threshold before any performance claim.
7. **Unverified market premises and broad vertical:** Marked SRE access and noisy evidence as hypotheses; narrowed first industrial discovery to motor/pump maintenance response and separated other assets.
8. **Source precision:** Linked AgentCore runtime and policy separately, bounded the AWS IoT Events migration signal, and labeled the NIST MEP page as secondary synthesis.

## Remaining evidence gap

The initial research could not resolve buyer urgency, budget ownership, willingness to adopt/pay, comparative integration cost, or real-world decision quality. These gaps remain open after the deeper desk phase.

## Second-phase challenge and correction record

Three additional independent reviewers examined the competitive/workflow evidence, economic/adoption logic, and product/evaluation readiness. Their material findings and dispositions are:

1. **The checked-in maintenance example was overstated.** The YAML accepts vibration, temperature, current and heartbeat and declares maintenance-ticket intents. Current golden traces are narrower. RPM, operating mode and maintenance events belong to the broader design acceptance case; they are not all present in the fixture/traces. Corrected [Deep Dive §1](DEEP_DIVE.md#1-plain-language-scope), [Findings Road B](FINDINGS.md#road-b-stationary-equipment-exceptions-at-the-edge), and the trace/spec links.
2. **SRE competition was understated.** Added Datadog Bits Investigation and incident.io Investigations alongside PagerDuty and CloudWatch, with plan/site/preview caveats and an explicit requirement to compare the buyer's live configuration. The sources provide feature claims, not comparative outcome evidence.
3. **Industrial alternatives needed a specialist PdM comparison.** Added Siemens Senseye's documented condition/work-event workflow. Corrected the work-order documentation URL to Siemens' actual `/workorders/` path.
4. **“Late data” was too broad as a possible wedge.** AWS SiteWise documents a 0–60 minute data-delay offset before inference. The report now treats that as a meaningful alternative and distinguishes inference buffering from post-publication Situation correction and pending-Intent revalidation. Those semantics still require a same-trace comparison.
5. **The first economics equation mixed annual and one-time units.** Replaced it with annual contribution per active paying deployment and a dimensionally consistent break-even deployment count; one-time pilots are separate from recurring run-rate, with ramp/churn/discount sensitivities called out.
6. **Shadow results were being asked to prove counterfactual savings.** Clarified that effect-disabled shadow can measure decision quality, lead time, disposition, effort and cost, but cannot observe avoided downtime or customer impact. Any avoided-loss claim needs a pre-specified causal design and independent outcome records.
7. **Adoption intent was too close to commercial evidence.** Split P2 operational shadow from P3 paid commercial validation. The latter requires paid pilot/committed spend and paid extension, renewal or repeat engagement before treating standalone unit economics as credible; it is not product-market-fit proof.
8. **Interview and trace gates differed across files.** Aligned the plan: six conversations per lane as initial screening; expand a promising lane to 8–12 conversations across at least four organizations; require two independent organization/trace families for retrospective screening. These are qualitative discovery gates, not statistical proof.
9. **Industrial purchasing evidence needed a bindingness caveat.** Revised the MultiSensor AI 10-K summary to separate issuer-reported recognized hardware revenue and software subscriptions from preliminary pilots, spot-buy orders, LOIs and strategic agreements that the filing says are largely non-binding and usually lack minimum purchase quantities.
10. **The attached ExecutionRail proposal was not sufficiently classified.** Added a product-boundary analysis: existing supervisory snapshot/expiry/policy/reconciliation semantics; an external qualified executor as default; a general high-rate physical execution rail as a distinct product/design and qualification change.
11. **“Late evidence” alone was too close to a primitive feature claim.** Added current Apache Flink documentation on configurable allowed lateness and late window firings, plus Temporal message-passing semantics for running workflows. The report now compares the integrated application burden and action revalidation contract; it does not imply these components cannot be composed.
12. **The incident.io evidence caveat was broader than the source.** Narrowed it to the blog’s statement that independent research confirming AI-versus-human accuracy for IT alert-severity triage does not yet exist; it is not evidence about product-to-product comparisons. Added dedicated Datadog and incident.io rows to the detailed SRE build/buy matrix.
13. **The replay screen was not aligned with the repository evaluation design.** Mapped P1/P2 to separate deterministic Class D checks and model-dependent Class C capability evidence, including the paired `k >= 3`, held-out-family, interval and per-cell-cost bar. Stated that the evaluation package is design-only, two trace families only screen for direction, and runtime benchmarking needs a separate matched-comparison protocol.

### Independent source checks

- Datadog's own docs describe monitor-triggered investigation and iterative hypotheses; the retrieved docs page said the selected site did not support the feature, and third-party integrations were marked Preview. This caveat is in [S40](SOURCES.md#s40-datadog-bits-investigation).
- incident.io's product page describes its AI investigation offer; its 2026-09-21 blog says independent research confirming AI-versus-human accuracy for IT alert-severity triage is lacking. That caveat is specific to severity accuracy, not product-to-product comparison. Both sources are vendor-published, not neutral outcomes evidence. See S41 in the [source ledger](SOURCES.md).
- Siemens documents that completed work orders become work events and can be pushed by API or connected from an existing maintenance system. See [S42](SOURCES.md#s42-siemens-senseye-predictive-maintenance-workflow).
- Current Flink docs describe zero default allowed lateness and configurable updated window firings; Temporal docs describe in-flight workflow Signals and Updates. See S01 and S03 in the [source ledger](SOURCES.md).
- AWS documents the SiteWise 0–60 minute data-delay offset; SEC filing text supports the MultiSensor AI non-binding caveat. Both are captured with dates and scope limits in the source ledger.
- The separate [`docs/eval/` design](../../eval/README.md) explicitly remains unimplemented; its measurement model and evaluation bar define the deterministic/capability split and the higher evidence gates now reflected in P1/P2.

### Updated review boundary

The current [deep dive](DEEP_DIVE.md) meets the desk-phase bar for workflow/build-buy analysis, product-surface mapping, alternatives, economic hypotheses and falsifiable next gates. It does **not** validate customer demand, prices, installation effort, comparative model quality, safety or product-market fit. P0–P4 in [Findings §8](FINDINGS.md#8-research-to-decision-experiment-program) are proposed future evidence gates; the unified evaluation design they reference is not implemented, so no Class C product capability claim is made. No code, tests, configuration or product contract was changed for this research.
