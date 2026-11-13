package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type FailedReasoning struct {
	SituationID      string
	SituationVersion int
}

type ReasonedVersion struct {
	Phase   string
	Version int
	Horizon time.Time
}

func FailedSituations(failed []FailedReasoning) []string {
	var situationIDs []string
	for _, reasoning := range failed {
		if len(situationIDs) == 0 || situationIDs[len(situationIDs)-1] != reasoning.SituationID {
			situationIDs = append(situationIDs, reasoning.SituationID)
		}
	}
	return situationIDs
}

func RetryDue(policy spec.RetryPolicy, at situations.Position, reasoned []ReasonedVersion, retried int) (int, bool, error) {
	after, err := spec.ParseDuration(policy.After)
	if err != nil {
		return 0, false, fmt.Errorf("parse retryFailedReasoning.after: %w", err)
	}
	failures, latest := failuresInPhase(at.Phase, reasoned)
	unretried := failures > 0 && latest.Version > retried
	return latest.Version, unretried && failures <= policy.Max && !at.EventTime.Before(latest.Horizon.Add(after)), nil
}

func failuresInPhase(phase string, reasoned []ReasonedVersion) (int, ReasonedVersion) {
	failures, latest := 0, ReasonedVersion{}
	for _, version := range reasoned {
		if version.Phase != phase {
			continue
		}
		failures++
		if version.Version > latest.Version {
			latest = version
		}
	}
	return failures, latest
}
