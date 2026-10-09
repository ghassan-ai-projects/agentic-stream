package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newNotificationsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "notifications", Short: "Maintain the durable notification outbox."}
	cmd.AddCommand(newNotificationsPruneCommand())
	return cmd
}

func newNotificationsPruneCommand() *cobra.Command {
	var retention time.Duration
	var dryRun bool
	return newOperatorCommand("prune --retention <duration>",
		"Retire notifications older than the retention (at least 168h); the runtime must be stopped.", cobra.NoArgs,
		func(cmd *cobra.Command, flags operatorFlags, _ []string) error {
			return runNotificationsPrune(cmd, flags, retention, dryRun)
		},
		func(cmd *cobra.Command) {
			cmd.Flags().DurationVar(&retention, "retention", 0, "Keep notifications newer than this (minimum 168h)")
			cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Count what would be retired without changing anything")
		})
}

func runNotificationsPrune(cmd *cobra.Command, flags operatorFlags, retention time.Duration, dryRun bool) error {
	var retired int64
	if err := withOperatorDatabase(cmd, flags, func(db *storage.DB) (err error) {
		retired, err = pruneNotifications(cmd.Context(), db, retention, dryRun)
		return err
	}); err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, map[string]any{"retired": retired, "dry_run": dryRun}, func() string {
		if dryRun {
			return fmt.Sprintf("notifications: %d would be retired (dry run, nothing changed)", retired)
		}
		return fmt.Sprintf("notifications: %d retired", retired)
	})
}

func pruneNotifications(ctx context.Context, db *storage.DB, retention time.Duration, dryRun bool) (int64, error) {
	outbox, err := notify.New(db)
	if err != nil {
		return 0, fmt.Errorf("open notification outbox: %w", err)
	}
	if dryRun {
		return countPrunable(ctx, outbox, retention)
	}
	return pruneUnderOwnership(ctx, db, outbox, retention)
}

func countPrunable(ctx context.Context, outbox *notify.Service, retention time.Duration) (int64, error) {
	count, err := outbox.Prunable(ctx, time.Now().UTC(), retention)
	if err != nil {
		return 0, fmt.Errorf("count prunable notifications: %w", err)
	}
	return count, nil
}

func pruneUnderOwnership(ctx context.Context, db *storage.DB, outbox *notify.Service, retention time.Duration) (int64, error) {
	var retired int64
	err := withRuntimeOwnership(ctx, db, func(operatorOwnership) (err error) {
		retired, err = outbox.Prune(ctx, time.Now().UTC(), retention)
		if err != nil {
			return fmt.Errorf("prune notifications: %w", err)
		}
		return nil
	})
	return retired, err
}
