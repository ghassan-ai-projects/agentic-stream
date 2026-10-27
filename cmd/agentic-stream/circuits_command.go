package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newCircuitsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "circuits", Short: "List playbook rule circuits or close one to switch its rule back on."}
	cmd.AddCommand(newListCommand("list", "List the tenant's rule circuits.", runCircuitsList), newCircuitsCloseCommand())
	return cmd
}

func runCircuitsList(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
	circuits, err := playbook.Circuits(cmd.Context(), db, flags.tenantID)
	if err != nil {
		return fmt.Errorf("list rule circuits: %w", err)
	}
	return printResult(cmd, flags.asJSON, circuits, func() string {
		return tableText(circuits, "no rule circuits", func(c playbook.CircuitView) string {
			return fmt.Sprintf("%s v%d %s %s opened %s %s", c.Rule.ID, c.Rule.Version, c.State, c.Cause, c.OpenedAt, c.CloseReason)
		})
	})
}

func newCircuitsCloseCommand() *cobra.Command {
	var rule playbook.RuleKey
	var reason string
	return newListCommand("close", "Close a rule's open circuit (the runtime must be stopped).",
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
			return closeCircuit(cmd.Context(), db, flags.tenantID, rule, reason)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&rule.ID, "rule", "", "Rule id (required)")
			cmd.Flags().IntVar(&rule.Version, "version", 0, "Rule version (required)")
			cmd.Flags().StringVar(&reason, "reason", "", "Why the rule may answer cases again (required)")
		})
}

func closeCircuit(ctx context.Context, db *storage.DB, tenantID string, rule playbook.RuleKey, reason string) error {
	return withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) error {
		fence := func(ctx context.Context, tx *sql.Tx) error { return ownership.owner.Assert(ctx, tx, ownership.epoch) }
		if err := playbook.CloseCircuit(ctx, db, fence, tenantID, rule, reason, time.Now()); err != nil {
			return fmt.Errorf("close the circuit of rule %s v%d: %w", rule.ID, rule.Version, err)
		}
		return nil
	})
}
