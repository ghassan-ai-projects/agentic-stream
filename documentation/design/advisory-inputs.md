# Advisory inputs

Status: built (A3 of the streams-simulator belt-conveyor M4 plan, story S5b).

## Context

A Situation's completeness is the weakest of its inputs' latest completeness, and the policy denies a consequential
(R2) intent while it is `provisional` or `uncertain`. A `missing_heartbeat` operator reports a lost input as
`uncertain`, so on a line that is one entity with many readings, the loss of one reading denies every R2 step on the
line, also the steps that do not rest on that reading. Without a heartbeat the loss is not seen at all: a window
operator emits only when its input arrives, so the lost input's features keep their last values, fully counted, and
read as a healthy, unchanging reading.

## Decision

An input declares the role its completeness plays:

```yaml
inputs:
  - name: drive3_vibration
    completenessRole: advisory   # required (the default) or advisory
operators:
  - name: drive3_vibration_lost
    kind: missing_heartbeat
    inputs: [drive3_vibration]
    duration: 5m
    output: drive3_vibration_lost_5m
```

| Question | Answer |
| --- | --- |
| Grammar | `completenessRole` on an input, `required` or `advisory`; absent means required. An advisory input needs a `missing_heartbeat` operator reading only it (a compile error otherwise): its `duration` says when the input is stale. |
| Loss in event time | A heartbeat over an advisory input is also judged in event time: when an event of another input of the same entity moves the partition watermark to at least the input's last event time plus `duration`, the operator emits its missing feature once (value `true`, event time last + `duration`). When the input returns, its own event emits the present feature as before. The processing-time timer still fires too. A required input keeps processing time only, so existing specs replay unchanged. |
| Fact, not uncertainty | A feature of a heartbeat over an advisory input never enters the Situation's completeness. It sets the fact `input_stale.<input>` (`true` lost, `false` present), and its flip publishes a version, as a completeness flip does. CEL reads `features.input_stale.<input>` (false until the loss is seen). The reasoner sees the fact in the snapshot. |
| Digests | The field is omitted when absent, so a spec without it compiles to the digest it had (pinned for every shipped spec in `cmd/agentic-stream/experiment_spec_digests_test.go`). |
| Layers | `internal/spec` (grammar, compile check, the fact name), `internal/operators` (the event-time judgement, a `stale` flag in the heartbeat state), `internal/situations` (fact and version). No table, migration, proto or import edge. |

What a spec gains is a principled statement of which evidence a decision rests on: every rule that reads an advisory
input must say what it does when the input is stale, through `features.input_stale.<input>`, and the line's other
steps keep their R2 route.

## Limits

- A required input's loss is still judged in processing time; on a barrier-paced twin that is wall time, hours of
  plant time. Judging it in event time too would move existing replays and is left open.
- The judgement needs another input of the same entity to move the watermark; a whole entity going silent is the
  processing-time timer's case.

## Next reads

- [Stream processing](stream-processing.md)
- [Cognition](cognition.md)
