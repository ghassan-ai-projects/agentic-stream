package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

type lostReading struct{ diagnosis, fault string }

var lostReadingEdits = map[string]string{"minDuration: " + undecidedCauseWait: "minDuration: 15m"}

var stuckVibration = lostReading{diagnosis: "sensor_stuck", fault: "conveyor_drive__vibration__frozen"}

func (s twinStory) withLost(lost lostReading) twinStory {
	s.lost = lost
	return s
}

func (s twinStory) faults() map[string]string {
	faults := map[string]string{s.diagnosis: s.fault}
	if s.lost.diagnosis != "" {
		faults[s.lost.diagnosis] = s.lost.fault
	}
	return faults
}

func (l lostReading) assertReportedBeforeTheCause(t *testing.T, runDir, cause string) {
	t.Helper()
	if l.fault == "" {
		return
	}
	var verdict struct {
		Detections []struct {
			EntityID   string `json:"entity_id"`
			DetectedAt string `json:"detected_at"`
			Label      string `json:"label"`
		} `json:"detections"`
	}
	decodeFile(t, filepath.Join(runDir, "verdict.json"), &verdict)
	first := map[string]string{}
	for _, d := range verdict.Detections {
		if _, seen := first[d.Label]; !seen && d.EntityID == "D-3" {
			first[d.Label] = d.DetectedAt
		}
	}
	lost, lostSeen := first[l.fault]
	caused, causeSeen := first[cause]
	t.Logf("D-3 detections: %s at %s, %s at %s", l.fault, lost, cause, caused)
	if !lostSeen || !causeSeen || lost >= caused {
		t.Fatalf("detections %+v: want %s on D-3 reported before %s", verdict.Detections, l.fault, cause)
	}
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestAStuckVibrationReadingIsReportedAndTheBearingIsJudgedByItsWinding(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	run := startDegradationServe(t, compressedSpec(t, lostReadingEdits), "a", reasonAboutTheLine(compressedCoverage))
	desk := approvalDesk(run.address, conveyorApprovalToken, conveyorApprover, campaign.ApproveAll{})
	line := connectBeltLine(t, run, stuckBearing)
	if !line.runUntil(t, degradationTicks, run.approvingUntilSettled(t, desk, len(isolationRun.order), "isolated")) || !slices.Equal(line.obeyed, isolationRun.order) {
		t.Fatalf("after %d minutes the host commanded %v, want %v", line.minutes(), line.obeyed, isolationRun.order)
	}
	assertDiagnosedFirst(t, run.db, stuckVibration.diagnosis, "bearing")
	assertR2PassedOnSettledFeatures(t, run.db, 2)
}

func stuckBearing(minute int) beltDisturbance {
	disturbance := bearingDegradation(minute)
	disturbance.heat += 0.02 * float64(max(minute-4, 0))
	disturbance.frozen = true
	return disturbance
}

func assertDiagnosedFirst(t *testing.T, dbPath string, first, then string) {
	t.Helper()
	query := `SELECT json_extract(d.raw_json, '$.diagnosis_code') FROM decisions d JOIN situation_versions v
		ON v.situation_id = d.situation_id AND v.version = d.situation_version WHERE d.validation_status = 'accepted' ORDER BY v.event_horizon, d.created_at`
	diagnoses := slices.Compact(columnValues(t, dbPath, query))
	if len(diagnoses) == 0 || diagnoses[0] != first || !slices.Contains(diagnoses, then) {
		t.Fatalf("accepted diagnoses in event time = %v, want %s first and %s later", diagnoses, first, then)
	}
}
