# 02 — Suite catalog

Every cell, what it proves, and its current coverage. Coverage is stated against the repository
as of 2026-09-17; a cell marked *covered* has an existing test or fixture the evaluation can
adopt, *partial* means the behavior is tested but not across the required matrix, and *missing*
means no cell exists.

The catalog is the source of truth. The runner reads it; it does not hard-code cells.

## S0 — Contract conformance (Class D)

The boundary where another repository can disagree with this one. Highest value per line,
because a break here is a cross-repo break.

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S0.1 device-wire valid frames | one byte-exact canonical example per message type (`command`, `receipt`, `result`, `state`) decodes | contract | covered — `internal/contractsv1/conformance/v1/valid` (4) |
| S0.2 device-wire invalid frames | each broken rule is rejected, one rule per fixture | contract | covered (13); grow with every new rule |
| S0.3 capability catalog | the closed route and safe-stop catalog digests to its pinned value; an out-of-catalog operation is refused | contract | covered — `thermal-capability-catalog.json` |
| S0.4 SituationSpec schema | a spec compiles to canonical JSON with a stable digest; schema violations are rejected | contract | covered — `internal/spec` |
| S0.5 worker protocol | handshake, capability scope, cancellation, and trace propagation across the UDS boundary | contract | partial — `internal/executor/conformance` |
| S0.6 executor parity | the in-process fixture and the streamed worker fixture reach the same semantic Decision for the same request | record | covered — `conformance.FixtureRequest`; extend to more semantics |

## S1 — Determinism and event-time (Class D)

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S1.1 replay determinism | three fresh runs over one golden trace produce byte-identical canonical projections for events, features, timers, watermarks, Situations, and trigger evaluations | byte | partial — `internal/replay`; the three-run projection digest is the missing piece |
| S1.2 virtual time | a shifted wall clock, and a restart, change no digest | byte | partial |
| S1.3 canonical JSON | RFC 8785 vectors, native floats, duplicate keys, malformed Unicode, negative zero, unsafe integers all follow the documented policy; every digest carries `sha256:` | contract + byte | covered — `internal/canonicaljson` |
| S1.4 duplicate and out-of-order | duplicate events are idempotent; bounded out-of-order is accepted; unbounded is quarantined, not guessed | record | partial — `trace-reboot-backlog.jsonl` |
| S1.5 late correction | a late correction republishes a new Situation version and never mutates the old one | record | partial |
| S1.6 idle and rejoin | an idle partition does not stall the watermark; a rejoining partition catches up deterministically | record | partial |
| S1.7 missing heartbeat | an absent heartbeat fires the declared timer exactly once | record | covered — `trace-heartbeat.jsonl` |
| S1.8 partition restart mid-stream | restart preserves watermark, keyed state, and situation version ordering | record | partial |
| S1.9 quarantine truthfulness | a quarantined input is durably visible and never silently dropped | record | partial |

Golden traces to sweep: `examples/predictive-maintenance/testdata/trace-{heartbeat,opening,watch}.jsonl`
and `examples/thermal-chamber/testdata/trace-{quiet,ambient-tracking,reboot-backlog,invalid-quality,opening}.jsonl`.

## S2 — Situation semantics (Class D)

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S2.1 immutability | a published version cannot be updated; only a superseding version is published | record | partial |
| S2.2 version ordering | versions are monotonic per `(entity, spec)` with no gaps | record | partial |
| S2.3 provenance | every field and every trigger decision resolves to durable inputs | record | partial — `runartifact` exports the inputs; the per-field map is the gap |

## S3 — Bounded cognition (Class D)

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S3.1 snapshot binding | an episode binds exactly one immutable snapshot; a superseded snapshot cannot be substituted | record | partial |
| S3.2 budget matrix | each of deadline, model calls, input tokens, output tokens, tool calls, tool bytes, retries, and cost is independently capped, and exceeding one stops the episode with a typed reason | record + counter | partial — `internal/control` (cost files), `internal/episodes` |
| S3.3 no model per event | an event that does not warrant reasoning starts no episode | record | partial |
| S3.4 lifecycle machines | episode aggregate, worker attempt, Decision validation, and outcome verification are separate durable state machines | record | partial |
| S3.5 supersession and staleness | cancellation, supersession, coalescing, expiration, abandonment, and stale output are durable, explainable, and idempotent | record | partial |
| S3.6 worker identity | the identity is `(episode_id, attempt_id, fence)`; a same-snapshot retry cannot make an old attempt's Decision acceptable | record | partial |

## S4 — Governed effects (Class D, hard-zero)

