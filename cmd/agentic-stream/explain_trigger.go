package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type triggerExplanation struct {
	Evaluation cognition.TriggerEvaluationRecord `json:"evaluation"`
	Scheduling *episodeledger.SchedulingRecord   `json:"scheduling,omitempty"`
}

func printTriggerExplanation(cmd *cobra.Command, flags operatorFlags, db *storage.DB, triggerID string) error {
	evaluation, err := cognition.TriggerEvaluation(cmd.Context(), db.DB, flags.tenantID, triggerID)
	if err != nil {
		return fmt.Errorf("explain trigger: %w", err)
	}
	explanation := triggerExplanation{Evaluation: evaluation}
	scheduling, found, err := episodeledger.Scheduling(cmd.Context(), db.DB, flags.tenantID, triggerID)
	if err != nil {
		return fmt.Errorf("explain trigger scheduling: %w", err)
	}
	if found {
		explanation.Scheduling = &scheduling
	}
	return printResult(cmd, flags.asJSON, explanation, func() string { return explanation.text() })
}

func (e triggerExplanation) text() string {
	v := e.Evaluation
	text := fmt.Sprintf("%s %s on %s version %d: outcome=%s score=%g threshold=%g lane=%s\nreasons: %s\ndelta: %s",
		v.TriggerID, v.TriggerName, v.SituationID, v.SituationVersion, v.Outcome, v.Score, v.Threshold, v.Lane, strings.Join(v.Reasons, "; "), string(v.Delta))
	if e.Scheduling == nil {
		return text + "\nscheduling: none (the evaluation admitted no work)"
	}
	return text + "\n" + schedulingText(*e.Scheduling)
}

func schedulingText(s episodeledger.SchedulingRecord) string {
	text := fmt.Sprintf("scheduling: item=%s status=%s lane=%s priority=%g expires_at=%s", s.SchedulerItemID, s.Status, s.Lane, s.Priority, s.ExpiresAt)
	if s.EpisodeID == "" {
		return text + "\nepisode: none"
	}
	text += fmt.Sprintf("\nepisode: %s status=%s situation_version=%d", s.EpisodeID, s.EpisodeStatus, s.EpisodeSituationVersion)
	for _, rejection := range s.Rejections {
		text += fmt.Sprintf("\nrejected: %s attempt=%s fence=%d details=%s", rejection.Reason, orNone(rejection.AttemptID), rejection.Fence, string(rejection.Details))
	}
	return text
}
