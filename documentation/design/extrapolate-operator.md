# Extrapolate operator

Status: accepted design (A5, decision O31 of the streams-simulator belt-conveyor M4 plan). The calibration below was
measured on streams-simulator's S1-B and S1-A do-nothing runs (10 seed rows each). It departs from O31, which names a
linear fit: the owner approved (2026-11-09) a `quadratic` fit beside the linear default, and the S1 spec's use of it.

## Context

A Situation can say that a quantity rises; it cannot yet say when the rise reaches a limit. A plan that must act a
fixed time before a protection trips (stop a drive 1.5 h before its overtemperature trip, and not earlier, because
every hour of early isolation costs production) needs that time as a feature, and a consumer graded on prognosis
(streams-simulator consumer-verdict v0.1, `predicted_failure_at`) needs it as an instant.

## Decision

A new operator kind, `extrapolate`: over the samples of one input's declared window, fit the quantity against event
time and publish the time until the fit reaches a declared limit.

```yaml
windows:
  - {name: trend_60m, kind: sliding, size: 60m, slide: 1m, emit: on_close}
operators:
  - name: drive3_winding_to_trip
    kind: extrapolate
    inputs: [drive3_winding]
    field: data.celsius
    window: trend_60m
    output: drive3_winding_to_trip_s
    unit: second
    configuration:
      limit: 150          # the protection's own threshold, in the field's unit
      fit: quadratic      # linear (default) or quadratic
      minPoints: 324      # 90 % of the window's expected readings
      maxHorizon: 6h      # a crossing further out is no prediction
```

