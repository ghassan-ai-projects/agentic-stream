# Deep dive: what would make Agentic Stream worth adopting?

Research continuation · 2026-09-24 · desk research and read-only repository review

This document deepens [the first findings report](FINDINGS.md) after its opportunity ranking proved too shallow. It replaces the numerical ranking with workflow comparisons, current product-surface evidence, economic tests, and explicit decision gates. This is not an implementation plan, release statement, market-size estimate, or product-roadmap change.

## Executive decision

The first report described a plausible “operational Situation supervisor” but did not establish why a buyer should add it to an already substantial operations stack. A stronger reading of the current evidence is:

1. **The underlying engineering problem is coherent.** A runtime that turns changing evidence into versioned conditions, admits bounded reasoning, rechecks authority, and records outcomes has a clear architectural purpose.
2. **The standalone product thesis is still unproven.** There is no customer evidence that the integrated contract is missing from a buyer's stack, causes material loss, or reduces total cost enough to justify another process, database, integration and support relationship.
3. **SRE incident response is much more crowded than the first report made clear.** PagerDuty now combines event orchestration with an SRE Agent and configurable Skills; CloudWatch Investigations, Datadog Bits Investigation and incident.io Investigations also document multi-signal AI incident workflows. These are direct workflow alternatives, not merely adjacent agent frameworks. Feature availability varies by customer plan, site and release status, and vendor capability claims do not establish comparative outcomes. [S31–S33 and S40–S41 in the source ledger](SOURCES.md)
4. **Industrial pump maintenance has documented pain, but not a demonstrated need for an agent runtime.** A NIST MEP case describes recurring pump failures improved through lean process, failure tracking, checklists and preventive maintenance. Its published page does not quantify the original 75% downtime-reduction goal. This is both evidence of a real operational problem and evidence that process work may beat new software. [S27](SOURCES.md#s27-nist-mep-pump-downtime-case)
5. **The best next move is a narrow, read-only comparison on real traces.** Test one SRE workflow and one stationary-equipment workflow against each customer’s actual incumbent configuration. Keep replay as the proof surface. Do not select a vertical or claim differentiation until an operator and budget owner confirm a recurring, costly decision that the current stack does not handle well.

My current recommendation is **continue product discovery, but do not yet declare or package Agentic Stream as a broad standalone product**. The most credible thesis to test is a *supervisory condition and decision record* that is valuable when evidence changes while a diagnosis or action is in flight. “Agentic,” “streaming-native,” use of Go/SQLite, and an LLM are not customer value by themselves.

## 1. Plain-language scope

### What “incident” means here

An **SRE incident** is a production software service or platform disruption that needs a coordinated response: a service is unavailable, slow, returning errors, breaching an SLO, or otherwise affecting users. Monitoring may detect it; an incident platform routes and groups alerts; an on-call engineer or incident commander coordinates investigation, mitigation, communication and handoff. The incident is not just an alert or a model-generated summary. Google’s SRE material describes a live incident document, explicit coordination and handoffs as part of the response. [S29](SOURCES.md#s29-sre-alerting-and-live-incident-practice)

The product question is whether Agentic Stream can make a particular step in that response materially better—for example, keeping one reproducible condition current as late telemetry and deploy events arrive, or preventing a proposed action from proceeding after the evidence that justified it has changed.

### What “motor/pump maintenance” means here

A **motor or pump maintenance exception** is a stationary machine showing evidence that may indicate degradation and could require monitoring, inspection, or a work order. The checked-in predictive-maintenance SituationSpec accepts vibration, temperature, current and heartbeat inputs and includes maintenance-ticket intents; its current golden traces are narrower, mostly vibration and heartbeat examples. The design's broader MVP acceptance case calls for simulated RPM, operating-mode and maintenance events too. Those are design targets, not evidence that the checked-in fixture or traces already cover them. The model is explicitly told not to stop or control the motor. All of this is simulation, not field data. [predictive-maintenance spec](../../design/examples/predictive-maintenance.situation.yaml) [current golden traces](../../../examples/predictive-maintenance/testdata/) [design acceptance case](../../design/README.md#mvp-proof)

The proposed runtime would supervise evidence and perhaps draft a maintenance recommendation. It would not become a motor controller, PLC, safety system, CMMS, historian or certified condition-monitoring product. Existing plant systems and qualified people keep those roles. The word “predictive” describes the example’s purpose; no failure-prediction accuracy or real maintenance outcome has been validated.

## 2. Two workflows, end to end

These are specific research targets, not statements about all SRE teams or plants. The event examples are illustrative. The NIST and vendor sources establish that related workflows exist, but only customer traces can establish frequency, severity and current failure modes.

| Stage | Software service incident | Stationary motor/pump exception |
| --- | --- | --- |
| **1. Signal** | An SLO, metric rule, log pattern, trace, customer report or platform health signal indicates a service may be impaired. A deployment/change event can be relevant context. | Vibration, temperature, current, speed, mode, heartbeat, operator observation or maintenance history suggests a machine may be deteriorating. SiteWise already offers native anomaly detection for relevant asset types. [S34](SOURCES.md#s34-aws-sitewise-native-anomaly-detection) |
| **2. Normalize and identify** | Map signals to one service, environment, time range, severity and owner. Conflicting clocks, duplicated alerts and missing telemetry can make the initial picture incomplete. | Map readings to one asset and operating mode, validate units and sensor health, and align them with maintenance/work-order history. The current Agentic Stream ingress expects normalized events; the customer owns normalization and asset mapping. |
| **3. Decide whether there is a real condition** | Combine evidence, check alert grouping and dependencies, choose whether to page, open or associate an incident, or leave a ticket for later. Prometheus Alertmanager already groups, deduplicates, routes, silences and inhibits alerts. [S30](SOURCES.md#s30-prometheus-alertmanager-primitives) | Separate a persistent degradation pattern from a transient reading, missing sensor, maintenance mode or expected load change. Decide to monitor, inspect, schedule work, or take no action. A threshold or anomaly score is only one input to that decision. |
| **4. Diagnose with bounded evidence** | Responder checks logs, metrics, traces, recent changes, runbooks and related incidents. PagerDuty, CloudWatch, Datadog Bits Investigation and incident.io Investigations all document AI-supported investigation over overlapping telemetry/change/incident context for eligible customers. [S31–S33 and S40–S41](SOURCES.md) | Reliability engineer checks trend, mode, sensor quality, asset criticality, prior failures and work history. A model could explain or draft a recommendation from a fixed evidence snapshot, but maintenance labels and useful context must come from the plant. |
| **5. Authorize a response** | Incident commander or service owner chooses mitigation; a configured workflow or runbook may perform a bounded action. Human coordination remains a core part of the process. | Maintenance/reliability owner approves inspection or work-order priority. Operations may need to schedule downtime. Any physical control remains under the plant’s existing control and safety authority. |
| **6. Act and reconcile** | A ticket/workflow/runbook is dispatched. A timeout may mean the API accepted the action but its response was lost; the system must reconcile before retrying. | A CMMS/work-order API may accept a draft, while real inspection, parts, repair and return-to-service occur later. “Ticket created” is not “pump repaired.” |
| **7. Verify and close** | Check service health and user impact, record timeline, close or keep the incident active, hand off ownership, and feed the postmortem. Google and PagerDuty describe durable incident records and timelines. | Confirm the physical inspection/repair result and subsequent operating evidence. Update the maintenance record and check whether the expected degradation cleared. An anomaly returning to normal is not by itself proof that a machine is healthy. |
| **Decision owner** | On-call/SRE and incident commander; likely economic owner is a platform/reliability engineering leader. Budget and procurement authority are not validated for this product. | Reliability engineer and maintenance/operations owner; OT/IT and an integrator may control deployment. The economic buyer and procurement path depend on the plant and are unknown. |
| **Useful outcome measures** | Severe incident misses, duplicate/noisy pages, time to useful diagnosis, reviewer time, stale or unsafe proposals, time to verified recovery, integration/operations cost. | Missed failures at a partner-set recall threshold, false inspections/work orders, lead time to useful work, operator/maintenance time, downtime verified independently, integration and commissioning cost. |

### Exceptions that reveal a real product gap

For SRE, candidate hard cases include a condition whose meaning changes as late logs or deploy events arrive, multiple services generating a shared symptom, a source going silent, or an action becoming stale while an agent investigates. Existing incident products handle grouping, ongoing investigations, notes and integrations to varying degrees. The product gap is therefore **not established** until a concrete trace demonstrates that the incumbent misses or mishandles one of these cases and the Agentic Stream replay catches it with acceptable operating cost.

For maintenance, candidate hard cases include a sensor reading that arrives late, an operating-mode change that explains a spike, two indicators that disagree, telemetry dropping out, or a maintenance recommendation still pending after the condition has materially changed. Existing IoT platforms, anomaly detection, historians, CMMS tools and human procedures can already cover parts of the workflow. A reliable process/checklist may solve some failures at lower cost than adding a runtime, as the NIST MEP case suggests. [S27](SOURCES.md#s27-nist-mep-pump-downtime-case)

## 3. What customers can assemble today

These comparisons list documented primitives and the glue a customer would still own. They do not claim that an incumbent lacks unlisted functions. A complete production comparison requires a partner’s actual license, configuration, connectors and trace.

### Service incidents

| Plausible stack | What it supplies | Customer-owned work and residual burden | Agentic Stream’s testable opening |
| --- | --- | --- | --- |
| **A. Installed observability + PagerDuty + responders** | Alert rules, dedupe/group/route, on-call and incident timeline; event orchestration and configured automation. PagerDuty documents an SRE Agent with log and runbook integrations and diagnostic/remediation suggestions; separate connector and Skills material broadens available context and instructions. The docs describe plan requirements and some early-access components, so verify account eligibility. [S31](SOURCES.md#s31-pagerduty-sre-agent-connectors-and-skills-availability) [S32](SOURCES.md#s32-pagerduty-ai-orchestration-and-commercial-maturity) | Tune signals and integrations, map ownership, create incident rules and skills, govern credentials, supply runbooks, review agent conclusions, and retain human incident command. The platform may already be “good enough.” | Demonstrate on the buyer’s incidents a meaningful advantage in corrected condition history, reproducibility or stale-action prevention. Check whether a documented limit matters to operators and remains in their current plan/version. |
| **B. AWS-centered CloudWatch operations** | Metric/log/change investigation, multi-resource hypotheses, accept/discard findings, continuing investigations, reports and suggested Systems Manager runbooks. [S33](SOURCES.md#s33-aws-cloudwatch-investigations-overlap) | Configure telemetry access, IAM, resource mappings, runbooks and customer-specific diagnosis. Validate permissions and cost; an investigation suggestion does not prove its cause or remediation is correct. | Only relevant if a trace exposes a gap in the buyer’s CloudWatch setup, such as non-AWS sources, changing evidence snapshots or cross-system outcome reconciliation. Broad AI incident diagnosis overlaps directly. |
| **C. Datadog observability + Bits Investigation** | Datadog documents manual or monitor-triggered investigations that query supported telemetry iteratively, update hypotheses, and add incident summaries and timeline context. External observability, code and knowledge integrations are documented, with some third-party integrations marked Preview and API/Terraform limits. The selected docs site did not support the feature during this research check. [S40](SOURCES.md#s40-datadog-bits-investigation) | Confirm site, region, plan and feature availability; connect supported data; govern access; check how its incident context and results fit the team's existing paging and response flow. The published docs do not settle every tenant's configuration. | Test whether the exact customer's Bits setup already handles the target traces. Only investigate a gap if the condition changes after an investigation starts, requires a cross-platform versioned evidence record, or needs pre-effect policy revalidation that the configured flow does not provide. |
| **D. incident.io + Investigations / AI SRE** | incident.io describes an investigation triggered at incident declaration, searching telemetry, code changes and prior incidents and returning an evidence-backed report to the incident channel. Its dated blog also describes enrichment, deduplication, timeline construction and human review; it cautions that independent research confirming AI-versus-human accuracy for IT alert-severity triage does not yet exist. [S41](SOURCES.md#s41-incidentio-investigations--ai-sre) | Verify the exact available integrations and review/approval flow; curate service and incident history; keep responders accountable for severity and production actions. Vendor workflow descriptions and testimonials are not comparative results. | Use the customer's actual incident traces to compare evidence freshness, reproducibility, operator review effort, stale proposals and recovery outcomes. Do not treat the vendor's narrow caveat about severity-accuracy research as evidence for or against other product comparisons. |
| **E. Kafka/Flink + incident platform** | Flink exposes event-time watermarks and configurable allowed lateness; window lateness defaults to zero, and configured lateness can refire updated window results. Kafka Streams/Flink provide state and recovery/checkpoints; the incident tool handles paging and collaboration. [S01–S02](SOURCES.md) | Build and operate connectors, schemas, identity joins, event-time/correction policy, retained state, duplicate/update handling, Situation semantics, explanation, incident writeback and external action reconciliation. This can be flexible and correct, but it is an engineering product the customer owns. | Show lower total implementation and operating effort for the integrated temporal condition-to-action contract at a fixed result. Compare production-quality Flink configuration and code; do not compare only against naive per-event logic. |
| **F. Temporal/agent framework + incident tools** | Temporal supplies durable workflow history/replay and Signals or Updates to change a running workflow; Activities perform external I/O. LangGraph supplies checkpointing and pause/resume patterns for an agent flow. [S03–S05](SOURCES.md) | Still design source ingestion/aggregation, event-time and correction semantics, identity joins, which evidence snapshot an episode sees, how new messages supersede stale work, access control, safe external calls and reconciliation. These primitives can host much of the lifecycle, but the stream-to-workflow adapter and application rules remain customer work. | Prove that a separate stream-native product reduces enough custom state/correction/policy glue to justify another runtime. Compare a realistic workflow implementation, including message histories, operations and failure recovery. Do not infer differentiation from the fact that custom composition is possible. |

**SRE conclusion:** this lane has many mature tools and a weak generic wedge. PagerDuty, CloudWatch, Datadog and incident.io document substantial overlap in alert-to-investigation workflows. Flink can implement event-time correction and Temporal can receive updates within durable workflows, although each leaves application-level semantics and integration to the customer. Feature gaps, eligibility, configurations and outcomes vary by tenant. Current public docs do not establish whether these systems deliver Agentic Stream's exact immutable, corrected Situation snapshot and pre-effect policy revalidation as a ready integrated workflow; nor do they prove that a buyer needs those semantics. Test the installed alternatives side by side on the same trace before making a competitive claim.

### Motor/pump maintenance

| Plausible stack | What it supplies | Customer-owned work and residual burden | Agentic Stream’s testable opening |
| --- | --- | --- | --- |
| **A. Existing historian/CMMS + threshold rules + maintenance process** | Existing plant telemetry and asset history, rules, work orders, technician experience, inspection and failure tracking. NIST’s pump case improved downtime-related practice through checklists, failure logs, root-cause analysis and preventive maintenance after a $6,000 workforce-development investment. The page does not report that the 75% goal was achieved. [S27](SOURCES.md#s27-nist-mep-pump-downtime-case) | Keep identifiers and work-order discipline accurate; investigate sensor quality and modes; connect equipment evidence to maintenance history. If the real defect is inconsistent process, a rule-and-checklist improvement may be the best fix. | Demonstrate better maintenance decisions than the plant’s real existing process, including a rules-only baseline. Do not infer that a model or runtime is required because a pump failed. |
| **B. AWS SiteWise or Azure IoT Operations + CMMS** | Edge/device integration, asset models and telemetry handling; AWS documents native anomaly detection for stationary machinery including motors/pumps; Azure IoT Operations provides edge flows and asset/device registry. AWS guidance calls for at least 14 days spanning normal modes and says SiteWise native anomaly detection does not support data below 1 Hz. SiteWise also documents a configurable 0–60 minute data-delay offset before inference to include late arrivals. [S10, S34 and S35](SOURCES.md) | Configure edge cluster/gateway, asset and tag mapping, operating modes, model training, alarm routing and CMMS-specific work-order integration. SiteWise's delay offset is a credible alternative for buffered inference; test its timing, coverage and resulting action workflow against the customer's actual lateness distribution. It should not be equated with correcting an already-published condition version or revalidating a pending intent. AWS IoT Events support ended in May 2026, so the legacy SiteWise alarm path must not be used as a new-deployment baseline. Verify the customer's actual migration and CMMS integration. See S23 in the [source ledger](SOURCES.md). | Focus downstream of a platform’s anomaly detector: prove that a replayable condition history and governed recommendation improve maintenance response after detection. Do not sell generic edge ingestion or predictive analytics. |
| **C. Siemens Senseye PdM + existing maintenance system** | Siemens documents asset condition insights (including anomaly, trend, forecast/degradation/failure), data-quality signals, cases and work-event integration with an existing maintenance system. This is a direct specialist alternative where already deployed. [S42](SOURCES.md#s42-siemens-senseye-predictive-maintenance-workflow) | Confirm the customer's exact Senseye modules, sensors, hierarchy, integrations and decision process. The public developer material does not settle comparative correction semantics, buyer fit, price or the work required to connect each CMMS. | Test the same question as with SiteWise: is the remaining pain after detection the changing evidence-to-work decision, and does Agentic Stream improve it without duplicating a capable PdM workflow? |
| **D. Kafka/Flink or cloud stream primitives + CMMS** | Stateful per-asset event processing, event-time windows and recovery; cloud services can be composed for ingestion and routing. | Plant or integrator builds unit conversion, asset identity, operating-mode semantics, model evaluation, maintenance joins, scheduling policy, CMMS effects and field support. Real-world commissioning can dominate the cost of a small runtime. | Reduce custom condition-state and audit code enough to justify adding a new service, while remaining compatible with existing OT architecture and safety boundaries. Measure engineer/integrator days and support burden. |
| **E. Anomaly detector + Temporal/agent + CMMS** | Anomaly detection produces candidates; a durable workflow can wait for approval, schedule inspection, retry an API, collect evidence and close the work process. An agent can summarize a bounded asset snapshot. See S03/S04 for Temporal workflow/activity capabilities and S05 for LangGraph. | Decide from telemetry what starts/updates the workflow, bind recommendations to a version of asset evidence, recheck whether the condition changed, create a CMMS adapter, and verify physical result. Work-order approval does not confer authority to actuate plant equipment. | Package the change-sensitive condition and action-revalidation contract across those pieces. Test whether this shortens diagnostic/approval work without increasing false work or requiring complex custom engineering. |

**Industrial conclusion:** this is a more specific operational pain and a close fit to the design direction, but the checked-in fixture remains narrower than the acceptance target and is entirely synthetic. It also has higher deployment friction, native anomaly-detection alternatives and a strong “process improvement first” counterexample. SiteWise's delay offset and Flink's configurable lateness handling mean that late data alone is not a product wedge. Test whether published-state corrections, cross-signal Situation history, and revalidation of already-pending maintenance work create a material downstream advantage. Industrial AI market pages and sensor-vendor revenue cannot demonstrate demand for a standalone software runtime. NIST’s national maintenance-economics report explicitly warns that public data do not support precise benefit estimates without strong, error-prone assumptions. [S28](SOURCES.md#s28-nist-maintenance-economics-evidence-limit)

## 4. What the current repository can support

The repository is a serious engineering prototype. “Implemented” here means present in the current development tree, not supported by a stable release or qualified in a customer environment. Public docs describe today’s user surface and deployment caveats; code may have surfaces that docs do not yet expose.

| Capability state | Evidence in current checkout | Product meaning |
| --- | --- | --- |
| **Callable and documented, narrow** | YAML SituationSpec validation; normalized JSONL replay; simulator traces; deterministic batch run; local `serve` with file/normalized JSONL input; health/readiness, metrics and authenticated SSE; simulated effect path. See [quickstart](../../../documentation/getting-started/quickstart.md), [CLI reference](../../../documentation/reference/cli.md), and [HTTP/SSE reference](../../../documentation/reference/http-api.md). | Enough for an engineer to prove runtime semantics on the included fixture or a normalized trace. Public in documentation does not mean stable, packaged or supported. |
| **Implemented but operator-internal or undiscoverable** | Approval resolution ([policy approval](../../../internal/policy/policy_approval.go)), quarantine release/redrive ([event log](../../../internal/eventlog/quarantine.go)), unknown-action-outcome reconciliation ([dispatcher](../../../internal/actions/dispatcher.go)) and automatic recovery ([runtime recovery](../../../internal/runtime/recovery.go)) exist internally. The CLI also registers `export-run` and `verify-run` ([CLI wiring](../../../cmd/agentic-stream/main.go)), but the public CLI reference omits them. | Important product logic exists, but a customer operator cannot use a supported general interface to inspect and resolve common operational states. Export/verification artifacts help evidence handoff but do not replace a full operator console. |
| **Integration work the customer would supply** | Broker/MQTT/Kafka/NATS sources and generic CMMS, PagerDuty or CloudWatch writeback are not current public connectors. Ingress requires normalized data. External effects need integration-specific idempotency or reconciliation. | Customer or partner must normalize sources, map identities, handle credentials and build destination integration. This is a material pilot and support cost, not a checkbox. |
| **Absent from current public surface** | No general situation/episode/intent query and explanation API; no public approval resolution, quarantine redrive, or unknown-outcome reconciliation workflow; no UI; no stable external SDK package. The HTTP API is intentionally small. | Operators would otherwise need internal tooling or unsafe database access. A standalone product needs to make diagnosis, approvals, recovery and audit routine operations. |
| **Operationally unqualified** | Release status remains development/unreleased. Backup/restore rehearsal, disk-full and unclean-shutdown evidence, soak/capacity, environment security, compatibility and provenance/rollback remain release gates. Physical/emulator paths exist in code but have no field qualification. [release posture](../../../documentation/governance/release-status.json) [limitations](../../../documentation/overview/limitations.md) | The core’s tests and replay history do not establish production operation, safety certification, or support readiness. Do not position the physical gateway as a ready industrial product. |

### Adoption path from a source checkout to a trustworthy trial

| Step | What a design partner would need | Current path and gap |
| --- | --- | --- |
| **1. Choose a bounded job and obtain trace access** | Named workflow owner, permitted historical data, source-system baseline, outcome labels and security/privacy approval. | Research must establish this; the repository cannot supply customer evidence. |
| **2. Normalize source records** | Stable event IDs, event time, entity/service/asset ID, schema, units, mode and source-health meaning; adapters that tolerate gaps and duplicates. | Normalized JSONL is supported; connector ingestion and customer-specific normalization are not. |
| **3. Encode and validate the condition** | Authoring model for domain thresholds/windows, expected absence, correction, alert/action rules; validation errors an operator can fix. | YAML authoring and compiler exist. Current examples are domain fixtures; an unfamiliar customer must adapt schemas and rules. |
| **4. Replay against the incumbent** | Diff condition history and proposals against labeled cases and current tools; inspect every false/missed candidate and explain why. | Deterministic replay exists; export/verify help produce evidence. Operator-facing browse/compare/explain commands are incomplete or undocumented. |
| **5. Run continuous shadow mode** | Supported package, durable input adapter, source-health view, runtime monitoring, local storage policy and recovery procedures. | `serve` supports narrow local input/UDS patterns and health/metrics/SSE; there is no turnkey connector, browser UI, or full operations interface. |
| **6. Integrate useful output** | Create an incident annotation, review queue, or draft work order with stable idempotency keys; record approvals, unknown outcomes and actual result. | No standard PagerDuty/CMMS connector; general resolution/reconciliation operators are internal. The integration and outcome source remain customer-specific. |
| **7. Keep it running safely** | Install/update/rollback, backups, restore drill, key handling, least privilege, capacity/soak evidence, support policy and compatibility promise. | Release and deployment qualification are pending. A local listener/token boundary is not a full deployment package. |

The current shortest credible path is therefore **developer-led offline proof followed by a custom shadow integration**, not download-connect-operate. The gap from a good prototype to a standalone product is mostly integration and operations: source onboarding, inspectability, effect destination, restore, upgrades, security and support. It is premature to prioritize multi-node scale before showing that the single-node workflow wins.

## 5. Economic and commercial reality

### What public data do and do not tell us

There are established paid categories around incident management, stream processing, workflow execution, observability and industrial monitoring. PagerDuty reported 15,351 paying customers at FY2026 year-end, including 861 above $100k ARR; this is strong evidence of a mature paid incident-operations category and an incumbent with substantial distribution—not evidence of unmet spend for Agentic Stream. [S32](SOURCES.md#s32-pagerduty-ai-orchestration-and-commercial-maturity)

Pricing pages show different value/cost units: workflow actions and stored histories, stream compute and throughput, signal/series volume, Kubernetes nodes, assets, gateways and support. For example, Temporal meters Actions and storage; Azure prices IoT Operations by billable Kubernetes nodes and Device Registry by assets/devices; SiteWise Edge charges separately for gateway processing and other services. [S35](SOURCES.md#s35-azure-iot-operations-price-and-deployment-units) [S36](SOURCES.md#s36-workflow-runtime-commercial-price-unit) [S38](SOURCES.md#s38-published-adjacent-pricing-does-not-establish-demand)

An industrial sensor supplier’s filing also reports software bundled with hardware and commissioning services. That is a warning to measure deployment labor and field support, not proof that Agentic Stream must bundle sensors or that this runtime has a market. [S37](SOURCES.md#s37-industrial-maintenance-purchasing-proxy)

These sources establish that adjacent categories are commercialized. They do **not** reveal a buyer’s negotiated prices, total internal build cost, willingness to pay for a condition supervisor, or the economics of this project. The NIST maintenance research warns against deriving broad failure-avoidance value from weak public data. [S28](SOURCES.md#s28-nist-maintenance-economics-evidence-limit)

### Falsifiable value equations

Keep all money variables blank until a partner provides source records and approves the accounting method.

**SRE incident supervision**

> Net annual customer value = measured reduction in responder labor + verified customer-impact reduction + verified false-escalation reduction − product total cost of ownership.

TCO includes license, compute/storage and duplicated telemetry, source normalization, incident/observability connectors, operator review, model calls, security/procurement, change management and support. Do not count MTTR reduction as financial benefit without an accepted causal method: faster closure may not reduce customer impact. Count severe misses and stale/unsafe proposals as guardrails, not just average speed.

An effect-disabled shadow can estimate alert/condition coverage, diagnosis usefulness, review effort and decision latency. It cannot by itself establish that a proposed action would have reduced customer impact or prevented an incident. Any avoided-impact claim needs a pre-agreed counterfactual or causal design and independently recorded customer-impact data; otherwise report the operational proxy and its uncertainty.

**Industrial exception supervision**

> Net annual customer value = verified avoided unplanned-stop loss + verified emergency-repair or secondary-damage avoided + maintenance labor/cost saved − false-work and interruption cost − product total cost of ownership.

Use CMMS orders, independent downtime/production records, sensor data, planned work, parts/labor and site deployment time. Do not count an anomaly as an avoided failure. The partner must show that the recommendation changed a maintenance decision and independently verify the subsequent outcome.

In effect-disabled shadow, “avoided downtime” is counterfactual and cannot be observed directly. Initially measure condition quality, lead time, recommendation disposition, review effort and false work. Only attribute avoided stoppage or repair loss after a pre-specified design—such as matched asset families or a prospective stepped rollout where interventions are safe—with independent production/downtime records and an explicit account of confounding. Do not price hypothetical avoided failures as observed savings.

**Replay/evaluation as an entry product**

> Net annual customer value = review/replay time demonstrably removed + confirmed regression or release losses avoided − trace normalization, labeling, storage and engineering cost.

Replay is the shortest product proof, but there is no evidence yet of a separate budget owner or recurring paid use. Treat it as a trust/distribution feature until repeated use and renewal are observed.

**Seller viability**

> Annual contribution per active paying deployment = annualized recurring or renewal-equivalent receipts + properly annualized services − annualized infrastructure/model and duplicated-telemetry cost − annualized onboarding and integration labor − annualized direct support labor − partner share.

> Break-even active paying deployments = annual fixed product, security/release and go-to-market costs ÷ mean annual contribution per active paying deployment.

The units now match: annual costs divided by annual contribution per deployment gives the active deployment count needed at steady state. Model one-time paid pilot revenue and delivery cost separately from recurring run-rate. Stress-test ramp time, churn, discounts, partner share and uneven support load before drawing a cash or headcount conclusion. Integration days, support incidents, environment variance, availability expectations, security review, deal/renewal cycle and recurring reason to renew are unknown. Do not pick a price meter from comparators alone. Test per site/instance, per asset, usage and support only after observing customer value and delivery cost.

### Who must adopt, and where it could be distributed

| Role | SRE lane | Industrial lane | Evidence to collect |
| --- | --- | --- | --- |
| Daily user | On-call engineer/SRE; incident commander | Reliability engineer, maintenance planner or operator | Observe a recent case and measure time spent switching tools and confirming the outcome. |
| Technical adopter | SRE/platform engineering team | Plant IT/OT engineer or system integrator | Ask who owns the source adapters, host, database, identity and upgrades. |
| Budget owner | Reliability/platform leader, possibly engineering operations | Maintenance/reliability/operations leadership | Ask for current spend and approval route, not hypothetical “would you buy?” answers. |
| Likely blocker | Existing incident vendor’s sufficient capabilities, security of telemetry/tool credentials, duplicate alert flow | OT security review, legacy protocols, site variation, commissioning, safety boundary and outage window | Record a real review checklist and lead time. |
| Plausible distribution | Complement existing PagerDuty/observability stack; perhaps engineer-led replay trial | Integrator/SI, edge-platform extension or locally deployed shadow service | Compare what an incumbent extension or SI can provide with lower customer burden than a new daemon. |

Four packaging routes remain plausible and mutually testable: a local binary with support; a container or component within an already approved edge/cloud stack; an extension/partner integration sold through an incumbent; or SI/MSP-delivered deployment. Marketplace procurement is a later route after a functioning, installable and production-qualified package; marketplace listings do not substitute for distribution or customer proof. Existing marketplace gates and technical requirements are documented in [S39](SOURCES.md#s39-marketplace-distribution-requirements) and should be rechecked before that decision.

| Product form | What would make this form fit | Evidence required before choosing it | Failure signal |
| --- | --- | --- | --- |
| **Standalone local service** | Operators need an always-on condition record across sources and will own another local runtime/database. | Repeated workflow, named operating owner, security/install acceptance, support and restore plan, and a paid buyer who renews for ongoing value. | Buyers request only a one-off analysis or refuse another running service. |
| **Installed-platform extension** | The value can live within a buyer's existing observability, edge or workflow platform and can use its identity, deployment and procurement path. | A platform owner wants the integration, grants a supported extension/API path, and would distribute or fund it. | Extension APIs cannot support versioning/revalidation, or platform vendor can reproduce the value with a native feature. |
| **Replay/evaluation tool** | Teams repeatedly use historical evidence to compare model/rule changes or inspect stale decisions. | Repeat use in a real release or operational gate, a budget owner, and paid renewal/extension or funded support need. | It remains a useful demo or occasional engineer utility without a recurring funded job. |
| **Integrator/SI-delivered product** | Customer value is real but source mapping, site variation and commissioning require field expertise. | A partner can deliver consistent outcomes with bounded effort, has margin after support, and accepts the evidence/safety boundary. | Every deployment becomes bespoke consulting or the runtime adds more support burden than partner margin. |

These are delivery alternatives, not market segments with known size. The default research sequence is to test user preference and burden before choosing: require repeated use and a payer for a standalone service; choose extension or partner delivery if it removes a documented procurement or integration obstacle; keep replay as a proof feature if it lacks a separate recurring budget. This is a decision rule, not a recommendation to implement any route now.

The open-source model can help engineers inspect and trial the runtime. It does not pay for compatibility testing, connectors, on-call support, field commissioning or long-term security maintenance. A hosted control plane is not automatically the answer; it must preserve customer control of evidence, approval and physical effect authority.

## 6. Where the thesis is sensitive

The value opportunity is not uniform. This decision matrix avoids numeric weights because buyer evidence is missing.

| Observed customer condition | Likely result | Product implication |
| --- | --- | --- |
| One or two simple signals, stable threshold, straightforward response | Existing alert or CMMS rule likely wins. | Do not sell a new runtime. |
| Existing incident/edge platform already correlates the right signals and operators trust its investigation/action flow | Native feature or extension likely wins. | Integrate only if customers show a specific remaining gap; consider partner route. |
| Repeated cross-source condition; changing or late evidence makes current alert/workflow stale; incident/equipment outcome is expensive; labels and source access exist | Best candidate for Agentic Stream to prove value. | Run replay against incumbent, deterministic rules-only, then bounded reasoning; include action revalidation and total operating cost. |
| Data identity, clocks, sensor health or work-order practice are poor | New runtime may amplify data problems. | Data/process improvement may be the product or prerequisite; quantify onboarding before model work. |
| Buyer requires high-frequency or safety-rated direct control | Outside the current product boundary. | Do not pursue as v1; use a qualified control system and keep Agentic Stream supervisory if at all. |
| Team likes the replay artifact but does not put it in a release/incident gate or pay to retain it | Useful open-source feature, weak standalone offer. | Keep it as proof/distribution; do not force a separate SaaS business thesis. |

### Defensibility if a workflow passes

The integrated semantics—event-time condition history, immutable decision snapshot, bounded cognition, live policy recheck and outcome record—could reduce custom cross-system glue. This is a product hypothesis, not a moat claim. Flink/Kafka, Temporal, agent frameworks and incumbents can be composed or extended to approximate it. Go, SQLite, MIT licensing and avoiding an LLM per event do not create durable differentiation by themselves.

Potential compounding advantages would have to arise from observed use: supported connectors that remove real onboarding work; a stable, portable evidence contract; reliable operator recovery and audit workflows; reusable, permissioned domain evaluations; and a support/partner route that customers choose repeatedly. The customer must retain an exportable record and a clear exit path. Do not create lock-in as a substitute for product value.

### ExecutionRail: a separate product hypothesis from the attached discussion

The attached Open-RAIL discussion proposes an execution interface with operations such as `accept`, `cancel`, `hold`, `resume` and `observe_state`, plus a complete record from model proposal through physical observation. That is a useful adjacent question, but it combines two different product boundaries:

| Boundary | Agentic Stream's supervisory role | Executor/control role |
| --- | --- | --- |
| Evidence and decision | Preserve the Situation snapshot, admit bounded cognition, bind an Intent to its source version and expiry. | Consume a bounded job request and report progress/state as new evidence. |
| Authority | Recheck policy and approval before dispatch; use idempotency and durable outcome/reconciliation records. | Own device capability, local interlocks, safe hold/stop behavior, timing deadlines and hardware-specific execution. |
| Failure handling | Expire or reject stale work, block or reconcile unknown outcomes, explain the decision history. | Respond safely to lost connectivity, controller faults, interruption and unsafe physical state without waiting for a model or remote runtime. |

Some supervisory pieces already exist in the current design/code: Decisions and Intents bind to a Situation version, have validity/expiry checks, pass through a current policy gate, and enter durable command/reconciliation paths. See the [Decision validator](../../../internal/decisions/validator.go), [policy plane](../../../internal/policy/), [action dispatcher](../../../internal/actions/dispatcher.go), and [design boundary](../../design/TECHNICAL_DESIGN.md). The attached interface and continuous motion-control behaviors are proposals; they are not a current public ExecutionRail contract. The technical design explicitly excludes robotics, PLC and sub-millisecond control from v1.

The product options are therefore:

1. **Keep the executor external** (current design fit): integrate a qualified executor using typed, expiring, policy-checked Intents and feed receipts/observed state back as evidence. This can test the lifecycle seam without making Agentic Stream responsible for control dynamics.
2. **Improve the supervisory handoff contract** (possible future design research): specify capability declaration, temporal validity, cancellation semantics, receipt identity, reconciliation evidence and outcome provenance across adapters. This still requires real integrator and buyer evidence before changing contracts.
3. **Own a general execution rail** (separate product and design change): accept, pause/resume, interpolate/smooth and control physical actions across device classes. That expands the safety case, local runtime, actuator adapters, deployment qualification and field-support obligation. Open-RAIL's published results are narrow, and its paper notes cross-robot validation remains future work; neither it nor an Agentic Stream simulator establishes the required qualification. See [S17–S20 in the source ledger](SOURCES.md).

**Research position:** do not fold a general high-rate executor into the standalone v1 product thesis. Only reopen that boundary if named robotics/OT buyers show that they need a vendor-neutral supervised job/receipt layer, existing qualified executors cannot provide it, and they will fund the resulting field qualification. A task-level handoff may be compatible with the current product boundary; trajectory generation, smoothing and direct control are not. These are hypotheses to test, not product commitments.

## 7. Concrete validation before implementation investment

### P0 — recent-workflow interviews and artifacts

Screen SRE and industrial lanes separately with six stakeholder conversations per lane as an initial **screening sample**, not a market-size or saturation study. Include practitioners who perform the work, technical owners who maintain the stack, and at least one budget/procurement owner. For each organization, reconstruct up to its three most recent qualifying cases from records where permitted; interviewees can clarify different parts of a case. Capture actual steps, systems and people; time spent; false/missed alerts or work; outcome; current build/maintenance cost; and authority to act. If a lane remains promising, expand that lane to 8–12 total conversations distributed across at least four organizations before selecting it. These counts are a discovery discipline, not statistical proof.

Advance a lane only if at least two organizations can name a repeated decision, show permitted historical artifacts, identify an owner and describe an approval route. Record whether each prefers a standalone runtime, an extension inside its current stack, or partner delivery and why. If interviews stay at generic AI interest, or artifacts show that a simple rule/process change resolves the issue, stop that lane before building adapters.

### P1 — blinded retrospective replay comparison

Require two independent organization/trace families before treating a result as a lane-level signal. Within each, freeze the trace and labels before evaluation. Compare:

1. the current incumbent workflow and its real configuration, including native AI features already licensed or available;
2. a deterministic, rules-only Agentic Stream configuration;
3. bounded cognition on the same frozen Situation snapshots.

Include normal/no-incident periods and hard cases, not only selected failures. Inject no fabricated customer results; use only original recorded timestamps and maintain an audit of redaction and transformations. Have operators label conditions and useful actions before seeing model output where feasible.

Required measures: coverage and missing-label rate; high-severity misses; false pages/work orders per defined exposure; time to useful decision; operator review time; stale proposals; source gaps; duplicate effects; unknown outcomes; model invocations; trace-normalization time; and full deployment/TCO inputs. Report counts and uncertainty. Agree thresholds with the partner before the run. A small replay is a screening result, not evidence of safety or general performance.

#### Evaluation evidence boundary

The repository’s [`docs/eval/` package](../../eval/README.md) is explicitly **design-only, not implemented as a unified evaluation**. Its `Class D`/`Class C` labels are evaluation classes; they are separate from the A–D source-evidence labels in [the deepening plan](DEEPENING_PLAN.md).

- **Apply deterministic Class D checks separately.** For the cases exercised, select the relevant exact cells from the documented axes: event-time correctness (A2), Situation immutability/provenance (A3), determinism (A4), episode binding and budgets (A5), model/effect separation (A6), pre-dispatch policy revalidation (A7), idempotency (A8), replay isolation (A9), explainability (A10), and any applicable safety/evidence counters (X1/X2). A Class D failure invalidates Class C claims from that run. Unsupported or missing evaluation cells are `blocked`, not passes.
- **Treat bounded-cognition uplift as Class C.** Predefine scenario outcomes, grade durable Decision/Intent records, compare paired trials against the deterministic baseline, hold out a scenario family, report an interval and per-cell cost, and meet the repository design’s `k >= 3` trial rule. Measure decision quality, abstention and regret as applicable; model narration is not a grader. A two-family screen by itself does not meet this bar.
- **Keep the incumbent comparison at screening strength.** Comparing the customer's configured incumbent with Agentic Stream is necessary for product discovery, but the evaluation design lists runtime benchmarking as a separate matched-comparison study. P1 can identify a candidate gap; it cannot support a competitive superiority claim unless that comparison has its own predeclared matched protocol and evidence.

These requirements describe the evidence bar to use if this research advances. They do not mean the current checkout has already run the unified evaluation or produced capability, comparative, safety, or physical-outcome evidence.

**Go screen to P2 (direction only):** one lane shows a partner-selected, operationally meaningful improvement at the same high-severity miss tolerance, no unacceptable stale or unsafe proposal, and a measured integration/operating burden that a named buyer will accept. Two independent trace families support the direction, without being mistaken for a safety or general-performance claim. The partner agrees to a prospective, effect-disabled shadow trial and identifies who can fund it if useful.

**Stop or pivot:** native tools match the result; rules-only matches bounded cognition; labels are unusable; integration exceeds the value; operators cannot explain or trust the record; security/procurement blocks the deployment; or no budget owner can fund the work. Pivot to an incumbent extension or integrator route only if that buyer prefers it. Keep replay as an open-source capability if it is useful but does not have an independent budget.

### P2 — prospective operational shadow

Run a bounded, effect-disabled shadow trial on one repeated workflow after retrospective screening. Preserve relevant Class D checks and use the Class C protocol before making a bounded-cognition capability claim; the evaluation package is still design-only. Every candidate situation must be reviewable, including suppressed/no-op cases and missing-source states. Log operator disposition and whether it changed a real decision. Compare ongoing system cost and operator time with baseline. A request to continue is adoption evidence only: also name the operational owner and document the procurement route, without counting either as willingness to pay.

### P3 — paid commercial validation

Offer a clearly scoped paid shadow continuation or low-risk pilot to a buyer who controls a named budget. Record contract value, service obligations, integration and support effort, approval path, renewal date and the customer's success measure. Seek at least one paid pilot/committed-spend result and evidence of a paid extension, renewal or repeat engagement before treating sustainable standalone unit economics as a credible hypothesis. One buyer is not product-market fit; it only justifies the next, broader commercial test. If buyers pay only for bespoke SI work, test that delivery model and its margin explicitly.

### P4 — governed effect qualification

Only after separate release/deployment qualification and site-specific authorization should a partner test a reversible, non-safety effect such as creating a draft ticket. Verify denial, supersession/expiry, unknown-outcome recovery and independent completion evidence. Physical control remains outside the proposed v1 product.

**Evidence boundary:** the shadow and replay stages can evaluate detection, classification, lead time, operator disposition, latency and workflow cost. They do not prove avoided downtime, customer impact or repairs because no counterfactual intervention occurred. Any avoided-loss claim needs an agreed causal design and independent outcome records; report confounders and uncertainty. A small number of traces can select what to test next, not establish safety, non-inferiority or market-wide performance.

## 8. Product shape worth proving

If the workflow evidence is positive, the smallest compelling standalone product is not a general agent platform. It is a local or customer-controlled **condition supervision service** with a clear operator loop:

1. Connect one supported source or accept a validated, exportable normalized trace.
2. Define and test one Situation contract against historical data.
3. Compare proposed condition versions and decisions with the customer’s current system.
4. Run shadow mode and surface source health, completeness, correction and operator disposition.
5. Bind each diagnosis to one immutable evidence snapshot; make changed evidence cancel or expire stale work.
6. Route a typed proposal to an existing incident or maintenance system; require the customer’s authority and approvals.
7. Record the full decision, effect acknowledgement and independently observed outcome; expose replay, export, recovery and audit through supported operator tools.
8. Operate with documented install, security, backup, restore, upgrade and rollback behavior.

The defining product claim should be a measured reduction in a selected workflow’s total decision and recovery burden, at a partner-set risk tolerance. If customers instead value only the Situation library, replay or an internal component, make the product form an extension/library/partner offer rather than assuming a separate runtime budget.

## 9. Research limits and revised conclusion

This deepening remains desk research plus a read-only code/docs review. It did not interview buyers, access private incidents or asset histories, execute a side-by-side competitor trial, measure deployment effort in a customer environment, obtain quotes, prove physical effects, or test renewal. Official product docs confirm available primitives and vendor positioning; filings and public case studies are issuer-reported; neither proves Agentic Stream demand.

**Revised conclusion:** Agentic Stream is a promising runtime architecture, not yet a validated standalone product. The broad SRE-agent story now faces mature direct competition. The motor/pump story is a better-aligned demonstration but faces platform competition, integration burden and a serious low-tech/process alternative. A standalone product only becomes credible if real traces show an important cross-source decision that incumbents and simpler rules do not handle adequately, and the complete product reduces total deployment and operating cost while preserving the project's authority and replay invariants.

Proceed with focused discovery and retrospective comparisons. Do not infer product-market fit from the repository fixture, feature launches, pricing pages, company revenue, market forecasts or this architecture review. The next positive evidence must come from real workflows, real operators, an identified payer and repeated use.
