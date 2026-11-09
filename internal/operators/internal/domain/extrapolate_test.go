package domain_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestExtrapolationPublishesTheSecondsUntilItsFitReachesTheLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		config   map[string]any
		readings []float64
		want     float64
	}{
		{"a straight line rising 10 K/h from 140 reaches 150 in an hour", linearFit(), []float64{130, 135, 140}, 3600},
		{"the parabola 140 + 4x + 3x² crosses 150 at (√136 − 4) / 6 h", quadraticFit(), []float64{139, 138.75, 140}, 4597.142273814361},
		{"noisy readings, quadratic fit", quadraticFit(), noisyReadings(), 8333.552665754356},
		{"noisy readings, linear fit", linearFit(), noisyReadings(), 28333.333333333256},
		{"a fit already past its limit while rising publishes the least time", linearFit(), []float64{145, 150, 155}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := lastExtrapolation(t, tc.config, tc.readings)
			if math.Abs(got-tc.want) > 1e-6*tc.want {
				t.Fatalf("seconds to limit = %.9f, want %.9f", got, tc.want)
			}
		})
	}
}

func TestExtrapolationPublishesZeroWhenItHasNoValidPrediction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		config   map[string]any
		readings []float64
	}{
		{"a falling fit", linearFit(), []float64{140, 135, 130}},
		{"fewer readings than minPoints", withConfig(linearFit(), "minPoints", 4), []float64{130, 135, 140}},
		{"a crossing beyond maxHorizon", withConfig(linearFit(), "maxHorizon", "30m"), []float64{130, 135, 140}},
		{"a parabola 140 + 4x − 10x² that bends away before the limit", quadraticFit(), []float64{126, 135.5, 140}},
		{"one reading cannot rise", linearFit(), []float64{140}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := lastExtrapolation(t, tc.config, tc.readings); got != 0 {
				t.Fatalf("seconds to limit = %g, want 0", got)
			}
		})
	}
}

func TestExtrapolationRefusesAnInvalidConfiguration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		config map[string]any
		want   string
	}{
		{map[string]any{"fit": "linear"}, "limit is not a number"},
		{map[string]any{"limit": 150, "fit": "cubic"}, `fit "cubic"`},
		{map[string]any{"limit": 150, "fit": "quadratic", "minPoints": 2}, "minPoints 2"},
		{map[string]any{"limit": 150, "maxHorizon": "soon"}, "maxHorizon"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewOperatorRuntime("d1", extrapolateSpec(tc.config), nil)
			if err == nil || !strings.Contains(err.Error(), "operator op1: "+tc.want) {
				t.Fatalf("error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

func lastExtrapolation(t *testing.T, config map[string]any, readings []float64) float64 {
	t.Helper()
	h := newHarness(t, extrapolateSpec(config))
	step := time.Hour / time.Duration(max(len(readings)-1, 1))
	var last []domain.Feature
	for i, celsius := range readings {
		last = h.applyOnTime(temperature(fmt.Sprintf("t%d", i), base.Add(time.Duration(i)*step), celsius))
	}
	if len(last) != 1 || last[0].Unit != "second" {
		t.Fatalf("features = %+v, want one in seconds", last)
	}
	value, ok := last[0].Value.(float64)
	if !ok {
		t.Fatalf("value %v is not a number", last[0].Value)
	}
	return value
}

func extrapolateSpec(config map[string]any) *spec.CompiledSpec {
	return temperatureSpec(slidingWindow("2h", "10m", "on_update"), spec.Operator{Kind: "extrapolate", Output: "to_limit_s", Unit: "second", Configuration: config})
}

func linearFit() map[string]any { return map[string]any{"limit": 150.0} }

func quadraticFit() map[string]any { return map[string]any{"limit": 150, "fit": "quadratic"} }

func withConfig(config map[string]any, key string, value any) map[string]any {
	config[key] = value
	return config
}

func noisyReadings() []float64 { return []float64{116.3, 116.2, 117.0, 117.1, 118.4, 119.1, 119.9} }

func TestExtrapolationReplaysToTheSameBits(t *testing.T) {
	t.Parallel()
	first := lastExtrapolation(t, quadraticFit(), noisyReadings())
	second := lastExtrapolation(t, quadraticFit(), noisyReadings())
	if math.Float64bits(first) != math.Float64bits(second) {
		t.Fatalf("replay gave %x then %x", math.Float64bits(first), math.Float64bits(second))
	}
}
