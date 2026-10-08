package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newEpisodeCommand() *cobra.Command {
	var flags operatorFlags
	show := &cobra.Command{
		Use: "show <episode-id>", Short: "Show an episode: its attempts, refused results, Decisions and their intents.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return readOperatorDatabase(cmd, flags, func(db *storage.DB) error { return printEpisode(cmd, flags, db, args[0]) })
		},
	}
	flags.register(show)
	cmd := &cobra.Command{Use: "episode", Short: "Inspect an episode."}
	cmd.AddCommand(show)
	return cmd
}

// episodeInspection follows an episode from its attempts to the intents its
// Decisions proposed.
type episodeInspection struct {
	Episode   episodeledger.EpisodeRecord `json:"episode"`
	Decisions []decisionInspection        `json:"decisions"`
}

// decisionInspection is one Decision and the intents it proposed.
type decisionInspection struct {
	Decision episodes.DecisionView `json:"decision"`
	Intents  []policy.IntentView   `json:"intents"`
}

func printEpisode(cmd *cobra.Command, flags operatorFlags, db *storage.DB, episodeID string) error {
	episode, err := episodeledger.Episode(cmd.Context(), db.DB, flags.tenantID, episodeID)
	if err != nil {
		return fmt.Errorf("show episode: %w", err)
	}
	inspection := episodeInspection{Episode: episode}
	if inspection.Decisions, err = inspectDecisions(cmd, db, flags.tenantID, episodeID); err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, inspection, func() string { return inspection.text() })
}

func inspectDecisions(cmd *cobra.Command, db *storage.DB, tenantID, episodeID string) ([]decisionInspection, error) {
	decisions, err := episodes.Decisions(cmd.Context(), db, episodeID)
	if err != nil {
		return nil, fmt.Errorf("show episode decisions: %w", err)
	}
	inspections := make([]decisionInspection, 0, len(decisions))
	for _, decision := range decisions {
		intents, err := policy.DecisionIntents(cmd.Context(), db.DB, tenantID, decision.DecisionID)
		if err != nil {
			return nil, fmt.Errorf("show decision intents: %w", err)
		}
		inspections = append(inspections, decisionInspection{Decision: decision, Intents: intents})
	}
	return inspections, nil
}

func (e episodeInspection) text() string {
	ep := e.Episode
	lines := []string{fmt.Sprintf("%s %s on %s version %d executor=%s/%s dispatch=%s snapshot=%s", ep.EpisodeID, ep.LifecycleStatus, ep.SituationID, ep.SituationVersion, ep.ExecutorName, ep.ModelPolicy, orNone(ep.DispatchPolicy), ep.SnapshotSHA256)}
	for _, attempt := range ep.Attempts {
		lines = append(lines, fmt.Sprintf("attempt %s fence=%d status=%s terminal=%s", attempt.AttemptID, attempt.Fence, attempt.Status, orNone(string(attempt.Terminal))))
	}
	for _, rejection := range ep.Rejections {
		lines = append(lines, fmt.Sprintf("rejected %s attempt=%s fence=%d", rejection.Reason, orNone(rejection.AttemptID), rejection.Fence))
	}
	for _, d := range e.Decisions {
		lines = append(lines, fmt.Sprintf("decision %s %s version=%d %s", d.Decision.DecisionID, d.Decision.ValidationStatus, d.Decision.SituationVersion, d.Decision.RejectionReason))
		for _, intent := range d.Intents {
			lines = append(lines, fmt.Sprintf("  intent %s %s risk=%s policy=%s", intent.IntentID, intent.IntentType, intent.RiskClass, intent.PolicyStatus))
		}
	}
	return strings.Join(lines, "\n")
}
