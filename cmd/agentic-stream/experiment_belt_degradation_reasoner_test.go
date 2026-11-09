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
	bearingVibrationResidualMmS = 0.3
	gearboxSlopeResidualKPerH   = 2.0
	gearboxQuietVibrationMmS    = 0.05
	unknownCause                = "unknown"
)

var (
	limpFeedA       = limpFeedOf("a")
	emptyTheBelt    = proposal{Type: "stop_feed", Risk: "R2", Parameters: map[string]any{"level": "empty"}}
	isolateTheDrive = proposal{Type: "isolate_drive", Risk: "R2", Parameters: map[string]any{"mode": "planned_stop"}}
	stopTheBelt     = proposal{Type: "stop_belt", Risk: "R2", Parameters: map[string]any{"mode": "planned_stop"}}
	plannedSteps    = map[string]bool{"degrading": true, "shedding": true, "cause_bearing": true, "cause_gearbox": true, "cause_undecided": true,
		"isolated": true}
)

type lineCoverage struct{ slopeReadings, vibrationReadings float64 }

var (
	shippedCoverage    = lineCoverage{slopeReadings: 162, vibrationReadings: 14}
	compressedCoverage = lineCoverage{slopeReadings: 33, vibrationReadings: 3}
	compressedWindows  = map[string]string{"size: 15m": "size: 3m", "size: 30m": "size: 6m",
		"winding_readings_15m, 81.0": "winding_readings_15m, 17.0", "vibration_readings_15m, 14.0": "vibration_readings_15m, 3.0",
		"slope_readings_30m, 162.0": "slope_readings_30m, 33.0", "minDuration: " + undecidedCauseWait: "minDuration: 5m"}
)

type lineSnapshot struct {
	SituationID string         `json:"situation_id"`
	Phase       string         `json:"phase"`
	Facts       map[string]any `json:"facts"`
}

func limpFeedOf(plant string) proposal {
	return proposal{Type: "reduce_feed", Risk: "R1", Parameters: map[string]any{"level": "limp_" + plant}}
}

func reasonAboutTheLine(coverage lineCoverage, limp proposal) workerfake.ExecuteFunc {
	var mu sync.Mutex
	named := map[string]string{}
	return func(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		var snapshot lineSnapshot
		if err := json.Unmarshal(req.GetSnapshotJson(), &snapshot); err != nil {
			return fmt.Errorf("decode the snapshot: %w", err)
		}
		mu.Lock()
		if named[snapshot.SituationID] == "" || named[snapshot.SituationID] == unknownCause {
			named[snapshot.SituationID] = snapshot.cause(coverage)
		}
		diagnosis := named[snapshot.SituationID]
		mu.Unlock()
		answer := workerfake.Answer{Hypothesis: diagnosis, DiagnosisCode: diagnosis, Summary: "D-3 degrades against its peers at equal current"}
		if step, planned := lineSteps(limp)[snapshot.Phase]; planned {
			answer.Intents = []proposal{step}
		}
		return workerfake.Propose(req, emit, answer)
	}
}

func lineSteps(limp proposal) map[string]proposal {
	return map[string]proposal{"degrading": limp, "shedding": emptyTheBelt, "cause_bearing": isolateTheDrive, "cause_gearbox": stopTheBelt,
		"cause_undecided": stopTheBelt, "isolated": limp}
}

func (s lineSnapshot) cause(coverage lineCoverage) string {
	vibrationCounted := s.leastReadings("vibration_readings_15m") >= coverage.vibrationReadings
	switch {
	case vibrationCounted && s.residual("vibration_15m") >= bearingVibrationResidualMmS:
		return "bearing"
	case vibrationCounted && s.leastReadings("winding_readings_30m") >= coverage.slopeReadings &&
		s.residual("winding_slope_30m") >= gearboxSlopeResidualKPerH && s.residual("vibration_15m") < gearboxQuietVibrationMmS:
		return "gearbox_seizing"
	}
	return unknownCause
}

func (s lineSnapshot) leastReadings(feature string) float64 {
	return min(s.fact("drive1_"+feature), s.fact("drive2_"+feature), s.fact("drive3_"+feature))
}

func (s lineSnapshot) residual(feature string) float64 {
	return s.fact("drive3_"+feature) - max(s.fact("drive1_"+feature), s.fact("drive2_"+feature))
}

func (s lineSnapshot) fact(name string) float64 {
	value, _ := s.Facts["facts."+name].(float64)
	return value
}
