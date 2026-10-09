package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newMaintenanceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "maintenance", Short: "Back up the runtime database or prune bookkeeping it no longer needs."}
	cmd.AddCommand(newBackupCommand(), newPruneCommand())
	return cmd
}

func newBackupCommand() *cobra.Command {
	var output string
	return newListCommand("backup", "Write a consistent copy of the database to a new file (safe while serve runs).",
		func(cmd *cobra.Command, _ operatorFlags, db *storage.DB) error {
			if output == "" {
				return fmt.Errorf("--output is required")
			}
			if err := db.BackupInto(cmd.Context(), output); err != nil {
				return fmt.Errorf("backup: %w", err)
			}
			cmd.Printf("backup=%s\n", output)
			return nil
		},
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&output, "output", "", "New file to write the backup to (required)")
		})
}

type pruneResult struct {
	Before             string `json:"before"`
	IgnoredEvaluations int64  `json:"ignored_evaluations"`
	InboxEntries       int64  `json:"inbox_entries"`
	Timers             int64  `json:"timers"`
	SituationVersions  int64  `json:"situation_versions"`
	LineageSets        int64  `json:"lineage_sets"`
}

func newPruneCommand() *cobra.Command {
	var olderThan time.Duration
	var vacuum bool
	return newListCommand("prune", "Remove bookkeeping older than --older-than that nothing references (the runtime must be stopped).",
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
			result, err := pruneHistory(cmd.Context(), db, flags.tenantID, time.Now().UTC().Add(-olderThan), vacuum)
			if err != nil {
				return err
			}
			return printResult(cmd, flags.asJSON, result, result.text)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().DurationVar(&olderThan, "older-than", 30*24*time.Hour, "Keep everything newer than this")
			cmd.Flags().BoolVar(&vacuum, "vacuum", false, "Reclaim the freed space afterwards")
		})
}

func pruneHistory(ctx context.Context, db *storage.DB, tenantID string, before time.Time, vacuum bool) (pruneResult, error) {
	result := pruneResult{Before: before.Format(time.RFC3339)}
	err := withRuntimeOwnership(ctx, db, func(operatorOwnership) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return pruneInTx(ctx, tx, tenantID, before, &result) })
	})
	if err != nil {
		return pruneResult{}, fmt.Errorf("prune: %w", err)
	}
	if vacuum {
		if err := db.Vacuum(ctx); err != nil {
			return result, fmt.Errorf("prune: %w", err)
		}
	}
	return result, nil
}

func pruneInTx(ctx context.Context, tx *sql.Tx, tenantID string, before time.Time, result *pruneResult) error {
	ignored, err := cognition.PruneIgnoredEvaluations(ctx, tx, tenantID, before)
	if err != nil {
		return fmt.Errorf("prune trigger evaluations: %w", err)
	}
	stream, err := engine.PruneHistory(ctx, tx, tenantID, before)
	if err != nil {
		return fmt.Errorf("prune stream history: %w", err)
	}
	result.IgnoredEvaluations = ignored
	result.InboxEntries, result.Timers, result.SituationVersions, result.LineageSets = stream.InboxEntries, stream.Timers, stream.SituationVersions, stream.LineageSets
	return nil
}

func (r pruneResult) text() string {
	return fmt.Sprintf("pruned before=%s ignored_evaluations=%d inbox_entries=%d timers=%d situation_versions=%d lineage_sets=%d",
		r.Before, r.IgnoredEvaluations, r.InboxEntries, r.Timers, r.SituationVersions, r.LineageSets)
}
