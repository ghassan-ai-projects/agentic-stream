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
	var flags operatorFlags
	var retention time.Duration
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prune --retention <duration>",
		Short: "Retire notifications older than the retention (at least 168h); the runtime must be stopped.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNotificationsPrune(cmd, flags, retention, dryRun)
		},
	}
	flags.register(cmd)
	cmd.Flags().DurationVar(&retention, "retention", 0, "Keep notifications newer than this (minimum 168h)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Count what would be retired without changing anything")
	return cmd
}

func runNotificationsPrune(cmd *cobra.Command, flags operatorFlags, retention time.Duration, dryRun bool) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	retired, err := pruneNotifications(cmd.Context(), db, retention, dryRun)
	if err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, map[string]any{"retired": retired, "dry_run": dryRun}, func() string {
		if dryRun {
			return fmt.Sprintf("notifications: %d would be retired (dry run, nothing changed)", retired)
		}
		return fmt.Sprintf("notifications: %d retired", retired)
	})
}

// pruneNotifications retires old notifications under the runtime owner lease,
// so a live subscriber's cursor cannot race the deletion; a dry run only counts.
func pruneNotifications(ctx context.Context, db *storage.DB, retention time.Duration, dryRun bool) (int64, error) {
	outbox, err := notify.New(db)
	if err != nil {
		return 0, fmt.Errorf("open notification outbox: %w", err)
	}
	now := time.Now().UTC()
	if dryRun {
		return outbox.Prunable(ctx, now, retention) //nolint:wrapcheck // The notify module names the failed step.
	}
	var retired int64
	err = withRuntimeOwnership(ctx, db, func(operatorOwnership) (err error) {
		retired, err = outbox.Prune(ctx, now, retention)
		return err //nolint:wrapcheck // The notify module names the failed step.
	})
	return retired, err
}
