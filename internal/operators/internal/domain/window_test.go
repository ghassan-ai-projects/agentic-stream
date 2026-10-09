package domain_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestMeanWindowReportsTheAverageOfItsSamples(t *testing.T) {
	t.Parallel()
	h := newHarness(t, meanSpec())
	var last []domain.Feature
	for i, celsius := range []float64{1, 2, 3} {
		last = h.applyOnTime(temperature("t"+string(rune('0'+i)), base.Add(time.Duration(i)*time.Minute), celsius))
	}
	if len(last) != 1 {
		t.Fatalf("features = %d, want 1", len(last))
	}
	got := last[0]
	if got.OutputName != "mean_value" || got.Unit != "celsius" || got.Value != 2.0 || got.Completeness != string(domain.CompletenessProvisional) {
		t.Fatalf("feature = %+v, want provisional mean_value 2 celsius", got)
	}
	if !slices.Equal(got.InputEventIDs, []string{"t0", "t1", "t2"}) {
		t.Fatalf("input event ids = %v, want t0 t1 t2", got.InputEventIDs)
	}
	if want := got.Watermark.Add(-5 * time.Minute); !got.WindowStart.Equal(want) || !got.WindowEnd.Equal(got.Watermark) {
		t.Fatalf("window = %s..%s, want %s..%s", got.WindowStart, got.WindowEnd, want, got.Watermark)
	}
}

func TestSlopeIsReportedInValueUnitsPerHour(t *testing.T) {
	t.Parallel()
	slope := temperatureSpec(slidingWindow("6h", "15m", "on_update"), spec.Operator{Kind: "slope", Aggregate: "slope", Output: "slope_value", Unit: "celsius_per_hour"})
	h := newHarness(t, slope)
	var last []domain.Feature
	for hour := range 3 {
		last = h.applyOnTime(temperature("t", base.Add(time.Duration(hour)*time.Hour), float64(hour)*2))
	}
	if len(last) != 1 || last[0].Value != 2.0 || last[0].Unit != "celsius_per_hour" {
		t.Fatalf("features = %+v, want one slope of 2 celsius_per_hour", last)
	}
}

func TestEmitModeDecidesWhichEventsEmitAFeature(t *testing.T) {
	t.Parallel()
	const none = ""
	provisional, final := string(domain.CompletenessProvisional), string(domain.CompletenessFinalByPolicy)
	tests := []struct {
		emit string
		want []string
	}{
		{"on_update", []string{provisional, provisional, provisional, provisional}},
		{"early_and_close", []string{provisional, provisional, provisional + "," + final, provisional}},
		{"on_close", []string{none, none, final, none}},
		{"", []string{none, none, final, none}},
	}
	for _, tc := range tests {
		t.Run("emit="+tc.emit, func(t *testing.T) {
			t.Parallel()
			compiled := temperatureSpec(slidingWindow("5m", "2m", tc.emit), spec.Operator{Kind: "aggregate", Aggregate: "mean", Output: "mean_value"})
			h := newHarness(t, compiled)
			var got []string
			for minute := range 4 {
				features := h.applyOnTime(temperature("t", base.Add(time.Duration(minute)*time.Minute), float64(minute*2+1)))
				got = append(got, completenessOf(features))
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("completeness per event = %q, want %q", got, tc.want)
			}
		})
	}
}

func completenessOf(features []domain.Feature) string {
	completeness := make([]string, len(features))
	for i, feature := range features {
		completeness[i] = feature.Completeness
	}
	return strings.Join(completeness, ",")
}

func TestOnCloseWindowEmitsTheClosedAggregateAtTheWatermark(t *testing.T) {
	t.Parallel()
	h := newHarness(t, temperatureSpec(slidingWindow("5m", "2m", ""), spec.Operator{Kind: "aggregate", Aggregate: "mean", Output: "mean_value"}))
	h.applyOnTime(temperature("s0", base, 1))
	h.applyOnTime(temperature("s1", base.Add(time.Minute), 3))
	closing := temperature("s2", base.Add(2*time.Minute), 5)
	features := h.applyOnTime(closing)
	if len(features) != 1 {
		t.Fatalf("features = %d, want 1 closed window", len(features))
	}
	if got := features[0]; got.Value != 3.0 || !got.WindowEnd.Equal(closing.EventTime) {
		t.Fatalf("closed window = value %v end %s, want 3 at %s", got.Value, got.WindowEnd, closing.EventTime)
	}
}

