package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

func newIntentCommand() *cobra.Command {
	return newShowGroup("intent", "Inspect an intent.", "intent-id",
		"Show an intent: policy evaluations, approvals, commands, outcomes, verifications and watches.", printIntent)
}

type intentInspection struct {
	Intent    policy.IntentView             `json:"intent"`
	Approvals []approvalledger.ApprovalView `json:"approvals"`
	Commands  []commandInspection           `json:"commands"`
}

type commandInspection struct {
	Command actions.CommandView `json:"command"`
	Watch   *watch.WatchView    `json:"watch,omitempty"`
}

func printIntent(cmd *cobra.Command, flags operatorFlags, db *storage.DB, intentID string) error {
	intent, err := policy.Intent(cmd.Context(), db.DB, flags.tenantID, intentID)
	if err != nil {
		return fmt.Errorf("show intent: %w", err)
	}
	inspection := intentInspection{Intent: intent}
	if inspection.Approvals, err = approvalledger.Approvals(cmd.Context(), db.DB, intentID); err != nil {
		return fmt.Errorf("show intent approvals: %w", err)
	}
	if inspection.Commands, err = inspectCommands(cmd, db, flags.tenantID, intentID); err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, inspection, func() string { return inspection.text() })
}

func inspectCommands(cmd *cobra.Command, db *storage.DB, tenantID, intentID string) ([]commandInspection, error) {
	commands, err := actions.IntentCommands(cmd.Context(), db, intentID)
	if err != nil {
		return nil, fmt.Errorf("show intent commands: %w", err)
	}
	inspections := make([]commandInspection, 0, len(commands))
	for _, command := range commands {
		installed, err := installedWatch(cmd, db, tenantID, command)
		if err != nil {
			return nil, err
		}
		inspections = append(inspections, commandInspection{Command: command, Watch: installed})
	}
	return inspections, nil
}

func installedWatch(cmd *cobra.Command, db *storage.DB, tenantID string, command actions.CommandView) (*watch.WatchView, error) {
	for _, outcome := range command.Outcomes {
		watchID := watch.InstalledWatchID(outcome.ProviderResult)
		if watchID == "" {
			continue
		}
		installed, found, err := watch.Watch(cmd.Context(), db, tenantID, watchID)
		if err != nil {
			return nil, fmt.Errorf("read watch %s of command %s: %w", watchID, command.CommandID, err)
		}
		if !found {
			return nil, nil
		}
		return &installed, nil
	}
	return nil, nil
}

func (i intentInspection) text() string {
	v := i.Intent
	lines := []string{fmt.Sprintf("%s %s risk=%s policy=%s on %s version %d (decision %s)", v.IntentID, v.IntentType, v.RiskClass, v.PolicyStatus, v.SituationID, v.SituationVersion, v.DecisionID)}
	for _, e := range v.Evaluations {
		lines = append(lines, fmt.Sprintf("policy %s: %s (%s) command=%s approval=%s", e.EvaluationID, e.Result, e.Reason, orNone(e.CommandID), orNone(e.ApprovalID)))
	}
	for _, a := range i.Approvals {
		lines = append(lines, fmt.Sprintf("approval %s %s approver=%s relay=%s reason=%s", a.ApprovalID, a.Status, orNone(a.Approver), orNone(a.Relay), orNone(a.Reason)))
	}
	for _, c := range i.Commands {
		lines = append(lines, c.text()...)
	}
	return strings.Join(lines, "\n")
}

func (c commandInspection) text() []string {
	lines := []string{fmt.Sprintf("command %s %s target=%s status=%s", c.Command.CommandID, c.Command.EffectorRoute, c.Command.Target, c.Command.Status)}
	for _, o := range c.Command.Outcomes {
		lines = append(lines, fmt.Sprintf("  outcome %s %s result=%s", o.OutcomeID, o.Status, string(o.ProviderResult)))
	}
	for _, v := range c.Command.Verifications {
		lines = append(lines, fmt.Sprintf("  verification %s %s verdict=%s", v.VerificationID, v.Status, orNone(string(v.Verdict))))
	}
	if c.Watch != nil {
		lines = append(lines, fmt.Sprintf("  watch %s %s fires=%d/%d", c.Watch.WatchID, c.Watch.Status, len(c.Watch.Fires), c.Watch.MaxFires))
	}
	return lines
}