The suite that can fail the run by itself.

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S4.1 capability denial | worker and model have no effector, credential, filesystem, shell, or MCP capability | contract | partial — `internal/evidence/capability.go` |
| S4.2 revalidation matrix | for each of freshness, preconditions, risk, approval, quota, and interlock, a stale or forged Intent is refused at dispatch time | record | partial — `internal/policy`, `internal/interlock` |
| S4.3 atomic outbox | a Command is created through the outbox with a stable idempotency key, or not at all | record | partial — `internal/actions` |
| S4.4 dispatch fault matrix | see below | record + counter | partial — device codec and serial tests exist; the matrix is the gap |
| S4.5 unknown outcome | an ambiguous dispatch enters reconciliation and is never blindly retried | record | covered — `soak` test |
| S4.6 safe stop | `safe_stop` reaches the safe state within the declared deadline | record + counter | partial — `serial_effector_test`, `emulator_effector_test` |
| S4.7 device operations | `set_led` and `set_pwm_lease` are bounded by the catalog; a lease expires and de-energizes | contract + record | partial |

**S4.4 dispatch fault matrix** — one cell per `(boundary × fault)`. Each asserts an invariant,
not a code path:

| Fault | Expected property |
| --- | --- |
| process dies before dispatch | no Command row, or a Command that never dispatched — no effect |
| process dies after dispatch, before receipt | outcome `unknown`; reconciliation; never a blind retry |
| receipt lost, result present | the result is authoritative; no duplicate dispatch |
| result lost after execution | `unknown`; safe-state or reconcile; no second accepted effect |
| device reboot mid-command (`boot_id` changes) | the old Command is refused as stale; a new boot cannot accept it |
| duplicate command delivery | one accepted effect (idempotency key) |
| output stuck energized | safe-state deadline fires; counter increments; verdict fails |
| clock jump | lease expiry and deadlines use the monotonic clock; no double expiry |
| stale `policy_digest` / `capability_digest` | the device or the action plane refuses the Command |

## S5 — Replay isolation and shadow (Class D, with a Class C shadow)

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S5.1 effect-free replay | replay loads no production effector, credential, outbox, or token | record + contract | partial — `internal/replay` |
| S5.2 no counterfactual mode | replay refuses the removed `counterfactual` mode | contract | done |
| S5.3 shadow parity | a new model or prompt runs against the same trace in effect-disabled shadow mode and produces no effect | record | partial |

## S6 — Explainability (Class D)

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S6.1 disposition completeness | every opportunity is durably admitted, deferred, coalesced, rejected, canceled, or expired, each with a reason | record | partial |
| S6.2 outcome explanation | every action outcome — executed, rejected, expired, superseded, safe-state, unknown — is explainable from records | record | partial |
| S6.3 artifact verification | `verify-run` passes on the produced artifact and detects a tampered JSONL, a stale command digest, and a stale safety-event digest | byte | covered — `runartifact.Verify` |

## S7 — Capability (Class C, off-CI)

Scenario families: `predictive-maintenance` and `thermal-chamber`. Each scenario declares a
definition of done; the episode's Decision and Intent are graded against it.

| Cell | Asserts | Oracle | Coverage |
| --- | --- | --- | --- |
| S7.1 decision quality | the proposed Intent matches the scenario's definition of done | record + DoD | missing |
| S7.2 off-catalog fail-closed | an Intent outside the declared catalog never dispatches | record | partial |
| S7.3 abstention quality | on cells where abstaining is correct, the runtime abstains; on cells where acting is correct, it does not | record + statistics | missing |
| S7.4 regret against the baseline | paired against `replay.DeterministicBaseline`, regret and win/loss clear a stated minimum effect | statistics | missing |
| S7.5 holdout | the family is held out from any prompt or catalog tuning and recorded by digest | contract | missing |
| S7.6 cost per cell | model calls, tokens, tool bytes, and wall time are reported per cell, not aggregated away | counter | partial — `internal/control` (cost files) |

`k >= 3` trials per scenario, paired, with an interval that clears zero. Fixtures and
scripted providers never enter S7.

## S8 — Operational qualification (Class D; currently release blockers)

These are named because they are release blockers in
[`documentation/governance/release-status.json`](../../documentation/governance/release-status.json);
today they have no cell.

| Cell | Asserts | Coverage |
| --- | --- | --- |
| S8.1 soak | an 8-hour fault soak produces a `soak.Report` with zero zero-tolerance counters and complete evidence | missing (`internal/soak` computes it; no soak run) |
| S8.2 backup and restore | a backup restores to a verifiable, equivalent artifact | missing |
| S8.3 disk full | a full disk stops safely without a false verified success | missing |
| S8.4 unclean shutdown | a `kill -9` mid-dispatch reconciles exactly once | partial — covered at unit level, not at process level |

## Coverage gaps, stated plainly

- The deterministic suites (S0–S6) are largely *tested* but not *aggregated*: no single verdict, no coverage map, no artifact a reviewer can verify end to end.
- S4.4 is the largest real gap: the dispatch fault matrix is the difference between "idempotent by construction" and "proven idempotent under fault".
- S7 does not exist as a harness. Without it there is no capability claim, only a plumbing claim.
- S8 is the release blocker; `soak` is the verdict engine waiting for a soak run.
