package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newCommandsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "commands", Short: "List and reconcile commands whose outcome is uncertain."}
	cmd.AddCommand(newCommandsListCommand(), newCommandsResolveCommand())
	return cmd
}

func newCommandsListCommand() *cobra.Command {
	var flags operatorFlags
	cmd := &cobra.Command{
		Use: "list", Short: "List commands awaiting reconciliation (outcome_unknown, reconciling, manual_review).", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runCommandsList(cmd, flags) },
	}
	flags.register(cmd)
	return cmd
}

func newCommandsResolveCommand() *cobra.Command {
	var flags operatorFlags
	var status, evidenceFile string
	cmd := &cobra.Command{
		Use:   "resolve <command-id> --status succeeded|failed|manual_review --evidence <evidence.json>",
		Short: "Close an uncertain command with independent evidence (the runtime must be stopped).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommandsResolve(cmd, flags, args[0], status, evidenceFile)
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&status, "status", "", "Final status: succeeded, failed or manual_review")
	cmd.Flags().StringVar(&evidenceFile, "evidence", "", "JSON evidence with source and evidence_type")
	return cmd
}

func runCommandsList(cmd *cobra.Command, flags operatorFlags) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	reconciler, err := actions.NewReconciler(actions.ReconcilerConfig{DB: db, RuntimeOwner: refuseWrites, Epoch: "read-only"})
	if err != nil {
		return fmt.Errorf("open reconciler: %w", err)
	}
	awaiting, err := reconciler.Awaiting(cmd.Context(), flags.tenantID)
	if err != nil {
		return fmt.Errorf("list commands: %w", err)
	}
	return printResult(cmd, flags.asJSON, awaiting, func() string { return awaitingTable(awaiting) })
}

func refuseWrites(context.Context, *sql.Tx, string) error {
	return fmt.Errorf("read-only listing holds no runtime ownership")
}

func awaitingTable(awaiting []actions.AwaitingCommand) string {
	if len(awaiting) == 0 {
		return "commands: none awaiting reconciliation"
	}
	lines := make([]string, 0, len(awaiting))
	for _, command := range awaiting {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s -> %s\tintent=%s\tupdated=%s", command.CommandID, command.Status, command.Route, command.Target, command.IntentID, command.UpdatedAt))
	}
	return strings.Join(lines, "\n")
}

func runCommandsResolve(cmd *cobra.Command, flags operatorFlags, commandID, status, evidenceFile string) error {
	evidence, err := readEvidence(evidenceFile)
	if err != nil {
		return err
	}
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := resolveCommand(cmd.Context(), db, commandID, status, evidence); err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, map[string]string{"command_id": commandID, "status": status}, func() string { return "commands: " + commandID + " resolved " + status })
}

func readEvidence(file string) (map[string]any, error) {
	if file == "" {
		return nil, fmt.Errorf("--evidence is required")
	}
	data, err := os.ReadFile(file) //nolint:gosec // The operator names the evidence file.
	if err != nil {
		return nil, fmt.Errorf("read evidence: %w", err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(data, &evidence); err != nil {
		return nil, fmt.Errorf("decode evidence %s: %w", file, err)
	}
	return evidence, nil
}

func resolveCommand(ctx context.Context, db *storage.DB, commandID, status string, evidence map[string]any) error {
	return withRuntimeOwnership(ctx, db, func(ownership operatorOwnership) error {
		reconciler, err := actions.NewReconciler(actions.ReconcilerConfig{DB: db, RuntimeOwner: ownership.owner.Assert, Epoch: ownership.epoch})
		if err != nil {
			return fmt.Errorf("open reconciler: %w", err)
		}
		if err := reconciler.Resolve(ctx, commandID, status, evidence); err != nil {
			return fmt.Errorf("resolve %s: %w", commandID, err)
		}
		return nil
	})
}
