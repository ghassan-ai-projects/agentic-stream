# 01 · Starting point

Where the project actually stands on 2026-10-02, combining this checkout with the companion
research lab. **Verified** = re-checked in this checkout; **lab-reported** = taken from
integration round reports and not re-run here.

## What is genuinely strong

| Strength | Evidence | Why it matters for "next level" |
| --- | --- | --- |
| A complete deterministic core | ~26k lines of non-test Go, ~20k lines of tests, 31 migrations (verified); 10 release-blocking invariants; deterministic, recorded, shadow, and counterfactual replay modes ([status](../../../documentation/overview/status.md)) | Few agent runtimes can replay a decision byte-for-byte. Everything below builds on this. |
| Authority separation that already works end to end | The model proposes typed Intents; policy rechecks them before dispatch; an outbox gives idempotency; unknown outcomes are recorded instead of retried | This is exactly what 2026 governance frameworks now ask for (see [02](02-LANDSCAPE-2026.md)). |
| Proven integration with an external reasoner | Lab rounds 1–3 joined three repositories: stream, Tamoz reasoner, and simulator. Results included a reconciled outcome admitted as an "observed" Experience, a real ed25519 approval round trip, and recall on a second occurrence (lab-reported) | The Situation seam is real, not theoretical. |
| Gross-late correction machinery | Round 5 cold-chain: an 840-reading store-and-forward burst was admitted, producing 840 corrected versions, 606 reconsiderations, and a correct compensating episode (lab-reported) | The hardest Q1 temporal case works mechanically. |
| A path to physical hardware | An ELEGOO Mega and DHT11 produced 8 temperature and 8 humidity events with zero quarantine through live ingress. A bounded LED command and `safe_stop` completed through a device-reported receipt and result (lab-reported; candidate evidence, not an independently observed physical effect) | A rare, demonstrable proof surface for "agents acting on the physical world, safely". |
| An evaluation design already written | [`docs/eval/`](../../eval/README.md) separates Class D (deterministic) from Class C (model) checks, uses claim tiers `plumbing` → `device_contract` → `physical`, and forbids averaging the two classes | An unusually rigorous evaluation philosophy. It is not implemented yet. |

## Open findings that block "next level"

| ID | Finding | Status in this checkout | Consequence |
| --- | --- | --- | --- |
| **F-1** | A late correction cannot move a windowed cumulative or extremum feature, because the only numeric reducer is `latest_event_time`, and a corrected value has an *older* event time | **Verified open.** `reducer.strategy` enum is `latest_event_time` and `set_union` only ([schema](../../../internal/spec/schema.json)) | Any running total, count, or max fed by late data silently stays stale. Lab-reported to affect the logistics (`delay_total_min`), auth/EDR (co-occurrence counts), and agent-fleet (token totals) starters as well. |
| **F-2** | `early_and_close` windows always emit `provisional`, and only heartbeat features reach `on_time`. R2 intents therefore bind provisional versions unless a trace ends on a beat | Lab-reported; not re-tested | Gated intents (`completeness: on_time`) are hard to author correctly. |
| **F-3** | No categorical or string "latest" operator, so "silence is normal unless due" needs a physical proxy | **Verified.** The operator `aggregate` enum is numeric only | Mode- and state-dependent logic (operating mode, power source, door state) is awkward to express. |
| **F-4** | Heartbeat timers run on wall-clock time only; `RunTimerLoop` was reported as dead code | `RunTimerLoop` no longer exists (verified). Whether heartbeat timers are now fully on virtual time in live mode needs re-verification | If still true, this breaks the determinism story for absence detection in live mode. |
| AS-1 | `reconciliation_version` was hard-coded to 1 | **Verified fixed.** The dispatcher now passes a computed `reconciliationVersion` | Closed. |
| Scale | The greenhouse 8-bay spec slowed per-event processing past the 5-minute heartbeat window and produced stale intents. The Round 4 BESS ceiling test was scaffolded but never run (lab-reported) | No published capacity number | There is no answer to "how many entities does one node govern?" Buyers ask this first. |
| Ingress durability | The live UDS source validates and quarantines but sends no per-event commit acknowledgement to the producer | **Verified.** No write-back path in `live_socket.go` | A successful socket write is not durable admission. Producers cannot safely delete from their spool. |
| Operator surface | Approval resolution, quarantine redrive, and unknown-outcome reconciliation exist internally with no public CLI or API | Verified via [limitations](../../../documentation/overview/limitations.md) | Pilots need unsafe database access or custom tooling. |
| Policy modes | The spec accepts `automatic`, `approval`, `deny`, and `simulate`, but runtime enforcement covers only part of this set | Verified via limitations | A schema-visible control that does nothing fully is a trust hazard. |
| Status drift | Live UDS ingress and the emulator/physical effect profiles exist in code, while public status still describes mostly file and simulated paths ([prior study](../standalone-product-2026/FINDINGS.md)) | Still present (release manifest dated 2026-08-17) | Outward claims lag reality in both directions. |

## Pattern in the lab history

Each integration round fixed a large batch of seam defects: 18 in Round 1, 15 in Round 2,
and 23 in Round 3 (lab-reported). Round 5 then needed nine live spec rewrites against a
wire shape that had moved since the starter was written. Two lessons follow:

1. **The runtime is ahead of its tooling.** Engine behavior is right far more often than
   specs written against it. Better authoring feedback is worth more than new engine
   features.
2. **Cross-repo contracts drift.** Capability catalogs differed between the stream and the
   simulator outside the thermal slice, and starters went stale. Pinned, published
   contract fixtures and conformance kits pay for themselves.

## Bottom line

Agentic Stream is a correct, unusually well-governed core with **three missing layers**:
proof at scale (capacity, crash, and soak evidence), **authoring ergonomics**, and an
**operator- and integrator-facing surface**. The next level is mostly about those three
layers, plus a small, targeted expansion of what a SituationSpec can express.
