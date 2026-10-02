# 05 · Sources

External sources were checked on **2026-10-02** through web search and page fetches.
Vendor and secondary sources are labelled as such. Product status changes quickly, so
re-verify before quoting any of these outward.

| ID | Source | Type | Used for |
| --- | --- | --- | --- |
| L1 | [Apache Flink Agents 0.3.0 release announcement (2026-06-19)](https://flink.apache.org/2026/06/19/apache-flink-agents-0.3.0-release-announcement/) | Primary (project) | 0.3.0 features: YAML API, Skills, Mem0 memory, durable reconciler, Fluss store, EventLog UI |
| L2 | [Apache Flink Agents 0.2.0 release announcement (2026-02-06)](https://flink.apache.org/2026/02/06/apache-flink-agents-0.2.0-release-announcement/) | Primary (project) | Release cadence |
| L3 | [Ververica: Alibaba Cloud, Ververica, Confluent, LinkedIn on streaming AI agents with Flink](https://ververica.com/blog/alibaba-cloud-ververica-confluent-and-linkedin-join-forces-on-the-streaming-ai-agents-innovation-with-apache-flink) | Vendor | Backers of Flink Agents |
| L4 | [IT Brief: Confluent launches Streaming Agents](https://itbrief.co.uk/story/confluent-launches-streaming-agents-to-scale-real-time-ai) | Secondary | Open-preview status |
| L5 | [AAIF: MCP 2026-07-28, what's changing and how to migrate](https://aaif.io/blog/mcp-2026-07-28-whats-changing-and-how-to-migrate) | Primary-adjacent (foundation) | Spec revision scope |
| L6 | [MCP Tasks in 2026: long-running, resumable agent tools](https://ecorpit.com/mcp-tasks-long-running-async-agent-tools-build-guide-2026/) | Secondary | Tasks lifecycle (`tasks/get`, `tasks/update`, `tasks/cancel`) |
| L7 | [Microsoft Open Source blog: Introducing the Agent Governance Toolkit (2026-04-02)](https://opensource.microsoft.com/blog/2026/04/02/introducing-the-agent-governance-toolkit/) | Primary (vendor) | Runtime policy interception; YAML/Rego/Cedar |
| L8 | [OWASP Top 10 for Agentic Applications for 2026](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026) | Primary (standards body) | ASI01–ASI10 risk taxonomy |
| L9 | [Help Net Security: What the EU AI Act requires for AI agent logging (2026-04-16)](https://www.helpnetsecurity.com/2026/04/16/eu-ai-act-logging-requirements/) and [Art. 12 text](https://artificialintelligenceact.eu/?p=2967) | Secondary and primary | Art. 12 categories; 2026-08-02 date; Omnibus status |
| L10 | [HSToday: CISA and partners joint guide on secure integration of AI in OT](https://www.hstoday.us/subject-matter-areas/cybersecurity/cisa-and-partners-issue-joint-guide-to-advance-secure-integration-of-artificial-intelligence-in-operational-technology/) | Secondary (reporting a government guide) | Four principles |
| L11 | [Dash0: OpenTelemetry GenAI semantic conventions explained (updated 2026-09-14)](https://www.dash0.com/knowledge/opentelemetry-genai-semantic-conventions-explained) | Secondary | Development stability; June 2026 repository move |
| L12 | [TigerBeetle VOPR docs](https://docs.tigerbeetle.com/about/vopr) | Primary (project) | DST practice |
| L13 | [detsim Go package](https://pkg.go.dev/github.com/arshnah/detsim) | Primary (code) | Go DST prior art (Aug 2026). Evaluate maturity before any use |
| L14 | [Software Toolbox: What is Sparkplug B](https://softwaretoolbox.com/resources/what-is-sparkplug-b) | Vendor | Birth/death certificates; typed payloads |
| L15 | [Litmus: How to build a Unified Namespace](https://litmus.io/blog/how-to-build-a-unified-namespace-architecture-timeline-and-failure-modes) | Vendor | UNS architecture and failure modes |
| L16 | [Emerson: Alarm management by the numbers](https://d1-live.emerson.com/documents/automation/article-alarm-management-by-numbers-deltav-en-38292.pdf) and [IChemE: alarm rationalisation metrics](https://www.icheme.org/media/17392/lc-0151_21-lead-process-safety-metrics-alarm-rationalisation-final.pdf) | Vendor and professional body | ISA-18.2 / EEMUA 191 flood and steady-state thresholds |
| L17 | [ICML 2026: Are time series foundation models ready for oil and gas drilling anomaly detection?](https://icml.cc/virtual/2026/71509) | Peer-reviewed venue | TSFM anomaly-detection evidence (domain-specific) |
| L18 | [Forkast: Temporal funding](https://forkast.news/temporal-wants-12-billion-to-keep-your-agents-from-crashing-mid-task/) | Secondary | Temporal Series D (Feb 2026) |
| L19 | [wazero](https://wazero.io/docs/) | Primary (project) | Pure-Go WASM runtime for B7 |
| L20 | [Litestream (Go package reference)](https://pkg.go.dev/github.com/ncruces/litestream) | Primary (code) | WAL shipping for F3 |

## Internal sources

- This checkout: `internal/spec/schema.json` (reducer and operator enums),
  `internal/ingress/live_socket.go` (no producer ack), `internal/actions/dispatcher.go`
  (`reconciliation_version`), `documentation/overview/{status,limitations}.md`,
  `documentation/governance/release-status.json`, and `docs/eval/`.
- The [standalone product research (2026-09-24)](../standalone-product-2026/SOURCES.md)
  for the competitor and industrial platform sources not repeated here.
- The companion research lab (not linked, by repository policy): integration rounds 1–5
  reports and assessments, the stream code review, the real-world sensor current-status
  assessment, the voice-AI stream-fit study (2026-10-01), the AI-coworker decision brief
  (2026-10-01), the streams-simulator design, and the planning-context-compiler one-pager.

## Evidence limits

No competitor product was run. No customer was interviewed. No benchmark was executed for
this package. Lab-reported results were not re-run, except the specific code checks marked
**verified** in [01](01-STARTING-POINT.md). Secondary sources were used where primary
pages were not fetched. Treat their specifics (dates, counts, status labels) as needing
re-verification before any external use.
