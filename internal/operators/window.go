package operators

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

func (r *OperatorRuntime) applyWindowOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	value, ok := r.extractValue(inst, env)
	if !ok {
		return nil, nil
	}

	if blob.Window == nil {
		blob.Window = &WindowState{}
	}
	ws := blob.Window
	bootID := deviceBootID(env)
	if ws.BootID == "" {
		ws.BootID = bootID
	}
	if ws.WindowEnd.IsZero() {
		ws.WindowEnd = watermark
	}
	corrected, err := r.isLateWindowCorrection(inst.window.size, env.EventTime, watermark, ws.LastEmit)
	if err != nil {
		return nil, err
	}
	ws.addSample(Sample{EventID: env.ID, EventTime: env.EventTime, Value: value, BootID: bootID}, watermark.Add(-inst.window.size))

	agg, err := computeAggregate(inst.def.Aggregate, ws.Samples)
	if err != nil {
		return nil, err
	}
	closeDue := windowCloseDue(inst.window, ws, watermark)
	var features []Feature
	for _, completeness := range windowEmissions(inst.window.emit, corrected, closeDue, len(ws.Samples) > 0) {
		features = append(features, r.windowFeature(inst, env, ws, agg, watermark, completeness))
	}
	if len(features) > 0 {
		ws.LastEmit = watermark
	}
	if closeDue {
		// Watermark advancement is the close signal. A single latest close is
		// emitted after a gap so the runtime never fabricates unobserved windows.
		ws.WindowEnd = watermark
	}
	return features, nil
}

// addSample records a sample, evicts samples before cutoff, and keeps the
// window sorted by event time, then event ID, for deterministic output.
func (ws *WindowState) addSample(sample Sample, cutoff time.Time) {
	ws.Samples = append(ws.Samples, sample)
	filtered := ws.Samples[:0]
	for _, s := range ws.Samples {
		if !s.EventTime.Before(cutoff) {
			filtered = append(filtered, s)
		}
	}
	ws.Samples = filtered
	slices.SortStableFunc(ws.Samples, func(a, b Sample) int {
		if a.EventTime.Equal(b.EventTime) {
			return strings.Compare(a.EventID, b.EventID)
		}
		if a.EventTime.Before(b.EventTime) {
			return -1
		}
		return 1
	})
}

// windowEmissions lists the completeness of each feature a window emits for
// this event, in emission order. A late correction re-emits the window as
// corrected; otherwise on_update and early_and_close emit a provisional value
// and on_close emits the final value when the window closes.
func windowEmissions(emit string, corrected, closeDue, hasSamples bool) []string {
	if !hasSamples {
		return nil
	}
	if corrected {
		return []string{string(CompletenessCorrected)}
	}
	var emissions []string
	if emit == "on_update" || emit == "early_and_close" {
		emissions = append(emissions, string(CompletenessProvisional))
	}
	if closeDue && emit == "on_close" {
		emissions = append(emissions, string(CompletenessFinalByPolicy))
	}
	return emissions
}

func (r *OperatorRuntime) windowFeature(inst *operatorInstance, env contractsv1.Envelope, ws *WindowState, agg float64, watermark time.Time, completeness string) Feature {
	return Feature{
		FeatureID:     r.idGen.New(ids.PrefixEvent),
		OperatorID:    inst.def.Name,
		OutputName:    inst.def.Output,
		TenantID:      env.TenantID,
		EntityType:    env.Entity.Type,
		EntityID:      env.Entity.ID,
		StateKey:      operatorStateKey(env),
		BootID:        deviceBootID(env),
		PartitionID:   env.PartitionID(0),
		WindowStart:   watermark.Add(-inst.window.size),
		WindowEnd:     watermark,
		Value:         agg,
		Unit:          inst.def.Unit,
		EventTime:     env.EventTime,
		Watermark:     watermark,
		InputEventIDs: eventIDs(ws.Samples),
		Completeness:  completeness,
		Traceparent:   env.Traceparent,
		Tracestate:    env.Tracestate,
	}
}

func windowCloseDue(cfg *windowConfig, ws *WindowState, watermark time.Time) bool {
	if ws.WindowEnd.IsZero() || watermark.Before(ws.WindowEnd) {
		return false
	}
	interval := cfg.size
	if cfg.slide > 0 {
		interval = cfg.slide
	}
	return watermark.Sub(ws.WindowEnd) >= interval
}

func (r *OperatorRuntime) isLateWindowCorrection(windowSize time.Duration, eventTime, watermark, lastEmit time.Time) (bool, error) {
	if r.spec.Time.LatePolicy != "correct" && r.spec.Time.LatePolicy != "correct_and_reconsider" {
		return false, nil
	}
	if lastEmit.IsZero() || !eventTime.Before(watermark) || !eventTime.Before(lastEmit) || eventTime.Before(lastEmit.Add(-windowSize)) {
		return false, nil
	}
	if r.spec.Time.AllowedLateness == "" {
		return false, nil
	}
	allowedLateness, err := parseDuration(r.spec.Time.AllowedLateness)
	if err != nil {
		return false, fmt.Errorf("allowed lateness: %w", err)
	}
	return watermark.Sub(eventTime) <= allowedLateness, nil
}

func computeAggregate(agg string, samples []Sample) (float64, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	switch agg {
	case "mean":
		var sum float64
		for _, s := range samples {
			sum += s.Value
		}
		return sum / float64(len(samples)), nil
	case "rms":
		var sumSquares float64
		for _, s := range samples {
			sumSquares += s.Value * s.Value
		}
		return math.Sqrt(sumSquares / float64(len(samples))), nil
	case "slope":
		return linearSlope(samples), nil
	case "count":
		return float64(len(samples)), nil
	case "sum":
		var sum float64
		for _, s := range samples {
			sum += s.Value
		}
		return sum, nil
	case "min":
		m := samples[0].Value
		for _, s := range samples {
			if s.Value < m {
				m = s.Value
			}
		}
		return m, nil
	case "max":
		m := samples[0].Value
		for _, s := range samples {
			if s.Value > m {
				m = s.Value
			}
		}
		return m, nil
	case "latest":
		// Samples are sorted by event time and then event ID before this
		// function is called. The final sample is therefore deterministic even
		// when events arrive out of order or share an event timestamp.
		return samples[len(samples)-1].Value, nil
	default:
		return 0, fmt.Errorf("unsupported aggregate %q", agg)
	}
}

// linearSlope returns the rate in value-units per hour. The output unit is
// metadata on the emitted feature; rate scaling is a single runtime contract,
// not inferred from domain-specific unit names.
func linearSlope(samples []Sample) float64 {
	if len(samples) < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	start := samples[0].EventTime
	for _, s := range samples {
		x := s.EventTime.Sub(start).Hours()
		sumX += x
		sumY += s.Value
		sumXY += x * s.Value
		sumXX += x * x
	}
	n := float64(len(samples))
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	slope := (n*sumXY - sumX*sumY) / denom
	return slope
}

func eventIDs(samples []Sample) []string {
	ids := make([]string, len(samples))
	for i, s := range samples {
		ids[i] = s.EventID
	}
	return ids
}

// TimerIdentity identifies the tenant and partition whose timer is firing.
// Timer calls that persist features must provide it explicitly; an omitted
// identity yields an unknown tenant and partition rather than a misleading
// default.
