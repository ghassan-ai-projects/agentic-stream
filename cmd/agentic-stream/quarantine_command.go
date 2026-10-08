package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newQuarantineCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "quarantine", Short: "List, release and redrive quarantined evidence."}
	cmd.AddCommand(newQuarantineListCommand(), newQuarantineChangeCommand("release", "Release a quarantined event for redrive (the runtime must be stopped).", releaseQuarantine),
		newQuarantineChangeCommand("redrive", "Validate a released event against the registered schemas and append it to the log (the runtime must be stopped).", redriveQuarantine))
	return cmd
}

func newQuarantineListCommand() *cobra.Command {
	return newDatabaseCommand("list", "List quarantined, released, redriven and rejected evidence, newest first.", cobra.NoArgs,
		func(cmd *cobra.Command, flags operatorFlags, db *storage.DB, _ []string) error {
			return runQuarantineList(cmd, flags, db)
		})
}

type quarantineChange func(context.Context, *storage.DB, string, string) (string, error)

func newQuarantineChangeCommand(name, short string, change quarantineChange) *cobra.Command {
	return newOperatorCommand(name+" <event-id>", short, cobra.ExactArgs(1),
		func(cmd *cobra.Command, flags operatorFlags, args []string) error {
			return runQuarantineChange(cmd, flags, change, args[0])
		})
}

func runQuarantineList(cmd *cobra.Command, flags operatorFlags, db *storage.DB) error {
	records, err := eventlog.NewEventLog(db).Quarantined(cmd.Context(), flags.tenantID)
	if err != nil {
		return fmt.Errorf("list quarantine: %w", err)
	}
	return printResult(cmd, flags.asJSON, records, func() string { return quarantineTable(records) })
}

func quarantineTable(records []eventlog.QuarantineRecord) string {
	if len(records) == 0 {
		return "quarantine: empty"
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\tattempts=%d\tlast_seen=%s\t%s", record.EventID, record.Status, record.ReasonCode, record.AttemptCount, record.LastSeenAt, record.EventType))
	}
	return strings.Join(lines, "\n")
}

func runQuarantineChange(cmd *cobra.Command, flags operatorFlags, change quarantineChange, eventID string) error {
	db, err := flags.openOperatorDatabase(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var outcome string
	if err := withRuntimeOwnership(cmd.Context(), db, func(operatorOwnership) (err error) {
		outcome, err = change(cmd.Context(), db, flags.tenantID, eventID)
		return err
	}); err != nil {
		return err
	}
	return printResult(cmd, flags.asJSON, map[string]string{"event_id": eventID, "result": outcome}, func() string { return "quarantine: " + eventID + " " + outcome })
}

func releaseQuarantine(ctx context.Context, db *storage.DB, tenantID, eventID string) (string, error) {
	if err := eventlog.NewEventLog(db).ReleaseQuarantine(ctx, tenantID, eventID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", fmt.Errorf("release %s: %w", eventID, err)
	}
	return "released", nil
}

func redriveQuarantine(ctx context.Context, db *storage.DB, tenantID, eventID string) (string, error) {
	position, err := eventlog.NewEventLog(db).RequireSchemaValidation().RedriveQuarantine(ctx, tenantID, eventID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", fmt.Errorf("redrive %s: %w", eventID, err)
	}
	return fmt.Sprintf("redriven at log position %d", position), nil
}