| Question | Answer |
| --- | --- |
| Inputs | One direct input and one numeric field, one sliding or tumbling window, as `slope` takes them. Configuration: `limit` (number, required), `fit` (`linear` or `quadratic`), `minPoints` (integer, at least 2 for linear and 3 for quadratic), `maxHorizon` (duration, optional). |
| Fit | Least squares over the window's samples, x in hours from the window end. `linear`: the line `a + b·x`; the time to the limit is `(limit − a) / b`, which is O31's `(limit − level) / slope` with the level taken on the fitted line at the window end (not the window mean, which sits half a window back). `quadratic`: `a + b·x + c·x²`; the time to the limit is the smallest root `x ≥ 0` of `c·x² + b·x + a − limit`. |
| Output | Seconds from the window end to the predicted crossing, a number, `unit: second`, at least 1 when valid (a fit already at or past the limit publishes 1). Seconds, not an instant: CEL has no event-time clock, so a transition reads `features.x > 0.0 && features.x <= 5580.0` directly. |
| Validity | Valid only when the window holds at least `minPoints` samples, the fitted value rises at the window end (`b > 0`), a crossing exists ahead (quadratic: a real root `≥ 0`), and it lies within `maxHorizon`. Otherwise the operator publishes 0. Zero is the flag: an operator output with no value yet already reads 0, so "no value" and "no valid prediction" read alike, and every condition guards `> 0`. A spec never needs a second output for the flag. |
| Emission and completeness | As the window operators: one feature per emission mode of the window (`on_close` for anything an R2 step depends on), `corrected` on an allowed late sample, the boot fence of every operator. |
| State, determinism, replay | No new state: the samples are the window state `slope` already keeps, sorted by event time and event id. The sums are taken in that order with the existing `roundedProduct`, the 3×3 normal equations are solved by Cramer's rule in a fixed order, so a replay of the same events gives bit-identical values; no wall clock, map order or randomness enters. The feature digest and the situation digest cover the value like any other feature. |
| Layer and package | `internal/operators/internal/domain` (pure): a new `extrapolate.go` with the fit and the crossing, dispatched from `applyOperator` through the window path with the value function replaced. `internal/spec/internal/domain`: the kind in `schema.json` and a compile check of `configuration` (limit present, fit known, minPoints within the fit's minimum, window present). No new package, table, migration, import edge or proto change. |
| Prediction instant | The campaign verdict writer (`internal/testsupport/campaign`) turns the fact into `predicted_failure_at` = the situation version's event horizon + the fact's seconds, on each detection whose label the test's `Subjects` maps to a prediction fact (data in the test, no names in the binary). The fact's window end is at most one slide before the horizon, so the instant is late by at most one slide (60 s here). To give the grader a track (prognostic horizon, α-λ), the writer adds the prediction of later situation versions as further detections of the same entity and label, at most one per 10 min of event time; streams-simulator grades a detection's latency and label from the earliest one, so the extra rows change neither. |

Tests, at the layer each proves: the operators domain (an independent oracle: a hand-computed straight line, a
hand-computed parabola, a noisy case whose expected values are written in the table, and each invalid case); the
spec compiler (configuration errors); an engine replay that publishes the same values twice; the verdict writer
(`predicted_failure_at` written and valid against streams-simulator's schema).

## Which form: the calibration

The degradation accelerates. A dry bearing's damage grows linearly in time at full feed (lube × load³ is constant
there), its heat as damage cubed and its vibration as damage to the fourth, and the winding follows its heat through a
15 min lag; the protection trips at 150 °C. D-3 do-nothing trips on S1-B at 8.01-10.59 h (onset 0.82-1.74 h), on
S1-A at 8.19-9.31 h.

Real lead at the first minute the fit predicts the trip within 1.5 h (hours, min-max over the 10 rows):

| Quantity and fit | 15 min | 30 min | 60 min |
| --- | --- | --- | --- |
| D-3 winding to 150 °C, linear (S1-B) | 1.16-1.23 | 1.11-1.18 | 1.01-1.08 |
| D-3 winding residual to 150 °C − peer, linear (S1-B) | 1.14-1.21 | 1.07-1.15 | 0.97-1.06 |
| D-3 vibration to 5.35 mm/s (its lowest value at a trip), linear (S1-B) | 1.34-1.46 | 1.29-1.38 | 1.19-1.28 |
| D-3 winding to 150 °C, quadratic (S1-B) | 1.66-2.22 (noise) | 1.39-1.61 | **1.42-1.47** |
| D-3 winding to 150 °C, quadratic (S1-A) | 1.42-1.64 | 1.41-1.46 | **1.40-1.44** |
| D-3 winding to 150 °C, linear (S1-A) | 1.11-1.18 | 1.07-1.12 | 0.97-1.04 |

Prediction error, predicted minus true trip (hours, + late; min / median / max over S1-B's rows):

| At the trip minus | Linear 15 min | Quadratic 60 min |
| --- | --- | --- |
| 4 h | +5.22 / +8.37 / +11.07 | +0.51 / +1.05 / +1.28 |
| 3 h | +2.33 / +3.09 / +3.82 | +0.21 / +0.34 / +0.51 |
| 2 h | +0.84 / +1.04 / +1.29 | +0.07 / +0.12 / +0.18 |
| 1.5 h | +0.44 / +0.53 / +0.60 | +0.04 / +0.06 / +0.07 |
| 1 h | +0.20 / +0.24 / +0.27 | +0.01 / +0.02 / +0.03 |

- Linear is late, as feared, but safe here: its error shrinks as the trip nears, so the trigger still fires at least
  1.0 h ahead on every row of both plants and every window. Its lead is not the one the plan states (1.2 h for 1.5 h)
  and its predictions grade badly: none of the predictions between the bearing detection and the trigger is inside
  α = 0.2 (late errors weigh twice), so it earns no prognostic horizon. A linear plan that wants a real 1.5 h lead
  needs a horizon of about 2 h (15 min window: 1.44-1.52 h), a factor tuned to this curve's shape.
- Quadratic over 60 min puts the real lead at 1.42-1.47 h (spread 0.05 h) on S1-B and 1.40-1.44 h on S1-A, late by
  0.03-0.10 h only; between the bearing detection and the trigger 52-74 % of its predictions are inside α, and its
  prognostic horizon is 2.2-3.0 h on S1-B. Over 15 min it is too noisy (false early crossings), over 30 min usable
  (1.39-1.61 h).
- A threshold on the quantity does not transfer: D-3's 15 min winding mean ≥ 128 °C leads the trip by 1.44-1.84 h on
  S1-B and 0.62-0.76 h on S1-A (plant A runs 30 K cooler at full feed); vibration ≥ 3.2 mm/s by 1.51-1.83 h on S1-B.
  Its lead depends on the plant and the fault's speed; the extrapolation's limit is the protection itself.
- The winding is the quantity: its limit is the drive's own protection threshold (150 °C, component data, not
  calibrated), it is read every 10 s, and the trip is on it. The residual against the peers needs a limit that moves
  with the peers' level (one input per operator cannot express it) and gains nothing at a steady feed. Vibration has no
  protection; its "limit" is a calibrated value at the trip, which makes the prediction circular.

Recommendation: `fit: quadratic` over a 60 min window on the degraded drive's winding toward its 150 °C trip, and the
planned isolation (feed 0, belt empty for 180 s, stop the drive) when 0 < time to the limit ≤ 5400 + 180 s. `linear`
stays the default fit and the form O31 names; the S1 spec does not use it. The owner approved this departure from
O31.

## Limits

- Measured on do-nothing runs at a steady full feed, which is what the plant-B plan keeps until its isolation. A load
  change moves the winding through its 15 min lag; a feed surge on plant B can raise every drive toward 150 °C and
  then the prediction is honest, not false. The isolation is gated by the bearing phase, so a prediction alone never
  acts.
- The calibration rows are the ones the plan is judged on; the long-tier twin rows remain the check.
- The command path adds latency between the trigger and the stop (episode, approval, a 60 s barrier slice); the real
  stop lead is the trigger lead less that latency, about 1.3-1.4 h.

## Next reads

- [Stream processing](stream-processing.md)
- [Cognition](cognition.md)
