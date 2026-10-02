# 06 · Observability ingress: Prometheus, Kubernetes, Grafana, and logs

Status: research proposal. Implementation would require the proposed
[ADR-015](../../design/DECISIONS.md#adr-015-proposed-external-ingress-bridges-over-an-acknowledged-socket)
to be accepted, plus opportunity D1 (durable ingress acknowledgements) from
[03-OPPORTUNITIES.md](03-OPPORTUNITIES.md). HTTP and broker ingress remain *deferred*
in the current design. Nothing here changes that on its own.

## The position in one paragraph

Do not build one connector per tool inside the runtime, and do not stream raw metrics or
raw log lines into the deterministic plane. Use the **OpenTelemetry Collector** as the
universal front end, and add **one small external bridge** that maps its output (and
Alertmanager or Grafana webhooks) into the normalized envelope, sent over the live socket
with acknowledgements. Feed Situations with *signals*: alerts, rollouts, Kubernetes
warnings, and structured log derivatives. Let episodes *pull* detail (PromQL, LogQL,
Kubernetes reads) through recorded evidence tools.

## Why this shape

| Constraint | Consequence |
| --- | --- |
| **Volume.** A Prometheus scrape firehose (thousands of series every 15–60 s) would dominate single-node SQLite and the scheduler, and the ceiling is not yet measured (A4). | Situations change on meaningful signals. Raw series stay in Prometheus and are queried on demand. |
| **Invariant 1:** raw events are evidence, never instructions. Log lines are attacker-controllable free text. | Logs enter as counts, signatures, and pattern IDs computed upstream. Free text reaches an episode only as bounded, untrusted evidence through evidence tools, never in a trigger or CEL expression. |
| **Invariants 4 and 9:** deterministic replay with no external effects. | Pulled query results are recorded in the evidence ledger, and replay uses the recordings. Replay never re-queries Prometheus or Loki. |
| **ADR-004 and ADR-012:** single node, no broker in core. | Collector and bridge are separate processes. The core gains no HTTP, Kafka, or OTLP listener. |
| **Domain data in JSON registries** (`AGENTS.md`) | New event types go in `internal/eventschema/registry_data.json`. Label-to-entity mapping lives in bridge configuration, not Go literals. |

## Topology

```text
Prometheus ──┐                         ┌─ Alertmanager / Grafana alert webhooks
k8s API ─────┤                         │
log files ───┼─► OpenTelemetry Collector ─► bridge (external process) ─► live UDS ingress
Loki/Elastic ┘   (receivers, filtering,     │  normalize envelope,        (D1: per-event
                  down-sampling, log→metric) │  entity mapping, event IDs   commit ack)
                                             └─ spool; delete only after ack
                                                                 │
                     episodes ◄── recorded evidence tools ◄──────┘
                     (PromQL / LogQL / k8s reads, time-bounded to the snapshot)
```

The collector already provides the receivers: Prometheus scrape, Kubernetes events and
objects, Kubernetes cluster metrics, and file logs. The bridge is the only new code. It
is small and stateless apart from its spool, and it is testable in isolation.

## Source-by-source mapping

### Prometheus and Alertmanager

| Input | Event type (proposed) | Notes |
| --- | --- | --- |
| Alertmanager webhook (firing/resolved) | `obs.alert.changed` | `fingerprint` plus `startsAt` gives a stable event ID. Resolved alerts are first-class events, not deletions. |
| Recording-rule outputs for a short allow-list (error ratio, latency p99, saturation) | `obs.metric.sampled` | Down-sample in the collector. Allow-list the series. Counter resets are handled upstream by using `rate()` or `increase()` recording rules, never raw counters. |
| Scrape failure / `up == 0` / staleness | `obs.source.health` | Maps to source-health and gap records. This is the absence signal most consumers ignore. |

### Kubernetes

| Input | Event type (proposed) | Notes |
| --- | --- | --- |
| Warning events (CrashLoopBackOff, OOMKilled, FailedScheduling, BackOff) | `k8s.warning.observed` | Kubernetes updates events in place with a `count` field. Use `(uid, count)` as identity so repeats are corrections or increments, not new incidents. |
| Rollouts and image changes (Deployment/StatefulSet generation, ReplicaSet creation) | `k8s.rollout.changed` | **The most valuable SRE signal**: change correlation. It carries the old and new image digest and revision. |
| HPA scaling, node conditions (NotReady, pressure) | `k8s.capacity.changed` | Low volume, high meaning. |
| Pod create and delete | *(not streamed by default)* | Use only if a spec needs entity churn. Otherwise it creates cardinality without value. |

### Logs

| Input | Event type (proposed) | Notes |
| --- | --- | --- |
| Error and warning counts per service per minute | `log.rate.sampled` | Computed in the collector (log-to-metric). |
| New or rare error signature (normalized template hash) | `log.signature.observed` | The template hash and a **redacted, length-bounded** exemplar reference. No raw line in the event. |
| Raw lines | *(never streamed)* | Available to episodes only through a bounded LogQL or Elastic evidence tool. |

### Grafana

Grafana is a presentation and alerting layer, not a data source. Its alert webhooks are
handled exactly like Alertmanager's. Its useful role is as an **effector target**:
writing Situation annotations onto dashboards (see effectors below).

## Identity, partitioning, and time

- **Entity = service or workload, not pod.** The partition key is
  `(tenant, cluster, namespace, workload)`. Pod and container labels go into `data`.
  Partitioning by pod would multiply partitions with every restart.
- **The label mapping is configuration**, versioned and digested with the bridge config,
  so a replay can state which mapping produced which entities.
- **Event time** is the sample timestamp, the alert `startsAt`/`endsAt`, or the
  Kubernetes `lastTimestamp`/`eventTime`. **Ingestion time** is set by the bridge. Use a
  watermark per source with an explicit `maxOutOfOrderness` sized to the observed scrape
  and evaluation lag, and record scrape lag as quality metadata.
- **Late and corrected data:** Kubernetes event count updates and Alertmanager resolve
  events use the existing correction and reconsideration path. Opportunity A1
  (`prefer_corrected`) matters here for any running count.

## Episode evidence tools (pull side)

These tools are called by Tamoz during an episode through the evidence socket (see T2).
Tamoz never receives Prometheus, Loki, or Kubernetes credentials.

Three tools, each time-bounded to the episode's snapshot window, with row and byte
limits, and recorded in the existing evidence-call ledger:

| Tool | Bound | Recording |
| --- | --- | --- |
| `promql_range` | Query from an allow-listed template with parameters bound to entity labels; `[from, until]` ≤ snapshot horizon; max points | Response body hash and body stored for recorded replay |
| `logql_query` | Template with entity labels; max rows and bytes; results marked untrusted | Same |
| `k8s_read` | Read-only, allow-listed kinds in the entity's namespace | Same |

Templates are spec or registry data, never model-authored strings. The model chooses a
template ID and an allowed time range, not a query text. Credentials stay in the evidence
server, never in the worker or model context (ADR-008, invariant 6).

## Effectors (in order of risk)

| Effector | Risk | Policy |
| --- | --- | --- |
| Grafana annotation / incident-tool note | R0–R1 | Automatic. Idempotency key = intent ID. |
| Open or annotate an incident (PagerDuty, incident.io event API) | R1 | Automatic or approval, with dedup key = Situation ID |
| Alertmanager silence | R2 | Approval. Must expire. The intent carries the matchers, verified by policy against the Situation entity. |
| Kubernetes rollback or scale | R2–R3 | Approval plus live policy recheck. Reconcile by reading rollout status. Unknown outcome blocks retry. |

Each needs the unknown-outcome reconciliation documented in D5.

## Phased plan

| Phase | Work | Runtime change | Exit evidence |
| --- | --- | --- | --- |
| **0. Offline proof** | Export one or two real incidents: Alertmanager history, Prometheus range queries, `kubectl get events -o json`, deploy history. Convert them to normalized JSONL with a script and replay against a `k8s-workload` SituationSpec. Compare the Situation history with what responders actually did. | **None** | One Situation per real incident with no duplicates; rollout correlation visible in `explain`; spec `test` expectations committed |
| **1. Live shadow** | Collector, bridge, and live UDS with D1 acks. Effects disabled (shadow dispatch). Run against your own cluster as dogfooding. | D1 ack contract | A week of shadow with zero lost and zero duplicated events across restarts; admission rate per service-day; ISA-18.2-style operator load (B6) |
| **2. Evidence tools** | `promql_range`, `logql_query`, `k8s_read`, recorded | New evidence tools | Recorded replay of an episode reproduces the identical Decision |
| **3. Low-risk effects** | Grafana annotations and incident notes | Effector adapters | Idempotent under kill/restart; unknown outcomes reconciled |
| **4. Gated effects** | Silence and rollback behind approval | Effector adapters | Stale-intent rejection demonstrated when a new rollout or resolve arrives while approval is pending |

## Risks and honest limits

- **A crowded lane.** PagerDuty, Datadog Bits Investigation, CloudWatch Investigations,
  and incident.io already do alert-to-investigation (see the
  [deep dive](../standalone-product-2026/DEEP_DIVE.md#service-incidents)). Your own
  cluster is a cheap, real-data testbed for the thesis. It is not market proof.
- **Cardinality creep.** Every allow-listed series is a cost. Start with fewer than 10
  signals per service.
- **Clock quality.** Scrape timestamps, Kubernetes event timestamps, and log timestamps
  come from different clocks. Measure skew in Phase 0 before trusting `on_time` gates.
- **Vendor log APIs differ.** Start with one log backend.
- **The Collector is a dependency of the deployment, not the module.** Keep it out of
  `go.mod`. The bridge can be a separate small Go command or a configuration-only
  collector pipeline with a minimal exporter.

## Decision requested

Accept or reject [ADR-015 (proposed)](../../design/DECISIONS.md#adr-015-proposed-external-ingress-bridges-over-an-acknowledged-socket).
If accepted, the first implementation task is D1 (ingress acks). Phase 0 can start
immediately because it needs no runtime change.
