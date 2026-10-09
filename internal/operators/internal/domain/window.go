package domain

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func (r *OperatorRuntime) applyWindowOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	value, ok := r.extractValue(inst, env)
	if !ok {
		return nil, nil
	}
	ws := blob.windowState(deviceBootID(env), watermark)
	corrected, err := r.isLateWindowCorrection(inst.window.size, env.EventTime, watermark, ws.LastEmit)
	if err != nil {
		return nil, err
	}
	ws.addSample(Sample{EventID: env.ID, EventTime: env.EventTime, Value: value, BootID: deviceBootID(env)}, watermark.Add(-inst.window.size))
	agg, err := computeAggregate(inst.def.Aggregate, ws.Samples)
	if err != nil {
		return nil, err
	}
	return r.emitWindow(inst, env, ws, agg, watermark, corrected), nil
}

func (blob *OperatorStateBlob) windowState(bootID string, watermark time.Time) *WindowState {
	if blob.Window == nil {
		blob.Window = &WindowState{}
	}
	ws := blob.Window
	if ws.BootID == "" {
		ws.BootID = bootID
	}
	if ws.WindowEnd.IsZero() {
		ws.WindowEnd = watermark
	}
	return ws
}

func (r *OperatorRuntime) emitWindow(inst *operatorInstance, env contractsv1.Envelope, ws *WindowState, agg float64, watermark time.Time, corrected bool) []Feature {
	closeDue := windowCloseDue(inst.window, ws, watermark)
	var features []Feature
	for _, completeness := range windowEmissions(inst.window.emit, corrected, closeDue, len(ws.Samples) > 0) {
		features = append(features, r.windowFeature(inst, env, ws, agg, watermark, completeness))
	}
	if len(features) > 0 {
		ws.LastEmit = watermark
	}
	if closeDue {
		ws.WindowEnd = watermark
	}
	return features
}

func (ws *WindowState) addSample(sample Sample, cutoff time.Time) {
	ws.Samples = slices.DeleteFunc(append(ws.Samples, sample), func(s Sample) bool { return s.EventTime.Before(cutoff) })
	slices.SortStableFunc(ws.Samples, compareSamples)
}

func compareSamples(a, b Sample) int {
	if a.EventTime.Equal(b.EventTime) {
		return strings.Compare(a.EventID, b.EventID)
	}
	if a.EventTime.Before(b.EventTime) {
		return -1
	}
	return 1
}

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
	if closeDue && emit != "on_update" {
		emissions = append(emissions, string(CompletenessFinalByPolicy))
	}
	return emissions
}

func (r *OperatorRuntime) windowFeature(inst *operatorInstance, env contractsv1.Envelope, ws *WindowState, agg float64, watermark time.Time, completeness string) Feature {
	return Feature{
		FeatureID: r.idGen.New(sources.PrefixEvent), OperatorID: inst.def.Name, OutputName: inst.def.Output,
		TenantID: env.TenantID, EntityType: env.Entity.Type, EntityID: env.Entity.ID,
		StateKey: operatorStateKey(env), BootID: deviceBootID(env), PartitionID: env.PartitionID(0),
		WindowStart: watermark.Add(-inst.window.size), WindowEnd: watermark,
		Value: agg, Unit: inst.def.Unit, EventTime: env.EventTime, Watermark: watermark,
		InputEventIDs: eventIDs(ws.Samples), Completeness: completeness,
		Traceparent: env.Traceparent, Tracestate: env.Tracestate,
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
	compute, ok := aggregates[agg]
	if !ok {
		return 0, fmt.Errorf("unsupported aggregate %q", agg)
	}
	return compute(samples), nil
}

var aggregates = map[string]func([]Sample) float64{
	"mean":   func(s []Sample) float64 { return sumValues(s) / float64(len(s)) },
	"rms":    rootMeanSquare,
	"slope":  linearSlope,
	"count":  func(s []Sample) float64 { return float64(len(s)) },
	"sum":    sumValues,
	"min":    func(s []Sample) float64 { return extremeValue(s, func(a, b float64) bool { return a < b }) },
	"max":    func(s []Sample) float64 { return extremeValue(s, func(a, b float64) bool { return a > b }) },
	"latest": func(s []Sample) float64 { return s[len(s)-1].Value },
}

func sumValues(samples []Sample) float64 {
	var sum float64
	for _, s := range samples {
		sum += s.Value
	}
	return sum
}

func rootMeanSquare(samples []Sample) float64 {
	var sumSquares float64
	for _, s := range samples {
		sumSquares += roundedProduct(s.Value, s.Value)
	}
	return math.Sqrt(sumSquares / float64(len(samples)))
}

func extremeValue(samples []Sample, beats func(candidate, current float64) bool) float64 {
	extreme := samples[0].Value
	for _, s := range samples {
		if beats(s.Value, extreme) {
			extreme = s.Value
		}
	}
	return extreme
}

func linearSlope(samples []Sample) float64 {
	if len(samples) < 2 {
		return 0
	}
	sumX, sumY, sumXY, sumXX := slopeSums(samples)
	n := float64(len(samples))
	denom := roundedProduct(n, sumXX) - roundedProduct(sumX, sumX)
	if denom == 0 {
		return 0
	}
	return (roundedProduct(n, sumXY) - roundedProduct(sumX, sumY)) / denom
}

func slopeSums(samples []Sample) (sumX, sumY, sumXY, sumXX float64) {
	start := samples[0].EventTime
	for _, s := range samples {
		x := s.EventTime.Sub(start).Hours()
		sumX += x
		sumY += s.Value
		sumXY += roundedProduct(x, s.Value)
		sumXX += roundedProduct(x, x)
	}
	return sumX, sumY, sumXY, sumXX
}

func roundedProduct(a, b float64) float64 {
	return float64(a * b)
}

func eventIDs(samples []Sample) []string {
	ids := make([]string, len(samples))
	for i, s := range samples {
		ids[i] = s.EventID
	}
	return ids
}
