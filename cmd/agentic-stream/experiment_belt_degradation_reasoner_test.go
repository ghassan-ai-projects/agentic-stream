package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	gearboxSlopeResidualKPerH = 2.0
	gearboxQuietVibrationMmS  = 0.05
)

var (
	limpFeedA       = proposal{Type: "reduce_feed", Risk: "R1", Parameters: map[string]any{"level": "limp_a"}}
	emptyTheBelt    = proposal{Type: "stop_feed", Risk: "R2", Parameters: map[string]any{"level": "empty"}}
	isolateTheDrive = proposal{Type: "isolate_drive", Risk: "R2", Parameters: map[string]any{"mode": "planned_stop"}}
	stopTheBelt     = proposal{Type: "stop_belt", Risk: "R2", Parameters: map[string]any{"mode": "planned_stop"}}
	plannedSteps    = map[string]bool{"degrading": true, "shedding": true, "belt_empty": true, "isolated": true}
)

type lineCoverage struct{ slopeReadings, vibrationReadings float64 }

var (
	shippedCoverage    = lineCoverage{slopeReadings: 162, vibrationReadings: 14}
	compressedCoverage = lineCoverage{slopeReadings: 33, vibrationReadings: 3}
	compressedWindows  = map[string]string{"size: 15m": "size: 3m", "size: 30m": "size: 6m",
		"winding_readings_15m, 81.0": "winding_readings_15m, 17.0", "vibration_readings_15m, 14.0": "vibration_readings_15m, 3.0"}
)

type lineSnapshot struct {
	SituationID string         `json:"situation_id"`
	Phase       string         `json:"phase"`
	Facts       map[string]any `json:"facts"`
}

func reasonAboutTheLine(coverage lineCoverage) workerfake.ExecuteFunc {
	var mu sync.Mutex
	seizing := map[string]bool{}
	return func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		var snapshot lineSnapshot
		if err := json.Unmarshal(req.GetSnapshotJson(), &snapshot); err != nil {
			return fmt.Errorf("decode the snapshot: %w", err)
		}
		mu.Lock()
		seizing[snapshot.SituationID] = seizing[snapshot.SituationID] || snapshot.gearboxSeizing(coverage)
		diagnosis := map[bool]string{true: "gearbox_seizing", false: "bearing"}[seizing[snapshot.SituationID]]
		mu.Unlock()
		answer := workerfake.Answer{Hypothesis: diagnosis, DiagnosisCode: diagnosis, Summary: "D-3 degrades against its peers at equal current"}
		if step, planned := lineSteps(diagnosis)[snapshot.Phase]; planned {
			answer.Intents = []proposal{step}
		}
		return workerfake.Propose(req, emit, answer)
	}
}

func lineSteps(diagnosis string) map[string]proposal {
	stop := isolateTheDrive
	if diagnosis == "gearbox_seizing" {
		stop = stopTheBelt
	}
	return map[string]proposal{"degrading": limpFeedA, "shedding": emptyTheBelt, "belt_empty": stop, "isolated": limpFeedA}
}

func (s lineSnapshot) gearboxSeizing(coverage lineCoverage) bool {
	counted := min(s.fact("drive1_winding_readings_30m"), s.fact("drive2_winding_readings_30m"), s.fact("drive3_winding_readings_30m")) >= coverage.slopeReadings &&
		min(s.fact("drive1_vibration_readings_15m"), s.fact("drive2_vibration_readings_15m"), s.fact("drive3_vibration_readings_15m")) >= coverage.vibrationReadings
	return counted && s.residual("winding_slope_30m") >= gearboxSlopeResidualKPerH && s.residual("vibration_15m") < gearboxQuietVibrationMmS
}

func (s lineSnapshot) residual(feature string) float64 {
	return s.fact("drive3_"+feature) - max(s.fact("drive1_"+feature), s.fact("drive2_"+feature))
}

func (s lineSnapshot) fact(name string) float64 {
	value, _ := s.Facts["facts."+name].(float64)
	return value
}
