package main

import (
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
)

var plannedStopSpec = filepath.Join(conveyorData, "drive-degradation-planned-stop.situation.yaml")

func TestThePlannedStopSpecSettlesEveryWindowAndPredictsTheTripFromAQuadraticFit(t *testing.T) {
	t.Parallel()
	out := runCLI(t, newRootCommand(), "validate", plannedStopSpec, "--event-schemas", conveyorEventSchemas)
	if !strings.HasPrefix(out, "ok: drive_degradation_planned_stop\n") {
		t.Fatalf("validate --event-schemas = %q, want drive_degradation_planned_stop", out)
	}
	compiled := compileConveyorSpec(t, plannedStopSpec)
	for _, window := range compiled.Windows {
		if window.Emit != "on_close" {
			t.Errorf("window %s emits %q, want on_close: an R2 intent is denied on provisional features", window.Name, window.Emit)
		}
	}
	predicts := slices.ContainsFunc(compiled.Operators, func(op spec.Operator) bool {
		extrapolation, err := op.Extrapolation()
		return op.Kind == "extrapolate" && err == nil && extrapolation.Limit == 150 && extrapolation.Fit == spec.FitQuadratic
	})
	if !predicts {
		t.Fatal("the planned-stop spec has no quadratic extrapolation to the 150 C winding trip")
	}
}

func (s twinStory) subjects(base campaign.Subjects) campaign.Subjects {
	if s.prognosis != "" {
		base.Prognosis = map[string]string{s.fault: s.prognosis}
	}
	return base
}

func (s twinStory) judge(t *testing.T, grade outcomeGrade, ran twinRun) {
	t.Helper()
	if s.spec != plannedStopSpec && s.lost.fault == "" {
		return
	}
	assertBeatsEveryBaseline(t, grade)
	trip, stop := doNothingTrip(grade, s.device), ran.firstCall(s.device, "stop_drive")
	t.Logf("%s stopped %.2f h before its do-nothing trip (stop at %s, trip at %s)", s.device, trip.Sub(stop).Hours(), stop, trip)
}

func assertBeatsEveryBaseline(t *testing.T, grade outcomeGrade) {
	t.Helper()
	for _, verdict := range grade.Verdicts {
		if verdict.Verdict != "beats" {
			t.Errorf("against %s the run %s by %+.2f equivalent hours, want it to beat every baseline", verdict.Baseline, verdict.Verdict, verdict.Delta)
		}
	}
}

func doNothingTrip(grade outcomeGrade, device string) time.Time {
	first := int64(math.MaxInt64)
	for _, run := range grade.Runs {
		for _, entity := range run.Facts.Entities {
			for _, trip := range entity.Trips {
				if run.Mode == "do_nothing" && entity.EntityID == device {
					first = min(first, trip.FirstNS)
				}
			}
		}
	}
	return time.Unix(0, first).UTC()
}

func (r twinRun) firstCall(device, operation string) time.Time {
	for _, command := range r.CommandLog {
		if command.Op == "effector.invoke" && command.Args.EntityID == device && strings.HasSuffix(command.Args.Effector, operation) {
			return time.Unix(0, command.Args.AtNS).UTC()
		}
	}
	return time.Time{}
}