func TestALateSampleIsLabelledCorrectedOnlyUnderACorrectingPolicyWithinAllowedLateness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		policy      string
		lateness    string
		lateBy      time.Duration
		wantCorrect bool
	}{
		{"correct within lateness", "correct", "5m", time.Minute, true},
		{"correct_and_reconsider within lateness", "correct_and_reconsider", "5m", time.Minute, true},
		{"exactly at allowed lateness", "correct", "5m", 5 * time.Minute, true},
		{"history only does not correct", "history_only", "5m", time.Minute, false},
		{"drop with audit does not correct", "drop_with_audit", "5m", time.Minute, false},
		{"no allowed lateness declared", "correct", "", time.Minute, false},
		{"later than allowed lateness", "correct", "30s", time.Minute, false},
		{"older than the window", "correct", "10m", 6 * time.Minute, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiled := meanSpec()
			compiled.Time = spec.TimePolicy{LatePolicy: tc.policy, AllowedLateness: tc.lateness}
			h := newHarness(t, compiled)
			h.apply(temperature("first", base, 10), base, base)
			late := temperature("late", base.Add(-tc.lateBy), 10)
			features := h.apply(late, base, late.IngestedAt)
			if len(features) != 1 {
				t.Fatalf("late event emitted %d features, want 1", len(features))
			}
			corrected := features[0].Completeness == string(domain.CompletenessCorrected)
			if corrected != tc.wantCorrect {
				t.Fatalf("completeness = %q, corrected = %v, want %v", features[0].Completeness, corrected, tc.wantCorrect)
			}
		})
	}
}

func TestCorrectedWindowReemitsEveryInputIncludingTheLateOne(t *testing.T) {
	t.Parallel()
	compiled := meanSpec()
	compiled.Time = spec.TimePolicy{LatePolicy: "correct_and_reconsider", AllowedLateness: "5m"}
	h := newHarness(t, compiled)
	h.apply(temperature("first", base, 10), base, base)
	late := temperature("late", base.Add(-time.Minute), 10)
	corrected := h.apply(late, base, late.IngestedAt)
	if len(corrected) != 1 || corrected[0].Value != 10.0 || !slices.Equal(corrected[0].InputEventIDs, []string{"late", "first"}) {
		t.Fatalf("corrected = %+v, want one feature over late then first", corrected)
	}
	inOrder := temperature("in-order", base.Add(time.Minute), 10)
	next := h.apply(inOrder, inOrder.EventTime, inOrder.IngestedAt)
	if len(next) != 1 || next[0].Completeness == string(domain.CompletenessCorrected) {
		t.Fatalf("in-order event after a correction = %+v, want one uncorrected feature", next)
	}
}

func TestMalformedAllowedLatenessFailsTheLateEvent(t *testing.T) {
	t.Parallel()
	compiled := meanSpec()
	compiled.Time = spec.TimePolicy{LatePolicy: "correct", AllowedLateness: "soon"}
	h := newHarness(t, compiled)
	h.apply(temperature("first", base, 10), base, base)
	late := temperature("late", base.Add(-time.Minute), 10)
	_, _, err := h.rt.ApplyEventAt(t.Context(), h.ps, late, base, late.IngestedAt)
	requireErrorContaining(t, err, "allowed lateness")
}

func TestLatestAndMaximumFollowEventTimeThenEventIDNotArrivalOrder(t *testing.T) {
	t.Parallel()
	type arrival struct {
		id     string
		offset time.Duration
		value  float64
	}
	tests := []struct {
		name        string
		arrivals    []arrival
		wantLatest  float64
		wantMaximum float64
	}{
		{"event time beats arrival order", []arrival{{"late-arrival", 2 * time.Minute, 10}, {"older-event-time", time.Minute, 100}}, 10, 100},
		{"event id breaks an event time tie", []arrival{{"event-b", time.Minute, 2}, {"event-a", time.Minute, 3}}, 2, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiled := meanSpec()
			compiled.Operators = []spec.Operator{
				{Name: "latest", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Aggregate: "latest", Window: "w1", Output: "latest_value"},
				{Name: "maximum", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Aggregate: "max", Window: "w1", Output: "max_value"},
			}
			h := newHarness(t, compiled)
			var features []domain.Feature
			for _, a := range tc.arrivals {
				features = h.applyOnTime(temperature(a.id, base.Add(a.offset), a.value))
			}
			values := map[string]any{}
			for _, feature := range features {
				values[feature.OutputName] = feature.Value
			}
			if values["latest_value"] != tc.wantLatest || values["max_value"] != tc.wantMaximum {
				t.Fatalf("latest, max = %v, %v; want %v, %v", values["latest_value"], values["max_value"], tc.wantLatest, tc.wantMaximum)
			}
		})
	}
}
